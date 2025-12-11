package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MdaiDalSpec defines the desired state of MdaiDal
type MdaiDalSpec struct {
	// +required
	// +kubebuilder:validation:Enum=weekly;daily;hourly;minutely
	Granularity string `json:"granularity"`
	// +kubebuilder:default=false
	UsePayloadTS bool `json:"usePayloadTs,omitempty"`
	// +kubebuilder:default=json
	Marshaler string `json:"marshaler,omitempty"`

	S3  MdaiDalS3Config  `json:"s3"`
	AWS MdaiDalAWSConfig `json:"aws"`
}

type MdaiDalS3Config struct {
	// +required
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern="^[a-z0-9][a-z0-9.-]*[a-z0-9]$"
	Bucket string `json:"bucket"`

	// +kubebuilder:validation:Pattern="^[a-zA-Z_]*$"
	Prefix string `json:"prefix,omitempty"`
}

type MdaiDalAWSConfig struct {
	// +required
	// +kubebuilder:validation:Pattern=`^[a-z]{2}(-[a-z]+)+-\d+$`
	Region string `json:"region"`

	Credentials MdaiDalAWSCredentials `json:"credentials,omitempty"`
}

type MdaiDalAWSCredentials struct {
	// +kubebuilder:default="aws-credentials"
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="AWS_ACCESS_KEY_ID"
	AccessKeyField string `json:"accessKeyField,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="AWS_SECRET_ACCESS_KEY"
	SecretKeyField string `json:"secretKeyField,omitempty"`
}

// MdaiDalStatus defines the observed state of MdaiDal.
type MdaiDalStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the MdaiDal resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MdaiDal is the Schema for the mdaidals API
type MdaiDal struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MdaiDal
	// +required
	Spec MdaiDalSpec `json:"spec"`

	// status defines the observed state of MdaiDal
	// +optional
	Status MdaiDalStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MdaiDalList contains a list of MdaiDal
type MdaiDalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`

	Items []MdaiDal `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&MdaiDal{}, &MdaiDalList{})
}
