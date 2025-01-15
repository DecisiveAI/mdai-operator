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
	"k8s.io/apimachinery/pkg/util/intstr"
)

//
//type Conversion struct {
//	Type      VariableType               `json:"type"`
//	Function  VariableConversionFunction `json:"function"`
//	Arguments map[string]string          `json:"arguments"`
//}
//
//type Variable struct {
//	// +kubebuilder:validation:MinLength=0
//	Name string `json:"name"`
//	// +kubebuilder:validation:Required
//	// +kubebuilder:validation:Enum:=int;float;boolean;string;set;array
//	Type VariableType `json:"type"`
//	// +kubebuilder:validation:Optional
//	Conversions []Conversion `json:"conversions,omitempty"`
//}

// EvaluationFuture would be renamed Evaluation and replace the current version
type EvaluationFuture interface {
	getEvaluationType() string
}

type PrometheusEvaluation struct {
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

func (p *PrometheusEvaluation) getEvaluationType() string {
	return "mdai/prometheus_alert"
}

type VariableEvaluation struct {
	// How this evaluation will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// Effectively a logical expression that's applied to the variable's value. Format needs to be determined.
	// something like `var > 17` or `var == 'foo'` maybe? Could end up being a struct, too. with logicalOperator and comparator values.
	// +kubebuilder:validation:Required
	Expression string `json:"expression" yaml:"expression"`
}

func (v *VariableEvaluation) getEvaluationType() string {
	return "mdai/variable_evaluation"
}

// TriggerFuture would be renamed Trigger and replace the current version
type TriggerFuture interface {
	getTriggerType() string
}

type PrometheusEvaluationTrigger struct {
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

func (p *PrometheusEvaluationTrigger) getTriggerType() string {
	return "mdai/prometheus_alert"
}

type VariableUpdateTrigger struct {
	// How this Trigger will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Required
	VariableName string `json:"variableName" yaml:"variableName"`
	// UpdateOperation could also maybe be an enum. it would be the operation to watch for.
	// So this Trigger could happen on any update to variable "Foo" or just when that variable is changed
	// with an "add" operation
	// +kubebuilder:validation:Optional
	UpdateOperation string `json:"updateOperation" yaml:"updateOperation"`
}

func (p *VariableUpdateTrigger) getTriggerType() string {
	return "mdai/variable_update"
}

type VariableEvaluationTrigger struct {
	// How this Trigger will be referred to elsewhere in the config
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Required
	VariableName string `json:"variableName" yaml:"variableName"`
	// +kubebuilder:validation:Required
	VariableEvaluationName string `json:"variableEvaluationName" yaml:"variableEvaluationName"`
}

func (v *VariableEvaluationTrigger) getTriggerType() string {
	return "mdai/variable_evaluation"
}

// ActionFuture would be renamed Action and replace the current version of Action
type ActionFuture interface {
	getActionType() string
}

type VariableUpdateAction struct {
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

func (p *VariableUpdateAction) getActionType() string {
	return "mdai/variable_update"
}
