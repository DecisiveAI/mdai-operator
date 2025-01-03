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

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	mdaiv1 "mdai.ai/operator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	LabelMdaiHubName = "mdaihub-name" // Replace with your actual label key
)

// MdaiHubReconciler reconciles a MdaiHub object
type MdaiHubReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mdai.mdai.ai,resources=mdaihubs/finalizers,verbs=update
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete

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

	_, err := r.ReconcileHandler(*NewHubAdapter(ctx, fetchedCR, log, r.Client, r.Recorder, r.Scheme))
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("-- Finished reconciliation --")
	return ctrl.Result{}, nil
}

func (r *MdaiHubReconciler) ReconcileHandler(adapter HubAdapter) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.ensureHubDeletionProcessed,
		adapter.ensureStatusInitialized,
		adapter.ensureFinalizerInitialized,
		adapter.ensureEvaluationsSynchronized,
		adapter.ensureVariableSynced,
	}
	for _, operation := range operations {
		result, err := operation()
		if err != nil || result.RequeueRequest {
			return ctrl.Result{RequeueAfter: result.RequeueDelay}, err
		}
		if result.CancelRequest {
			return ctrl.Result{}, nil
		}
	}
	// TODO final status update?
	return ctrl.Result{}, nil
}

type ReconcileOperation func() (OperationResult, error)

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiHubReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mdaiv1.MdaiHub{}).
		Watches(
			&v1beta1.OpenTelemetryCollector{},
			handler.EnqueueRequestsFromMapFunc(r.requeueByLabels),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
			builder.WithPredicates(noDeletePredicate),
		).
		Named("mdaihub").
		Complete(r)
}

func (r *MdaiHubReconciler) requeueByLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logger.FromContext(ctx)
	log.Info("requeueByLabels called", "object", obj.GetName())

	otelCollector, ok := obj.(*v1beta1.OpenTelemetryCollector)
	if !ok {
		log.Error(nil, "object is not an OpenTelemetryCollector")
		return nil
	}

	label, exists := otelCollector.Labels[LabelMdaiHubName]
	if !exists || label == "" {
		log.Info("OpenTelemetryCollector does not have the label 'mdaihub-name'; skipping requeue")
		return nil
	}
	log.Info("OpenTelemetryCollector for MdaiHub found with label", "label", label)

	nsName := types.NamespacedName{
		Name:      label,
		Namespace: otelCollector.Namespace, // TODO Assuming MdaiHub is namespaced
	}

	var mdaiHub mdaiv1.MdaiHub
	err := r.Get(ctx, nsName, &mdaiHub)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch MdaiHub", "label", label, "namespace", otelCollector.Namespace)
		} else {
			log.Info("MdaiHub not found; possible misconfiguration", "label", label, "namespace", otelCollector.Namespace)
		}
		return nil
	}

	log.Info("Requeuing MdaiHub triggered by otel collector", "mdaiHub", mdaiHub.Name, "otelCollector", otelCollector.Name)

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      mdaiHub.Name,
				Namespace: mdaiHub.Namespace, // TODO Assuming MdaiHub is namespaced
			},
		},
	}
}

var noDeletePredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false // Skip delete events
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return false // Skip generic events
	},
}
