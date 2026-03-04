package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ Adapter = (*ObserverAdapter)(nil)

type ObserverAdapter struct {
	observerCR  *mdaiv1.MdaiObserver
	logger      logr.Logger
	client      client.Client
	recorder    record.EventRecorder
	scheme      *runtime.Scheme
	releaseName string
	greptime    Greptime
}

type ObserverIndex struct {
	byProvider     map[mdaiv1.ObserverProvider][]mdaiv1.Observer
	byProviderType map[mdaiv1.ObserverProvider]map[mdaiv1.ObserverType][]mdaiv1.Observer
}

func BuildObserverIndex(observers []mdaiv1.Observer) ObserverIndex {
	idx := ObserverIndex{
		byProvider:     make(map[mdaiv1.ObserverProvider][]mdaiv1.Observer),
		byProviderType: make(map[mdaiv1.ObserverProvider]map[mdaiv1.ObserverType][]mdaiv1.Observer),
	}
	for _, obs := range observers {
		idx.byProvider[obs.Provider] = append(idx.byProvider[obs.Provider], obs)
		if idx.byProviderType[obs.Provider] == nil {
			idx.byProviderType[obs.Provider] = make(map[mdaiv1.ObserverType][]mdaiv1.Observer)
		}
		idx.byProviderType[obs.Provider][obs.Type] = append(idx.byProviderType[obs.Provider][obs.Type], obs)
	}
	return idx
}

func (idx ObserverIndex) ByProvider(provider mdaiv1.ObserverProvider) []mdaiv1.Observer {
	return idx.byProvider[provider]
}

func (idx ObserverIndex) By(provider mdaiv1.ObserverProvider, obsType mdaiv1.ObserverType) []mdaiv1.Observer {
	if idx.byProviderType[provider] == nil {
		return nil
	}
	return idx.byProviderType[provider][obsType]
}

func NewObserverAdapter(
	cr *mdaiv1.MdaiObserver,
	log logr.Logger,
	k8sClient client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
	greptime Greptime,
) *ObserverAdapter {
	return &ObserverAdapter{
		observerCR:  cr,
		logger:      log,
		client:      k8sClient,
		recorder:    recorder,
		scheme:      scheme,
		releaseName: os.Getenv("RELEASE_NAME"),
		greptime:    greptime,
	}
}

func (c ObserverAdapter) ensureFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if controllerutil.ContainsFinalizer(c.observerCR, hubFinalizer) {
		return ContinueProcessing()
	}
	c.logger.Info("Adding Finalizer for MdaiObserver")
	if ok := controllerutil.AddFinalizer(c.observerCR, hubFinalizer); !ok {
		c.logger.Error(nil, "Failed to add finalizer into the custom resource")
		return RequeueWithError(errors.New("failed to add finalizer " + hubFinalizer))
	}

	if err := c.client.Update(ctx, c.observerCR); err != nil {
		c.logger.Error(err, "Failed to update custom resource to add finalizer")
		return RequeueWithError(err)
	}
	return StopProcessing() // when finalizer is added it will trigger reconciliation
}

func (c ObserverAdapter) ensureStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.observerCR.Status.Conditions) != 0 {
		return ContinueProcessing()
	}
	meta.SetStatusCondition(&c.observerCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	if err := c.client.Status().Update(ctx, c.observerCR); err != nil {
		c.logger.Error(err, "Failed to update MdaiObserver status")
		return RequeueWithError(err)
	}
	c.logger.Info("Re-queued to reconcile with updated status")
	return StopProcessing()
}

// ensureDeletionProcessed deletes MdaiObserver in cases a deletion was triggered
func (c ObserverAdapter) ensureDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if c.observerCR.DeletionTimestamp.IsZero() {
		return ContinueProcessing()
	}
	c.logger.Info("Deleting MdaiObserver:" + c.observerCR.Name)
	crState, err := c.finalize(ctx)
	if crState == ObjectUnchanged || err != nil {
		c.logger.Info("Has to requeue mdaiobserver")
		return RequeueAfter(requeueTime, err)
	}
	return StopProcessing()
}

