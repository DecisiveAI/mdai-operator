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

type Conversion struct {
	Type      VariableType               `json:"type"`
	Function  VariableConversionFunction `json:"function"`
	Arguments map[string]string          `json:"arguments"`
}

type Variable struct {
	// +kubebuilder:validation:MinLength=0
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=int;float;boolean;string;set;array
	Type VariableType `json:"type"`
	// +kubebuilder:validation:Optional
	Conversions []Conversion `json:"conversions,omitempty"`
}

type Evaluation struct {
	// How this evaluation will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
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
}

type Trigger struct {
	// How this Trigger will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// The name of the evaluation that you want to key off of
	// +kubebuilder:validation:Required
	EvaluationName string `json:"evaluationName" yaml:"evaluationName"`
	// Does this evaluation kick off an action on 'firing' status or 'resolved'? If omitted, the evaluation will only trigger on the 'firing' status.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern:="^(firing|resolved)$"
	AlertStatus string `json:"alertStatus,omitempty" yaml:"alertStatus,omitempty"`
	// Label from the Expr indicating which value(s) the alert payload will forward. Helpful for dictating downstream behavior.
	// +kubebuilder:validation:Optional
	RelevantLabels []string `json:"relevantLabels,omitempty" yaml:"relevantLabels,omitempty"`
}

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Image  string            `json:"image"`
	Config map[string]string `json:"config,omitempty"`
}

// MdaiHubSpec defines the desired state of MdaiHub.
type MdaiHubSpec struct {
	Variables   *[]Variable   `json:"variables,omitempty"`
	Observers   *[]Observer   `json:"observers,omitempty"`   // watchers configuration (datalyzer)
	Evaluations *[]Evaluation `json:"evaluations,omitempty"` // evaluation configuration (alerting rules)
	Triggers    *[]Trigger    `json:"triggers,omitempty"`    // triggers configuration (alert assessment)
	Actions     *[]Action     `json:"actions,omitempty"`     // events configuration (update variables through api and operator)
	Platform    *Platform     `json:"platform,omitempty"`    // declare the behavior of the constructs
}

// Platform is where a user will define the behavior of the constructs that they have configured
type Platform struct {
	// EventMap Keys should be names of Triggers
	// Values should be arrays of name of Actions
	EventMap *map[string]*[]string `json:"eventMap,omitempty"`
}

type Action struct {
	// How this Action will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// Values depend on the type of the variable named. To be expanded.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern:="^(mdai/(add|remove))$"
	Operation string `json:"operation" yaml:"operation"`
	// The name of the variable that this Action will affect
	// +kubebuilder:validation:Required
	VariableName string `json:"variableName" yaml:"variableName"`
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

type TriggerType string
type VariableSourceType string
type VariableType string
type VariableConversionFunction string

const (
	TriggerTypePrometheus    TriggerType                = "prometheus"
	VariableSourceTypeValkey VariableSourceType         = "valkey"
	VariableTypeInt          VariableType               = "int"
	VariableTypeFloat        VariableType               = "float"
	VariableTypeBoolean      VariableType               = "boolean"
	VariableTypeString       VariableType               = "string"
	VariableTypeSet          VariableType               = "set"
	VariableTypeArray        VariableType               = "array"
	VariableConversionJoin   VariableConversionFunction = "join"
)
