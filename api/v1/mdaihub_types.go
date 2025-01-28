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

type VariableWith struct {
	// ExportedVariableName The environment variable name to be used to access the variable's value.
	// +kubeuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubeuilder:validation:Required
	ExportedVariableName string `json:"exportedVariableName" yaml:"exportedVariableName"`
	// Transformer The transformation applied to the value of the variable before it is assigned as an environment variable.
	// +kubebuilder:validation:Optional
	Transformer *VariableTransformer `json:"transformer,omitempty" yaml:"transformer,omitempty"`
}

type JoinFunction struct {
	// Delimiter The delimiter inserted between each item in the collection during the Join
	// +kubebuilder:validation:Required
	Delimiter string `json:"delimiter" yaml:"delimiter"`
}

type VariableTransformer struct {
	// Join For use with "set" or "array" type variables, joins the items of the collection into a string.
	// +kubebuilder:validation:Optional
	Join *JoinFunction `json:"join,omitempty" yaml:"join,omitempty"`
}

type Variable struct {
	// StorageKey The key for which this variable's managed value is assigned. Will also be used as the environment variable name for variables of type "string"
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	StorageKey string `json:"storageKey" yaml:"storageKey"`
	// Type Data type for the managed variable value
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=int;float;boolean;string;set;array
	Type VariableType `json:"type" yaml:"type"`
	// StorageType defaults to "mdai-valkey" if not provided
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="mdai-valkey"
	// +kubebuilder:validation:Enum:=mdai-valkey
	StorageType *VariableStorageType `json:"storageType" yaml:"storageType"`
	// DefaultValue The initial value when the variable is instantiated. If not provided, a "zero value" of the variable's Type will be used.
	// +kubebuilder:validation:Optional
	DefaultValue *string `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`
	// +kubebuilder:validation:Optional
	With *[]VariableWith `json:"with,omitempty" yaml:"with,omitempty"`
}

type VariableUpdate struct {
	// VariableRef The StorageKey of the variable to be updated.
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	VariableRef string `json:"variableRef" yaml:"variableRef"`
	// Operation how the variable will be updated
	// +kubebuilder:validation:Enum:=mdai/add_element;mdai/remove_element
	Operation string `json:"operation" yaml:"operation"`
}

type Action struct {
	// VariableUpdate Modify the value of a managed variable.
	// +kubebuilder:validation:Optional
	VariableUpdate *VariableUpdate `json:"variableUpdate" yaml:"variableUpdate"`
}

type PrometheusAlertEvaluationStatus struct {
	// Firing Action performed when the Prometheus Alert status changes to "firing"
	// +kubebuilder:validation:Optional
	Firing *Action `json:"firing" yaml:"firing"`
	// Resolved Action performed when the Prometheus Alert status changes to "resolved"
	// +kubebuilder:validation:Optional
	Resolved *Action `json:"resolved" yaml:"resolved"`
}

type Evaluation struct {
	// Name How this evaluation will be referred to elsewhere in the config. Also, the name applied to the Prometheus Alert
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Enum:=mdai/prometheus_alert
	// +kubebuilder:validation:Required
	Type string `json:"type" yaml:"type"`
	// Expr A valid PromQL query expression
	// +kubebuilder:validation:Required
	Expr intstr.IntOrString `json:"expr" yaml:"expr"`
	// For Alerts are considered firing once they have been returned for this long.
	// +kubebuilder:validation:Optional
	For *prometheusv1.Duration `json:"for,omitempty" yaml:"for,omitempty"`
	// KeepFiringFor defines how long an alert will continue firing after the condition that triggered it has cleared.
	// +kubebuilder:validation:Optional
	KeepFiringFor *prometheusv1.NonEmptyDuration `json:"keep_firing_for,omitempty" yaml:"keep_firing_for,omitempty"`
	// +kubebuilder:validation:Pattern:="^(warning|critical)$"
	// +kubebuilder:validation:Required
	Severity string `json:"severity" yaml:"severity"`
	// RelevantLabels indicates which part(s) of the alert payload to forward to the Action.
	// +kubebuilder:validation:Optional
	RelevantLabels *[]string `json:"relevantLabels,omitempty" yaml:"relevantLabels,omitempty"`
	// OnStatus allows the user to specify actions depending on the state of the evaluation
	// +kubebuilder:validation:Optional
	OnStatus *PrometheusAlertEvaluationStatus `json:"onStatus,omitempty" yaml:"onStatus,omitempty"`
	// Specify the interval at which this evaluation is assessed in the Prometheus infrastructure.
	// Interval Specify how often this evaluation is assessed in the Prometheus infrastructure.
	// +kubebuilder:validation:Format:=duration
	// +kubebuilder:validation:Optional
	Interval *metav1.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
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

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Optional
	Image *string `json:"image" yaml:"image"`
	// +kubebuilder:validation:Required
	LabelResourceAttributes []string `json:"labelResourceAttributes" yaml:"labelResourceAttributes"`
	// +kubebuilder:validation:Optional
	CountMetricName *string `json:"countMetricName,omitempty" yaml:"countMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	BytesMetricName *string `json:"bytesMetricName,omitempty" yaml:"bytesMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	Filter *ObserverFilter `json:"filter,omitempty" yaml:"filter,omitempty"`
}

type Config struct {
	// Interval at which to reconcile the Cluster Configuration, applied only if built-in ValKey is enabled and variables defined for Valkey storage.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="2m"
	// +kubebuilder:validation:Format:=duration
	ReconcileLoopInterval *metav1.Duration `json:"reconcileLoopInterval,omitempty" yaml:"reconcileLoopInterval,omitempty"`
	// EvaluationInterval Specify the interval at which all evaluations are assessed in the Prometheus infrastructure.
	// Evaluations with explicit `Interval`s will override this value
	// +kubebuilder:validation:Optional
	EvaluationInterval prometheusv1.Duration `json:"evaluation_interval,omitempty" yaml:"evaluation_interval,omitempty"`
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

type (
	ActionName          string
	TriggerName         string
	EvaluationName      string
	TriggerType         string
	VariableSourceType  string
	VariableStorageType string
	VariableType        string
	VariableTransform   string
)

const (
	TriggerTypePrometheus          TriggerType         = "prometheus"
	VariableSourceTypeBultInValkey VariableStorageType = "mdai-valkey"
	VariableTypeInt                VariableType        = "int"
	VariableTypeFloat              VariableType        = "float"
	VariableTypeBoolean            VariableType        = "boolean"
	VariableTypeString             VariableType        = "string"
	VariableTypeSet                VariableType        = "set"
)
