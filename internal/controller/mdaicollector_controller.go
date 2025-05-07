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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
)

// MdaiCollectorReconciler reconciles a MdaiCollector object
type MdaiCollectorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaicollectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaicollectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaicollectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events;namespaces;namespaces/status;nodes;nodes/spec;pods;pods/status;replicationcontrollers;replicationcontrollers/status;resourcequotas;services,verbs=get;list;watch
// +kubebuilder:rbac:groups="apps",resources=daemonsets;deployments;replicasets;statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups="extensions",resources=daemonsets;deployments;replicasets,verbs=get;list;watch
// +kubebuilder:rbac:groups="batch",resources=jobs;cronjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="autoscaling",resources=horizontalpodautoscalers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MdaiCollector object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *MdaiCollectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	log.Info("-- Starting MdaiCollector reconciliation --")

	fetchedCR := &mdaiv1.MdaiCollector{}
	if err := r.Get(ctx, req.NamespacedName, fetchedCR); err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch MdaiCollector CR:"+req.NamespacedName.Namespace+" : "+req.NamespacedName.Name)
		}
		log.Info("-- Exiting MdaiCollector reconciliation, CR is deleted already --")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	_, err := r.MdaiCollectorReconcileHandler(ctx, *NewMdaiCollectorAdapter(fetchedCR, log, r.Client, r.Recorder, r.Scheme))
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("-- Finished MdaiCollector reconciliation --")

	return ctrl.Result{}, nil
}

func (r *MdaiCollectorReconciler) MdaiCollectorReconcileHandler(ctx context.Context, adapter MdaiCollectorAdapter) (ctrl.Result, error) {
	operations := []ReconcileOperation{
		adapter.ensureMdaiCollectorDeletionProcessed,
		adapter.ensureMdaiCollectorStatusInitialized,
		adapter.ensureMdaiCollectorFinalizerInitialized,
		adapter.ensureMdaiCollectorSynchronized,
		adapter.ensureMdaiCollectorStatusSetToDone,
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

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiCollectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mdaiv1.MdaiCollector{}).
		Named("mdaicollector").
		Complete(r)
}
