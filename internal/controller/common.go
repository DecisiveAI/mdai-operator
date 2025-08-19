package controller

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
