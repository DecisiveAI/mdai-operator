package controller

import (
	"context"
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	// typeAvailableHub represents the status of the Deployment reconciliation
	typeAvailableHub = "Available"
	// typeDegradedHub represents the status used when the custom resource is deleted and the finalizer operations are must occur.
	typeDegradedHub = "Degraded"

	hubFinalizer = "mydecisive.ai/finalizer"

	ObjectModified          ObjectState = true
	ObjectUnchanged         ObjectState = false
	LabelManagedByMdaiKey               = "app.kubernetes.io/managed-by"
	LabelManagedByMdaiValue             = "mdai-operator"

	otlpGRPCPort        = 4317
	otlpHTTPPort        = 4318
	promHTTPPort        = 8899
	otelMetricsPort     = 8888
	observerMetricsPort = 8899
)

type ObjectState bool

// getList queries the Kubernetes API to list the requested resource, setting the list l of type T.
func getList[T client.Object](ctx context.Context, cl client.Client, l T, options ...client.ListOption) (map[types.UID]client.Object, error) {
	ownedObjects := map[types.UID]client.Object{}
	gvk, err := apiutil.GVKForObject(l, cl.Scheme())
	if err != nil {
		return nil, err
	}
	gvk.Kind = fmt.Sprintf("%sList", gvk.Kind)
	list, err := cl.Scheme().New(gvk)
	if err != nil {
		return nil, fmt.Errorf("unable to list objects of type %s: %w", gvk.Kind, err)
	}

	objList := list.(client.ObjectList)

	err = cl.List(ctx, objList, options...)
	if err != nil {
		return ownedObjects, fmt.Errorf("error listing %T: %w", l, err)
	}
	objs, err := apimeta.ExtractList(objList)
	if err != nil {
		return ownedObjects, fmt.Errorf("error listing %T: %w", l, err)
	}
	for i := range objs {
		typedObj, ok := objs[i].(T)
		if !ok {
			return ownedObjects, fmt.Errorf("error listing %T: %w", l, err)
		}
		ownedObjects[typedObj.GetUID()] = typedObj
	}
	return ownedObjects, nil
}
