package controller

import (
	"context"
	"errors"
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
	"slices"
)

const (
	mdaiReplayFinalizerName = "mydecisive.ai/finalizer"
)

var _ Adapter = (*MdaiReplayAdapter)(nil)

type MdaiReplayAdapter struct {
	replayCR *mdaiv1.MdaiReplay
	logger   logr.Logger
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
}

func NewMdaiReplayAdapter(
	cr *mdaiv1.MdaiReplay,
	log logr.Logger,
	k8sClient client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
) *MdaiReplayAdapter {
	return &MdaiReplayAdapter{
		replayCR: cr,
		logger:   log,
		client:   k8sClient,
		recorder: recorder,
		scheme:   scheme,
	}
}

func (c MdaiReplayAdapter) ensureDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if c.replayCR.DeletionTimestamp.IsZero() {
		return ContinueProcessing()
	}
	c.logger.Info("Deleting MdaiReplay:" + c.replayCR.Name)
	crState, err := c.finalize(ctx)
	if crState == ObjectUnchanged || err != nil {
		c.logger.Info("Has to requeue mdaireplay")
		return RequeueAfter(requeueTime, err)
	}
	return StopProcessing()
}

func (c MdaiReplayAdapter) ensureFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if controllerutil.ContainsFinalizer(c.replayCR, mdaiReplayFinalizerName) {
		return ContinueProcessing()
	}
	c.logger.Info("Adding Finalizer for MdaiReplay")
	if ok := controllerutil.AddFinalizer(c.replayCR, mdaiReplayFinalizerName); !ok {
		c.logger.Error(nil, "Failed to add finalizer into the custom resource")
		return RequeueWithError(errors.New("failed to add finalizer " + mdaiReplayFinalizerName))
	}

	if err := c.client.Update(ctx, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to update custom resource to add finalizer")
		return RequeueWithError(err)
	}
	return StopProcessing() // when finalizer is added it will trigger reconciliation
}

func (c MdaiReplayAdapter) ensureFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting MdaiReplay Finalizer")
	return c.deleteFinalizer(ctx, c.replayCR, mdaiReplayFinalizerName)
}

func (c MdaiReplayAdapter) ensureStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.replayCR.Status.Conditions) != 0 {
		return ContinueProcessing()
	}
	meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to update MdaiReplay status")
		return RequeueWithError(err)
	}
	c.logger.Info("Re-queued to reconcile with updated status")
	return StopProcessing()
}

func (c MdaiReplayAdapter) ensureStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.replayCR.Name, Namespace: c.replayCR.Namespace}, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch MdaiReplay")
		return Requeue()
	}
	meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update mdai replay status")
		return Requeue()
	}
	c.logger.Info("Status set to done for mdai replay", "mdaiHub", c.replayCR.Name)
	return ContinueProcessing()
}

func (c MdaiReplayAdapter) ensureSynchronized(ctx context.Context) (OperationResult, error) {
	//replays := c.replayCR.Spec.replays
	//replayResource := c.replayCR.Spec.replayResource
	//
	//if len(replays) == 0 {
	//	c.logger.Info("No replays found in the CR, skipping replay synchronization")
	//	return ContinueProcessing()
	//}
	//
	//hash, err := c.createOrUpdatereplayResourceConfigMap(ctx, replayResource, replays)
	//if err != nil {
	//	return RequeueWithError(err)
	//}
	//
	//if err := c.createOrUpdatereplayResourceDeployment(ctx, c.replayCR.Namespace, hash, replayResource); err != nil {
	//	if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
	//		c.logger.Info("re-queuing due to resource conflict")
	//		return Requeue()
	//	}
	//	return RequeueWithError(err)
	//}
	//
	//if err := c.createOrUpdatereplayResourceService(ctx, c.replayCR.Namespace); err != nil {
	//	return RequeueWithError(err)
	//}

	return ContinueProcessing()
}

func (c MdaiReplayAdapter) finalize(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.replayCR, mdaiReplayFinalizerName) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for MdaiReplay before delete CR")

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.replayCR.Name, Namespace: c.replayCR.Namespace}, c.replayCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("MdaiReplay has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "Failed to re-fetch MdaiReplay")
		return ObjectUnchanged, err
	}

	//if meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{
	//	Type:    typeDegradedHub,
	//	Status:  metav1.ConditionTrue,
	//	Reason:  "Finalizing",
	//	Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.replayCR.Name),
	//}) {
	//	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
	//		if apierrors.IsNotFound(err) {
	//			c.logger.Info("MdaiReplay has been deleted, no need to finalize")
	//			return ObjectModified, nil
	//		}
	//		c.logger.Error(err, "Failed to update MdaiReplay status")
	//
	//		return ObjectUnchanged, err
	//	}
	//}

	c.logger.Info("Removing Finalizer for MdaiReplay after successfully perform the operations")
	if err := c.ensureFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

func (c MdaiReplayAdapter) deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error {
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