// finalize handles the deletion of a MdaiObserver
func (c ObserverAdapter) finalize(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.observerCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for MdaiObserver before delete CR")

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.observerCR.Name, Namespace: c.observerCR.Namespace}, c.observerCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("MdaiObserver has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "Failed to re-fetch MdaiObserver")
		return ObjectUnchanged, err
	}

	if meta.SetStatusCondition(&c.observerCR.Status.Conditions, metav1.Condition{
		Type:    typeDegradedHub,
		Status:  metav1.ConditionTrue,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.observerCR.Name),
	}) {
		if err := c.client.Status().Update(ctx, c.observerCR); err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Info("MdaiObserver has been deleted, no need to finalize")
				return ObjectModified, nil
			}
			c.logger.Error(err, "Failed to update MdaiObserver status")

			return ObjectUnchanged, err
		}
	}

	c.logger.Info("Removing Finalizer for MdaiObserver after successfully perform the operations")
	if err := c.ensureFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

// ensureFinalizerDeleted removes finalizer of a Hub
func (c ObserverAdapter) ensureFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting MdaiObserver Finalizer")
	return c.deleteFinalizer(ctx, c.observerCR, hubFinalizer)
}

// deleteFinalizer deletes finalizer of a generic CR
func (c ObserverAdapter) deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error {
	metadata, err := meta.Accessor(object)
	if err != nil {
		c.logger.Error(err, "Failed to delete finalizer", "finalizer", finalizer)
		return err
	}
	finalizers := metadata.GetFinalizers()
	if slices.Contains(finalizers, finalizer) {
		metadata.SetFinalizers(slices.DeleteFunc(finalizers, func(f string) bool { return f == finalizer }))
		return c.client.Update(ctx, object)
	}
	return nil
}

func (c ObserverAdapter) ensureSynchronized(ctx context.Context) (OperationResult, error) {
	observers := c.observerCR.Spec.Observers
	observerResource := c.observerCR.Spec.ObserverResource

	if len(observers) == 0 {
		c.logger.Info("No observers found in the CR, skipping observer synchronization")
		return ContinueProcessing()
	}

	idx := BuildObserverIndex(observers)

	err := c.syncOtel(ctx, observerResource, idx)
	if err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		return RequeueWithError(err)
	}

	err = c.syncGreptime(idx)
	if err != nil {
		return RequeueWithError(err)
	}

	return ContinueProcessing()
}

func (c ObserverAdapter) syncOtel(
	ctx context.Context,
	observerResource mdaiv1.ObserverResource,
	idx ObserverIndex,
) error {
	observers := idx.ByProvider(mdaiv1.OTEL_COLLECTOR)
	if len(observers) == 0 {
		return nil
	}

	hash, err := c.createOrUpdateObserverResourceConfigMap(ctx, observerResource, observers)
	if err != nil {
		return err
	}

	if err := c.createOrUpdateObserverResourceDeployment(ctx, c.observerCR.Namespace, hash, observerResource); err != nil {
		return err
	}

	if err := c.createOrUpdateObserverResourceService(ctx, c.observerCR.Namespace); err != nil {
		return err
	}

	return nil
}

func (c ObserverAdapter) syncGreptime(idx ObserverIndex) error {
	observers := idx.By(mdaiv1.GREPTIME_FLOW, mdaiv1.SPAN_METRICS)
	if len(observers) == 0 {
		return nil
	}
	for _, obs := range observers {
		if obs.SpanMetricsObserver == nil {
			return fmt.Errorf("observer %s missing spanMetricsObserver config", obs.Name)
		}
		dimensions := []string{}
		primaryKey := ""
		if obs.SpanMetricsObserver.Greptime != nil {
			dimensions = obs.SpanMetricsObserver.Greptime.Dimensions
			primaryKey = obs.SpanMetricsObserver.Greptime.PrimaryKey
		}
		if len(dimensions) == 0 && len(obs.SpanMetricsObserver.Dimensions) > 0 {
			dimensions = obs.SpanMetricsObserver.Dimensions
		}
		if primaryKey == "" && obs.SpanMetricsObserver.PrimaryKey != "" {
			primaryKey = obs.SpanMetricsObserver.PrimaryKey
		}
		if len(dimensions) == 0 || primaryKey == "" {
			return fmt.Errorf("observer %s missing greptime dimensions/primaryKey", obs.Name)
		}
		return doGreptime(c.greptime, dimensions, primaryKey)
	}
	// TODO: delete Greptime resources (sink table and flow) when observers are deleted

	return nil
}

func (c ObserverAdapter) ensureStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.observerCR.Name, Namespace: c.observerCR.Namespace}, c.observerCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch MdaiObserver")
		return Requeue()
	}
	meta.SetStatusCondition(&c.observerCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.observerCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update mdai observer status")
		return Requeue()
	}
	c.logger.Info("Status set to done for mdai observer", "mdaiHub", c.observerCR.Name)
	return ContinueProcessing()
}
