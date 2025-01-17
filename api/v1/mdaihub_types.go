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
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type VariableOverride struct {
	// StorageType defaults to "mdai-valkey" if not provided
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:=mdai-valkey
	StorageType VariableStorageType `json:"storageType" yaml:"storageType"`
	// +kubebuilder:validation:Optional
	DefaultValue *string `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`
	// +kubeuilder:validation:Required
	ExportedVariableName string `json:"exportedVariableName,omitempty" yaml:"exportedVariableName,omitempty"`
	// +kubebuilder:validation:Optional
	Serialize string `json:"serialize,omitempty" yaml:"serialize,omitempty"`
}

type Variable struct {
	// +kubebuilder:validation:Required
	StorageKey VariableName `json:"storageKey" yaml:"storageKey"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=int;float;boolean;string;set;array
	Type VariableType `json:"type" yaml:"type"`
	// StorageType defaults to "mdai-valkey" if not provided
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:=mdai-valkey
	StorageType VariableStorageType `json:"storageType" yaml:"storageType"`
	// +kubebuilder:validation:Optional
	DefaultValue *string `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`
	// +kubeuilder:validation:Optional
	ExportedVariableName string `json:"exportedVariableName,omitempty" yaml:"exportedVariableName,omitempty"`
	// +kubebuilder:validation:Optional
	Serialize string `json:"serialize,omitempty" yaml:"serialize,omitempty"`
	// If ExportedVariableName is not present, at least one VariableOverride should be declared
	// +kubebuilder:validation:Optional
	With []VariableOverride `json:"with,omitempty" yaml:"with,omitempty"`
}

type Action struct {
	// +kubebuilder:validation:Enum:=mdai/variable_update
	Type string `json:"type" yaml:"type"`
	// +kubebuilder:validation:Enum:=mdai/add;mdai/remove
	Operation string `json:"operation" yaml:"operation"`
}

type PrometheusAlertEvaluationResolveStatus struct {
	// +kubebuilder:validation:Optional
	Firing Action `json:"firing" yaml:"firing"`
	// +kubebuilder:validation:Optional
	Resolved Action `json:"resolved" yaml:"resolved"`
}

type Evaluation struct {
	// How this evaluation will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name EvaluationName `json:"name" yaml:"name"`
	// +kubebuilder:validation:Enum:=mdai/prometheus_alert
	Type string `json:"type" yaml:"type"`
	// A valid PromQL query expression
	// +kubebuilder:validation:Required
	Expr intstr.IntOrString `json:"expr" yaml:"expr"`
	// Alerts are considered firing once they have been returned for this long.
	// +kubebuilder:validation:Optional
	For *prometheusv1.Duration `json:"for,omitempty" yaml:"for,omitempty"`
	// KeepFiringFor defines how long an alert will continue firing after the condition that triggered it has cleared.
	// +kubebuilder:validation:Optional
	KeepFiringFor *prometheusv1.NonEmptyDuration `json:"keep_firing_for,omitempty" yaml:"keep_firing_for,omitempty"`
	// +kubebuilder:validation:Pattern:="^(warning|critical)$"
	Severity string `json:"severity" yaml:"severity"`
	// RelevantLabels indicates which part(s) of the alert payload to forward to the Action.
	// +kubebuilder:validation:Optional
	RelevantLabels []string `json:"relevantLabels" yaml:"relevantLabels"`
	// Resolve is the action that's taken when this alert fires. It's a shorthand for ResolvedStatus.Firing
	// +kubebuilder:validation:Optional
	Resolve Action `json:"resolve" yaml:"resolve"`
	// ResolveStatus allows the user to specify actions depending on the state of the evaluation
	// If Resolve is not provided, ResolveStatus.Firing is required
	// If both are provided, ResolveStatus will override Resolve
	// +kubebuilder:validation:Optional
	ResolvedStatus PrometheusAlertEvaluationResolveStatus `json:"resolvedStatus" yaml:"resolvedStatus"`
}

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Optional
	Image  string            `json:"image" yaml:"image"`
	Config map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
}

type Config struct {
	// Interval at which to reconcile the Cluster Configuration, applied only if built-in ValKey is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="2m"
	// +kubebuilder:validation:Format:=duration
	ReconcileLoopInterval *metav1.Duration `json:"reconcileLoopInterval,omitempty" yaml:"reconcileLoopInterval,omitempty"`
}

// MdaiHubSpec defines the desired state of MdaiHub.
type MdaiHubSpec struct {
	// kubebuilder:validation:Optional
	Config      *Config       `json:"config,omitempty" yaml:"config,omitempty"`
	Observers   *[]Observer   `json:"observers,omitempty" yaml:"observers,omitempty"`
	Variables   *[]Variable   `json:"variables,omitempty"`
	Evaluations *[]Evaluation `json:"evaluations,omitempty"` // evaluation configuration (alerting rules)
}

// MdaiHubStatus defines the observed state of MdaiHub.
type MdaiHubStatus struct {
	// Status of the Cluster defined by its modules and dependencies statuses
	HubStatus string `json:"hubStatus"`

	// Time when last Cluster Configuration change was detected
	// Right now it's updated on each reconcile, we have to skip when reconciliation detects no changes
	// +optional
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Conditions store the status conditions of the Cluster instances
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MdaiHub is the Schema for the mdaihubs API.
type MdaiHub struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiHubSpec   `json:"spec,omitempty"`
	Status MdaiHubStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiHubList contains a list of MdaiHub.
type MdaiHubList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MdaiHub `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiHub{}, &MdaiHubList{})
}

type ActionName string
type TriggerName string
type EvaluationName string
type VariableName string

type TriggerType string
type VariableSourceType string
type VariableStorageType string
type VariableType string
type VariableTransform string

const (
	TriggerTypePrometheus          TriggerType         = "prometheus"
	VariableSourceTypeBultInValkey VariableStorageType = "mdai-valkey"
	VariableTypeInt                VariableType        = "int"
	VariableTypeFloat              VariableType        = "float"
	VariableTypeBoolean            VariableType        = "boolean"
	VariableTypeString             VariableType        = "string"
	VariableTypeSet                VariableType        = "set"
	VariableTypeArray              VariableType        = "array"
	VariableTransformJoin          VariableTransform   = "join"
	VariableTypeScalar             VariableType        = "scalar"
)
