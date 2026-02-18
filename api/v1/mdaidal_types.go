package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ImagePullPolicy string

// MdaiDalSpec defines the desired state of MdaiDal
type MdaiDalSpec struct {
	// +required
	// +kubebuilder:validation:Enum=weekly;daily;hourly;minutely
	// Granularity is how often to partition the data
	Granularity string `json:"granularity"`
	// +kubebuilder:default=false
	// +optional
	// UsePayloadTS determines whether to partition by earliest payload
	// timestamp instead of ingest time
	UsePayloadTS bool `json:"usePayloadTs,omitempty"`
	// +kubebuilder:default=json
	// +optional
	// Marshaler of payload data
	Marshaler string `json:"marshaler,omitempty"`

	// +kubebuilder:validation:Required
	S3 MdaiDalS3Config `json:"s3"`
	// +kubebuilder:validation:Required
	AWS MdaiDalAWSConfig `json:"aws"`
	// +optional
	// +kubebuilder:default:={repository:"public.ecr.aws/mydecisive/mdai-dal",tag:"0.0.1",pullPolicy:"IfNotPresent"}
	ImageSpec *MdaiDalImageSpec `json:"imageSpec,omitempty"`
}

type MdaiDalS3Config struct {
	// +required
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern="^[a-z0-9][a-z0-9.-]*[a-z0-9]$"
	// Bucket is the name of the S3 bucket to write to
	Bucket string `json:"bucket"`

	// +kubebuilder:validation:Pattern="^[A-Za-z0-9._/\\-]*$"
	// +optional
	// Prefix is the S3 bucket prefix (path)
	Prefix string `json:"prefix,omitempty"`
}

type MdaiDalAWSConfig struct {
	// +required
	// +kubebuilder:validation:Pattern=`^[a-z]{2}(-[a-z]+)+-\d+$`
	// Region is the AWS Region
	Region string `json:"region"`

	// +optional
	Credentials MdaiDalAWSCredentials `json:"credentials,omitempty"`
}

type MdaiDalAWSCredentials struct {
	// +kubebuilder:default="aws-credentials"
	// +kubebuilder:validation:MinLength=1
	// +optional
	// SecretName is the name of the kubernetes secret that holds
	// the AWS credentials
	SecretName string `json:"secretName,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="AWS_ACCESS_KEY_ID"
	// +optional
	// AccessKeyField is the key of the AWS credentials secret
	// that holds the value for `AWS_ACCESS_KEY_ID`
	AccessKeyField string `json:"accessKeyField,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:default="AWS_SECRET_ACCESS_KEY"
	// +optional
	// SecretKeyField is the key of the AWS credentials secret
	// that holds the value for `AWS_SECRET_ACCESS_KEY`
	SecretKeyField string `json:"secretKeyField,omitempty"`
}

type MdaiDalImageSpec struct {
	// Repository, e.g. public.ecr.aws/mydecisive/mdai-dal
	// +kubebuilder:default="public.ecr.aws/mydecisive/mdai-dal"
	Repository string `json:"repository,omitempty"`

	// Tag, e.g. v0.0.1
	// +kubebuilder:default="0.0.1"
	Tag string `json:"tag,omitempty"`

	// PullPolicy, e.g. IfNotPresent, Always, Never
	// +kubebuilder:default="IfNotPresent"
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	PullPolicy ImagePullPolicy `json:"pullPolicy,omitempty"`
}

// MdaiDalStatus defines the observed state of MdaiDal.
type MdaiDalStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last generation the controller processed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the desired number of replicas for the DAL deployment.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready replicas for the DAL deployment.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Endpoint is the in-cluster address for the DAL service.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// LastSyncedTime is the last time the controller successfully reconciled.
	// +optional
	LastSyncedTime metav1.Time `json:"lastSyncedTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// MdaiDal is the Schema for the mdaidals API
type MdaiDal struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of MdaiDal
	// +required
	Spec MdaiDalSpec `json:"spec"`

	// status defines the observed state of MdaiDal
	// +optional
	Status MdaiDalStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiDalList contains a list of MdaiDal
type MdaiDalList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MdaiDal `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&MdaiDal{}, &MdaiDalList{})
}

func (p ImagePullPolicy) ToK8s() corev1.PullPolicy {
	if p == "" {
		return corev1.PullIfNotPresent
	}
	return corev1.PullPolicy(p)
}
