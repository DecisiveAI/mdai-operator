/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/valkey-io/valkey-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	mdaiv1 "mdai.ai/operator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	LabelMdaiHubName  = "mdaihub-name" // Replace with your actual label key
	VariableKeyPrefix = "variable/"
)

// MdaiHubReconciler reconciles a MdaiHub object
type MdaiHubReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	ValKeyClient *valkey.Client
	valkeyEvents chan event.GenericEvent
}

// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs/finalizers,verbs=update
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *MdaiHubReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	log.Info("-- Starting reconciliation --")

	fetchedCR := &mdaiv1.MdaiHub{}
	if err := r.Get(ctx, req.NamespacedName, fetchedCR); err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch MyDecisiveEngine CR:"+req.NamespacedName.Namespace+" : "+req.NamespacedName.Name)
		}
		log.Info("-- Exiting reconciliation, CR is deleted already --")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	_, err := r.ReconcileHandler(ctx, *NewHubAdapter(fetchedCR, log, r.Client, r.Recorder, r.Scheme, r.ValKeyClient))
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("-- Finished reconciliation --")
	return ctrl.Result{}, nil
}

func (r *MdaiHubReconciler) ReconcileHandler(ctx context.Context, adapter HubAdapter) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.ensureHubDeletionProcessed,
		adapter.ensureStatusInitialized,
		adapter.ensureFinalizerInitialized,
		adapter.ensureEvaluationsSynchronized,
		adapter.ensureVariableSynced,
		adapter.ensureObserversSynchronized,
		adapter.ensureStatusSetToDone,
	}
	for _, operation := range operations {
		result, err := operation(ctx)
		if err != nil || result.RequeueRequest {
			return ctrl.Result{RequeueAfter: result.RequeueDelay}, err
		}
		if result.CancelRequest {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

type ReconcileOperation func(context.Context) (OperationResult, error)

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.initializeValkey(); err != nil {
		return err
	}

	// watch collectors which have the hub label
	collectorSelector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      LabelMdaiHubName,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	selectorPredicate, err := predicate.LabelSelectorPredicate(collectorSelector)
	if err != nil {
		return err
	}

	combinedPredicate := predicate.And(selectorPredicate, createPredicate)

	r.valkeyEvents = make(chan event.GenericEvent)

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&mdaiv1.MdaiHub{}).
		Owns(&v1.ConfigMap{}, builder.WithPredicates(mdaiResourcesPredicate())).
		Owns(&v1.Service{}, builder.WithPredicates(mdaiResourcesPredicate())).
		Owns(&appsv1.Deployment{}, builder.WithPredicates(mdaiResourcesPredicate())).
		Watches(
			// we are watching OpenTelemetryCollector resources to detect if new ones have been created
			// if new ones are created, we have to provide mdai variables for new collectors
			// we are not interested in delete or update events for otel collectors
			&v1beta1.OpenTelemetryCollector{},
			handler.EnqueueRequestsFromMapFunc(r.requeueByLabels),
			builder.WithPredicates(combinedPredicate),
		).
		WatchesRawSource(
			source.Channel(
				r.valkeyEvents,
				&handler.EnqueueRequestForObject{},
			),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Named("mdaihub").
		Complete(r); err != nil {
		return err
	}

	if r.ValKeyClient != nil {
		go r.startValkeySubscription()
	}

	return nil
}

func mdaiResourcesPredicate() predicate.Predicate {
	log := logger.FromContext(context.TODO())
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// log.Info("<CreateFunc> " + e.Object.GetName() + " ignored")
			return false // assuming only mdai operator creates managed resources
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			shouldReconsile := predicate.GenerationChangedPredicate{}.Update(e)
			log.Info("<UpdateFunc> " + e.ObjectNew.GetName() + " shouldReconsile: " + strconv.FormatBool(shouldReconsile))
			return shouldReconsile
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// log.Info("<DeleteFunc> " + e.Object.GetName() + " ignored")
			return false // assuming only mdai operator deletes managed resources
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// log.Info("<GenericFunc> " + e.Object.GetName() + " ignored")
			return false // we do not handle generic events
		},
	}
}

