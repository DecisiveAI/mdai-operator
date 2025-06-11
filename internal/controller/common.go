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
)

type ObjectState bool
