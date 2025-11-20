// Package v1 contains API Schema definitions for the mdai v1 API group.
// +kubebuilder:object:generate=true
// +groupName=hub.mydecisive.ai
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "hub.mydecisive.ai", Version: "v1"}

	// SchemaGroupVersion is the same as GroupVersion
	SchemeGroupVersion = schema.GroupVersion{Group: GroupVersion.Group, Version: GroupVersion.Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