func (r *MdaiHubReconciler) startValkeySubscription() {
	ctx := context.Background()
	log := logger.FromContext(ctx)
	pattern := "__keyspace@0__:" + VariableKeyPrefix + "*"
	valkeyClient := *r.ValKeyClient
	log.Info("Starting ValKey subscription", "pattern", pattern)
	// Subscribe to all ValKey events targeting any key
	// later, we can switch to dynamically subscribing to events targeting specific keys
	if err := valkeyClient.Receive(ctx, valkeyClient.B().Psubscribe().Pattern(pattern).Build(), func(msg valkey.PubSubMessage) {
		// Do we need to batch here to avoid multiple reconciliations and restarts?
		// Apparently for most cases controller-runtime will do the deduplication of requests
		log.Info("Received message", "channel", msg.Channel, "message", msg.Message)
		// Extract the key from the channel name
		key := strings.TrimPrefix(msg.Channel, "__keyspace@0__:")
		// find hub by name from the channel name by prefix
		parts := strings.SplitN(key, "/", 3)
		if len(parts) != 3 {
			log.Info("invalid key format, skipping", "key", key)
			return
		}
		hubName := parts[1]
		hubNamespace, err := r.findHubNamespace(ctx, log, hubName)
		if err != nil {
			log.Error(err, "failed to find hub namespace", "hubName", hubName)
			return
		}

		r.valkeyEvents <- event.GenericEvent{
			Object: &mdaiv1.MdaiHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hubName,
					Namespace: hubNamespace,
				},
			},
		}
		log.Info("-- Requeueing MdaiHub triggered by valkey key change", "hubName", hubName, "hubNamespace", hubNamespace, "key", key)
	}); err != nil {
		log.Error(err, "failed to subscribe to ValKey channel")
		return
	}
}

func (r *MdaiHubReconciler) initializeValkey() error {
	// for built-in valkey storage we read the environment variable to get connection string
	valkeyEndpoint := os.Getenv("VALKEY_ENDPOINT")
	valkeyPassword := os.Getenv("VALKEY_PASSWORD")
	if valkeyEndpoint == "" || valkeyPassword == "" {
		log := logger.FromContext(context.Background())
		log.Info("ValKey client is not enabled; skipping initialization")
	} else {
		log := logger.FromContext(context.Background())
		log.Info("Initializing ValKey client", "endpoint", valkeyEndpoint)
		operation := func() error {
			valkeyClient, err := valkey.NewClient(valkey.ClientOption{
				InitAddress: []string{valkeyEndpoint},
				Password:    valkeyPassword,
			})
			if err != nil {
				log.Error(err, "Failed to initialize ValKey client. Retrying...")
				return err
			}
			r.ValKeyClient = &valkeyClient
			return nil
		}

		backoffConfig := backoff.NewExponentialBackOff()
		backoffConfig.InitialInterval = 5 * time.Second
		backoffConfig.MaxElapsedTime = 3 * time.Minute

		err := backoff.Retry(operation, backoffConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize ValKey client after retries: %w", err)
		}
	}
	return nil
}

func (r *MdaiHubReconciler) requeueByLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logger.FromContext(ctx)
	log.Info("requeueByLabels called", "object", obj.GetName())

	otelCollector, ok := obj.(*v1beta1.OpenTelemetryCollector)
	if !ok {
		log.Error(nil, "object is not an OpenTelemetryCollector")
		return nil
	}

	hubNameFromLabel, exists := otelCollector.Labels[LabelMdaiHubName]
	if !exists || hubNameFromLabel == "" {
		log.Info("OpenTelemetryCollector does not have the hubNameFromLabel 'mdaihub-name'; skipping requeue")
		return nil
	}
	log.Info("OpenTelemetryCollector for MdaiHub found with hubNameFromLabel", "hubNameFromLabel", hubNameFromLabel)

	hubNamespace, err := r.findHubNamespace(ctx, log, hubNameFromLabel)
	if err != nil {
		return nil
	}

	log.Info("-- Requeueing MdaiHub triggered by otel collector", "hubNameFromLabel", hubNameFromLabel, "otelCollector", otelCollector.Name, "hubNamespace", hubNamespace)

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      hubNameFromLabel,
				Namespace: hubNamespace,
			},
		},
	}
}

func (r *MdaiHubReconciler) findHubNamespace(ctx context.Context, log logr.Logger, hubNameFromLabel string) (string, error) {
	listOptions := []client.ListOption{
		client.InNamespace(""), // all namespaces
	}
	hubList := &mdaiv1.MdaiHubList{}
	if err := r.List(ctx, hubList, listOptions...); err != nil {
		log.Error(err, "Failed to list MdaiHubs")
		return "", err
	}

	var targetHub *mdaiv1.MdaiHub
	for _, hub := range hubList.Items {
		if hub.Name == hubNameFromLabel {
			targetHub = &hub
			break
		}
	}

	if targetHub == nil {
		log.Info("MdaiHub not found", "hubName", hubNameFromLabel)
		return "", errors.New("mdaiHub not found")
	}

	// Assuming that hub names are unique across namespaces, take the first match
	hubNamespace := targetHub.Namespace
	return hubNamespace, nil
}

var createPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return false
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false // Skip delete events
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return false // Skip generic events
	},
}
