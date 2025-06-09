/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Required
	LabelResourceAttributes []string `json:"labelResourceAttributes" yaml:"labelResourceAttributes"`
	// +kubebuilder:validation:Optional
	CountMetricName *string `json:"countMetricName,omitempty" yaml:"countMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	BytesMetricName *string `json:"bytesMetricName,omitempty" yaml:"bytesMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	Filter *ObserverFilter `json:"filter,omitempty" yaml:"filter,omitempty"`
}

type ObserverLogsFilter struct {
	// +kubebuilder:validation:Required
	LogRecord []string `json:"log_record" yaml:"log_record"`
}

type ObserverFilter struct {
	// +kubebuilder:validation:Optional
	ErrorMode *string `json:"error_mode" yaml:"error_mode"`
	// +kubebuilder:validation:Optional
	Logs *ObserverLogsFilter `json:"logs" yaml:"logs"`
}

// TODO: Add metrics and trace filters

type ObserverResource struct {
	// +kubebuilder:validation:Optional
	Image *string `json:"image,omitempty" yaml:"image,omitempty"`
	// +kubebuilder:validation:Optional
	Replicas *int32 `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	// +kubebuilder:validation:Optional
	Resources *v1.ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	GrpcReceiverMaxMsgSize *uint64 `json:"grpcReceiverMaxMsgSize,omitempty" yaml:"grpcReceiverMaxMsgSize,omitempty"`
	// +kubebuilder:validation:Optional
	OwnLogsOtlpEndpoint *string `json:"ownLogsOtlpEndpoint,omitempty" yaml:"ownLogsOtlpEndpoint,omitempty"`
}

// MdaiObserverSpec defines the desired state of MdaiObserver.
type MdaiObserverSpec struct {
	// +optional
	Observers []Observer `json:"observers,omitempty" yaml:"observers,omitempty"`
	// +optional
	ObserverResource ObserverResource `json:"observerResource,omitempty" yaml:"observerResource,omitempty"`
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
	Items           []MdaiObserver `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiObserver{}, &MdaiObserverList{})
}
