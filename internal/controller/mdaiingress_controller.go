package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/decisiveai/mdai-operator/internal/manifests/collector"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
)

const resourceOwnerKey = "metadata.ownerReferences.name"

// MdaiIngressReconciler reconciles a MdaiIngress object
type MdaiIngressReconciler struct {
	client.Client

	Scheme *runtime.Scheme
	Logger *zap.Logger
}

//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses/finalizers,verbs=update
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *MdaiIngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	log.Info("-- Starting MDAI Ingress reconciliation --", "name", req.Name)

	var instanceMdaiIngress hubv1.MdaiIngress
	if err := r.Get(ctx, req.NamespacedName, &instanceMdaiIngress); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch MdaiIngress")
		}

		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var instanceOtel v1beta1.OpenTelemetryCollector
	if err := r.Get(ctx, req.NamespacedName, &instanceOtel); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch OpenTelemetryCollector")
		}

		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	otelMdaiComb := hubv1.NewOtelIngressConfig(instanceOtel, instanceMdaiIngress)

	params := r.GetParams(*otelMdaiComb)

	desiredObjects, buildErr := BuildCollector(params)
	if buildErr != nil {
		return ctrl.Result{}, buildErr
	}

	ownedObjects, err := r.findMdaiIngressOwnedObjects(ctx, params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = reconcileDesiredObjects(ctx, r.Client, log, &instanceMdaiIngress, params.Scheme, desiredObjects, ownedObjects)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("-- Finished MDAI Ingress reconciliation --", "name", req.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	// Index for Service
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.Service{},
		resourceOwnerKey,
		func(rawObj client.Object) []string {
			service, ok := rawObj.(*corev1.Service)
			if !ok {
				return nil
			}
			owners := service.GetOwnerReferences()
			if len(owners) == 0 {
				return nil
			}
			return []string{owners[0].Name}
		},
	); err != nil {
		return err
	}
	// Index for Ingress
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&networkingv1.Ingress{},
		resourceOwnerKey,
		func(rawObj client.Object) []string {
			ing, ok := rawObj.(*networkingv1.Ingress)
			if !ok {
				return nil
			}
			var ownerNames []string
			for _, owner := range ing.GetOwnerReferences() {
				ownerNames = append(ownerNames, owner.Name)
			}
			return ownerNames
		},
	); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&hubv1.MdaiIngress{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				cr, ok := e.Object.(*hubv1.MdaiIngress)
				if !ok {
					return false
				}
				return r.pairOtelcolMdaiIngressExist(ctx, cr.Name, cr.Namespace)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				newCr, ok := e.ObjectNew.(*hubv1.MdaiIngress)
				if !ok {
					return false
				}
				return r.pairOtelcolMdaiIngressExist(ctx, newCr.Name, newCr.Namespace)
			},
			DeleteFunc: func(e event.DeleteEvent) bool { // TODO: implement deletion logic
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		})).
		Watches(
			// TODO: try to narrow down the watches if possible
			&v1beta1.OpenTelemetryCollector{},
			handler.EnqueueRequestsFromMapFunc(requeueByCollectorRef),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCR, okOld := e.ObjectOld.(*v1beta1.OpenTelemetryCollector)
					oldCR.GetObjectKind().GroupVersionKind()
					if !okOld {
						return false
					}
					newCR, okNew := e.ObjectNew.(*v1beta1.OpenTelemetryCollector)
					if !okNew {
						return false
					}
					return !reflect.DeepEqual(oldCR.Spec.Config, newCR.Spec.Config) && r.pairOtelcolMdaiIngressExist(ctx, newCR.Name, newCR.Namespace)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					cr, ok := e.Object.(*v1beta1.OpenTelemetryCollector)
					if !ok {
						return false
					}
					return r.pairOtelcolMdaiIngressExist(ctx, cr.Name, cr.Namespace)
				},
				DeleteFunc: func(e event.DeleteEvent) bool { // TODO: implement deletion logic
					return false
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return false
				},
			}),
		).
		Complete(r)
}

func requeueByCollectorRef(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logger.FromContext(ctx)
	log.Info("-- Requeueing MdaiIngress triggered by otel collector", "collector name", obj.GetName(), "namespace", obj.GetNamespace())

	// As soon as this method is called _after_ predicated applied, and therefore, we know that a linked pair MdaiIngress -> Otelcol exists,
	// we dont need to check anything here, just requeue the MdaiIngress
	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			},
		},
	}
}

// BuildCollector returns the generation and collected errors of all manifests for a given instance.
func BuildCollector(params manifests.Params) ([]client.Object, error) {
	mBuilders := []manifests.Builder[manifests.Params]{
		collector.Build,
	}
	var resources []client.Object
	for _, mBuilder := range mBuilders {
		objs, err := mBuilder(params)
		if err != nil {
			return nil, err
		}
		resources = append(resources, objs...)
	}
	return resources, nil
}

func (r *MdaiIngressReconciler) GetParams(otelMdaiComb hubv1.OtelMdaiIngressComb) manifests.Params {
	p := manifests.Params{
		Client:              r.Client,
		OtelMdaiIngressComb: otelMdaiComb,
		Log:                 r.Logger,
		Scheme:              r.Scheme,
	}
	return p
}

// pairOtelcolMdaiIngressExist checks if there is a MdaiIngress with the same name and namespace as the provided OpentelemetryCollector
// TODO: change mdaiingress -> otelcol mapping logic? Labels?
func (r *MdaiIngressReconciler) pairOtelcolMdaiIngressExist(ctx context.Context, name string, namespace string) bool {
	log := logger.FromContext(ctx)

	mdaiIngresses := hubv1.MdaiIngressList{}
	compositeKey := fmt.Sprintf("%s/%s", namespace, name)
	if err := r.List(ctx, &mdaiIngresses,
		client.InNamespace(namespace),
		client.MatchingFields{"spec.otelCol.compositeKey": compositeKey},
	); err != nil {
		log.Info("Failed to list OpentelemetryCollectors", "name", name, "namespace", namespace, "error", err)
		return false
	}

	if len(mdaiIngresses.Items) == 0 {
		log.Info("No MdaiIngress found", "name", name, "namespace", namespace)
		return false
	}

	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}
	otelCollector := v1beta1.OpenTelemetryCollector{}
	if err := r.Get(ctx, namespacedName, &otelCollector); err != nil {
		log.Info("Failed to get MdaiIngress", "name", name, "namespace", namespace, "error", err)
		return false
	}

	return true
}

func (r *MdaiIngressReconciler) findMdaiIngressOwnedObjects(ctx context.Context, params manifests.Params) (map[types.UID]client.Object, error) {
	ownedObjects := map[types.UID]client.Object{}
	ownedObjectTypes := GetOwnedResourceTypes()
	listOpts := []client.ListOption{
		client.InNamespace(params.OtelMdaiIngressComb.Otelcol.Namespace),
		client.MatchingFields{resourceOwnerKey: params.OtelMdaiIngressComb.MdaiIngress.Name},
	}
	for _, objectType := range ownedObjectTypes {
		objs, err := getList(ctx, r.Client, objectType, listOpts...)
		if err != nil {
			return nil, err
		}
		for uid, object := range objs {
			ownedObjects[uid] = object
		}
	}

	return ownedObjects, nil
}

// GetOwnedResourceTypes returns all the resource types the controller can own. Even though this method returns an array
// of client.Object, these are (empty) example structs rather than actual resources.
func GetOwnedResourceTypes() []client.Object {
	ownedResources := []client.Object{
		&corev1.Service{},
		&networkingv1.Ingress{},
	}

	return ownedResources
}
