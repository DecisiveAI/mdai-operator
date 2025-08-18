package controller

import (
	"context"
	"reflect"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
)

// MdaiIngressReconciler reconciles a MdaiIngress object
type MdaiIngressReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Cache     cache.Cache
	ZapLogger *zap.Logger
}

//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiingresses/finalizers,verbs=update

func (r *MdaiIngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	log.Info("-- Starting MDAI Ingress reconciliation --", "name", req.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiIngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()
	return ctrl.NewControllerManagedBy(mgr).
		For(&hubv1.MdaiIngress{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				cr := e.Object.(*hubv1.MdaiIngress)
				return r.pairOtelcolMdaiIngressExist(ctx, cr.Name, cr.Namespace)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				newCr := e.ObjectNew.(*hubv1.MdaiIngress)
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
			handler.EnqueueRequestsFromMapFunc(r.requeueByCollectorRef),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCR := e.ObjectOld.(*v1beta1.OpenTelemetryCollector)
					newCR := e.ObjectNew.(*v1beta1.OpenTelemetryCollector)
					return !reflect.DeepEqual(oldCR.Spec.Config, newCR.Spec.Config) && r.pairOtelcolMdaiIngressExist(ctx, newCR.Name, newCR.Namespace)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					cr := e.Object.(*v1beta1.OpenTelemetryCollector)
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

func (r *MdaiIngressReconciler) requeueByCollectorRef(ctx context.Context, obj client.Object) []reconcile.Request {
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

// pairOtelcolMdaiIngressExist checks if there exists a MdaiIngress with the same name and namespace as the provided OpentelemetryCollector
// TODO: change mdaiingress -> otelcol mapping logic? Labels?
func (r *MdaiIngressReconciler) pairOtelcolMdaiIngressExist(ctx context.Context, name string, namespace string) bool {
	log := logger.FromContext(ctx)
	
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	otelcol := v1beta1.OpenTelemetryCollector{}
	if err := r.Cache.Get(ctx, namespacedName, &otelcol); err != nil {
		log.Info("Failed to get OpentelemetryCollector", "name", name, "namespace", namespace, "error", err)
		return false
	}
	mdaiIngress := hubv1.MdaiIngress{}
	if err := r.Cache.Get(ctx, namespacedName, &mdaiIngress); err != nil {
		log.Info("Failed to get MdaiIngress", "name", name, "namespace", namespace, "error", err)
		return false
	}

	return true
}
