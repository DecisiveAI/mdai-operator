package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Adapter interface {
	ensureDeletionProcessed(ctx context.Context) (OperationResult, error)

	ensureFinalizerInitialized(ctx context.Context) (OperationResult, error)
	ensureFinalizerDeleted(ctx context.Context) error

	ensureStatusInitialized(ctx context.Context) (OperationResult, error)
	ensureStatusSetToDone(ctx context.Context) (OperationResult, error)

	ensureSynchronized(ctx context.Context) (OperationResult, error)

	finalize(ctx context.Context) (ObjectState, error)

	deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error
}
