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

type Variable struct {
	// +kubebuilder:validation:MinLength=0
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	Source VariableSource `json:"source"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=scalar;array
	Type VariableType `json:"type"`
	// +kubebuilder:validation:Optional
	Delimiter string `json:"delimiter,omitempty"`
}

type VariableSource struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=valkey
	Type VariableSourceType `json:"type"`
	// depending on the type some additional fields are needed
}

type AlertingRule struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Required
	AlertQuery intstr.IntOrString `json:"alert_query" yaml:"alert_query"`
	// Alerts are considered firing once they have been returned for this long.
	// +kubebuilder:validation:Optional
	For *prometheusv1.Duration `json:"for,omitempty" yaml:"for,omitempty"`
	// +kubebuilder:validation:Pattern:="^(warning|critical)$"
	Severity string `json:"severity" yaml:"severity"`
	// +kubebuilder:validation:Required
	Action string `json:"action" yaml:"action"`
}

type Evaluation struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	EvaluationType EvaluationType `json:"evaluationType"` // prometheus
	// +kubebuilder:validation:Optional
	AlertingRules *[]AlertingRule `json:"alertingRules"`
}

type Observer struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"` // TODO: define the kind of observer (datalyzer)
}

// MdaiHubSpec defines the desired state of MdaiHub.
type MdaiHubSpec struct {
	Variables   *[]Variable   `json:"variables,omitempty"`
	Observers   *[]Observer   `json:"observers,omitempty"`   // watchers configuration (datalyzer)
	Evaluations *[]Evaluation `json:"evaluations,omitempty"` // evaluations configuration (alerting rules)
	Events      *[]Event      `json:"events,omitempty"`      // events configuration (update variables through api and operator)
}

type Event struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"` // TODO: define the kind of event (update variables through api and operator)
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

type EvaluationType string
type VariableSourceType string
type VariableType string

const (
	EvaluationTypePrometheus EvaluationType     = "prometheus"
	VariableSourceTypeValkey VariableSourceType = "valkey"
	VariableTypeScalar       VariableType       = "scalar"
	VariableTypeArray        VariableType       = "array"
)
