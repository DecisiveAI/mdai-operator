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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type AWSConfig struct {
	AWSAccessKeySecret *string `json:"awsAccessKeySecret,omitempty" yaml:"awsAccessKeySecret,omitempty"`
}

type S3LogsConfig struct {
	// TODO: Implement this and figure out a good way to marshal OTEL LOG severity level number for gt/lt in OTTL
	// For now will be coded to WARN or greater
	// LogLevel *string `json:"logLevel,omitempty" yaml:"logLevel,omitempty"`
	S3Region *string `json:"s3Region,omitempty" yaml:"s3Region,omitempty"`
	S3Bucket *string `json:"s3Bucket,omitempty" yaml:"s3Bucket,omitempty"`
}

type LogsConfig struct {
	S3 *S3LogsConfig `json:"s3,omitempty" yaml:"s3,omitempty"`
}

// MdaiCollectorSpec defines the desired state of MdaiCollector.
type MdaiCollectorSpec struct {
	AWSConfig *AWSConfig  `json:"aws,omitempty" yaml:"aws,omitempty"`
	Logs      *LogsConfig `json:"logs,omitempty" yaml:"logs,omitempty"`
}

// MdaiCollectorStatus defines the observed state of MdaiCollector.
type MdaiCollectorStatus struct {
	// Time when last MDAI Collector Configuration change was detected
	// Right now it's updated on each reconcile, we have to skip when reconciliation detects no changes
	// +optional
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Conditions store the status conditions of the MDAI Collector instances
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MdaiCollector is the Schema for the mdaicollectors API.
type MdaiCollector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiCollectorSpec   `json:"spec,omitempty"`
	Status MdaiCollectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiCollectorList contains a list of MdaiCollector.
type MdaiCollectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MdaiCollector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiCollector{}, &MdaiCollectorList{})
}
