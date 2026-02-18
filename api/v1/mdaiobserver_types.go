package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Required
	LabelResourceAttributes []string `json:"labelResourceAttributes" yaml:"labelResourceAttributes"`
	// +optional
	CountMetricName *string `json:"countMetricName,omitempty" yaml:"countMetricName,omitempty"`
	// +optional
	BytesMetricName *string `json:"bytesMetricName,omitempty" yaml:"bytesMetricName,omitempty"`
	// +optional
	Filter *ObserverFilter `json:"filter,omitempty" yaml:"filter,omitempty"`
}

type ObserverLogsFilter struct {
	// +kubebuilder:validation:Required
	LogRecord []string `json:"log_record" yaml:"log_record"` //nolint:tagliatelle
}

type ObserverFilter struct {
	// +optional
	ErrorMode *string `json:"error_mode" yaml:"error_mode"` //nolint:tagliatelle
	// +optional
	Logs *ObserverLogsFilter `json:"logs" yaml:"logs"`
}

// TODO: Add metrics and trace filters

type ObserverResource struct {
	// +kubebuilder:default="public.ecr.aws/mydecisive/observer-collector:0.1.6"
	// +optional
	Image string `json:"image,omitempty"`
	// +kubebuilder:default=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	GrpcReceiverMaxMsgSize *uint64 `json:"grpcReceiverMaxMsgSize,omitempty"`
	// +optional
	OwnLogsOtlpEndpoint *string `json:"ownLogsOtlpEndpoint,omitempty"`
	// +kubebuilder:default={}
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// MdaiObserverSpec defines the desired state of MdaiObserver.
type MdaiObserverSpec struct {
	// +optional
	Observers []Observer `json:"observers,omitempty"`
	// +optional
	ObserverResource ObserverResource `json:"observerResource,omitempty"`
}

// MdaiObserverStatus defines the observed state of MdaiObserver.
type MdaiObserverStatus struct {
	// Status of the Cluster defined by its modules and dependencies statuses
	ObserverStatus string `json:"observerStatus"`

	// +optional
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Conditions store the status conditions of the Cluster instances
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// MdaiObserver is the Schema for the mdaiobservers API.
type MdaiObserver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiObserverSpec   `json:"spec,omitempty"`
	Status MdaiObserverStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiObserverList contains a list of MdaiObserver.
type MdaiObserverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MdaiObserver `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&MdaiObserver{}, &MdaiObserverList{})
}
