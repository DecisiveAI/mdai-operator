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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Serializer struct {
	// Name The environment variable name to be used to access the variable's value.
	// +kubeuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubeuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// Transformers The transformation applied to the value of the variable in the order provided before it is assigned
	// as an environment variable.
	// +kubebuilder:validation:Optional
	Transformers []VariableTransformer `json:"transformers,omitempty" yaml:"transformers,omitempty"`
}

type VariableTransformer struct {
	Type TransformerType `json:"type" yaml:"type"`
	// Join For use with "set" or "array" type variables, joins the items of the collection into a string.
	// +kubebuilder:validation:Optional
	Join *JoinTransformer `json:"join,omitempty" yaml:"join,omitempty"`
}

type JoinTransformer struct {
	// Delimiter The delimiter inserted between each item in the collection during the Join
	// +kubebuilder:validation:Required
	Delimiter string `json:"delimiter" yaml:"delimiter"`
}

type Variable struct {
	// StorageKey The key for which this variable's managed value is assigned. Will also be used as the environment variable name for variables of type "string"
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	StorageKey string `json:"storageKey" yaml:"storageKey"`
	// Type for the variable, defaults to "computed" if not provided
	// +kubebuilder:validation:Required
	// +kubebuilder:default="computed"
	// +kubebuilder:validation:Enum:=manual;computed
	Type VariableType `json:"type" yaml:"type"`
	// DataType Data type for the managed variable value
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=string;int;set;map;boolean
	DataType VariableDataType `json:"dataType" yaml:"dataType"`
	// StorageType defaults to "mdai-valkey" if not provided
	// +kubebuilder:default="mdai-valkey"
	// +kubebuilder:validation:Enum:=mdai-valkey
	StorageType VariableStorageType `json:"storageType" yaml:"storageType"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	SerializeAs []Serializer `json:"serializeAs" yaml:"serializeAs"`
}

type VariableUpdate struct {
	// VariableRef The StorageKey of the variable to be updated.
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	VariableRef string `json:"variableRef" yaml:"variableRef"`
	// Operation how the variable will be updated
	// +kubebuilder:validation:Enum:=mdai/add_element;mdai/remove_element;mdai/set;mdai/delete;mdai/incr_by;mdai/decr_by;mdai/map_set_entry;mdai/map_remove_entry;mdai/bulk_set_key_value
	// +kubebuilder:validation:Required
	Operation VariableUpdateOperation `json:"operation" yaml:"operation"`
}

type Action struct {
	// VariableUpdate Modify the value of a managed variable.
	// +kubebuilder:validation:Optional
	VariableUpdate *VariableUpdate `json:"variableUpdate,omitempty" yaml:"variableUpdate,omitempty"`
}

type PrometheusAlertEvaluationStatus struct {
	// Firing Action performed when the Prometheus Alert status changes to "firing"
	// +kubebuilder:validation:Optional
	Firing *Action `json:"firing,omitempty" yaml:"firing,omitempty"`
	// Resolved Action performed when the Prometheus Alert status changes to "resolved"
	// +kubebuilder:validation:Optional
	Resolved *Action `json:"resolved,omitempty" yaml:"resolved,omitempty"`
}

type Evaluation struct {
	// Name How this evaluation will be referred to elsewhere in the config. Also, the name applied to the Prometheus Alert
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// Type The type of evaluation. Currently only "mdai/prometheus_alert" is supported and set as default value.
	// +kubebuilder:validation:Enum:=mdai/prometheus_alert
	// +kubebuilder:validation:Required
	// +kubebuilder:default="mdai/prometheus_alert"
	Type EvaluationType `json:"type" yaml:"type"`
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
	// +kubebuilder:validation:Required
	ResourceRef string `json:"resourceRef" yaml:"resourceRef"`
	// +kubebuilder:validation:Required
	LabelResourceAttributes []string `json:"labelResourceAttributes" yaml:"labelResourceAttributes"`
	// +kubebuilder:validation:Optional
	CountMetricName *string `json:"countMetricName,omitempty" yaml:"countMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	BytesMetricName *string `json:"bytesMetricName,omitempty" yaml:"bytesMetricName,omitempty"`
	// +kubebuilder:validation:Optional
	Filter *ObserverFilter `json:"filter,omitempty" yaml:"filter,omitempty"`
}

type ObserverResource struct {
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
	// +kubebuilder:validation:Optional
	Image *string `json:"image,omitempty" yaml:"image,omitempty"`
	// +kubebuilder:validation:Optional
	Replicas *int32 `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	// +kubebuilder:validation:Optional
	Resources *v1.ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	GrpcReceiverMaxMsgSize *uint64 `json:"grpcReceiverMaxMsgSize,omitempty" yaml:"grpcReceiverMaxMsgSize,omitempty"`
}

type Config struct {
	// EvaluationInterval Specify the interval at which all evaluations within this hub are assessed in the Prometheus infrastructure.
	// +kubebuilder:validation:Optional
	EvaluationInterval *prometheusv1.Duration `json:"evaluation_interval,omitempty" yaml:"evaluation_interval,omitempty"`
}

// MdaiHubSpec defines the desired state of MdaiHub.
type MdaiHubSpec struct {
	// kubebuilder:validation:Optional
	Config            *Config             `json:"config,omitempty" yaml:"config,omitempty"`
	Observers         *[]Observer         `json:"observers,omitempty" yaml:"observers,omitempty"`
	ObserverResources *[]ObserverResource `json:"observerResources,omitempty" yaml:"observerResources,omitempty"`
	Variables         *[]Variable         `json:"variables,omitempty"`
	Evaluations       *[]Evaluation       `json:"evaluations,omitempty"` // evaluation configuration (alerting rules)
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
	VariableSourceType      string
	VariableStorageType     string
	VariableType            string
	VariableDataType        string
	VariableTransform       string
	EvaluationType          string
	VariableUpdateOperation string
	ObserverResourceType    string
	TransformerType         string
)

const (
	VariableSourceTypeBuiltInValkey VariableStorageType = "mdai-valkey"

	VariableTypeManual   VariableType = "manual"
	VariableTypeComputed VariableType = "computed"
	VariableTypeMeta     VariableType = "meta" // meta variable disabled for now

	VariableDataTypeInt     VariableDataType = "int"     // internally stored in string
	VariableDataTypeFloat   VariableDataType = "float"   // internally stored in string, disabled for now
	VariableDataTypeBoolean VariableDataType = "boolean" // 0 or 1 as string in valkey
	VariableDataTypeString  VariableDataType = "string"
	VariableDataTypeSet     VariableDataType = "set" // valkey set
	VariableDataTypeMap     VariableDataType = "map" // valkey hashes

	// MetaVariableTypeHashSet LookupTable takes an input/key variable and a lookup variable, disabled for now
	MetaVariableTypeHashSet VariableDataType = "hashSet"
	// MetaVariableTypePriorityList takes a list of variable refs, and will evaluate to the first one that is not empty.
	// disabled for now
	MetaVariableTypePriorityList VariableDataType = "priorityList"

	EvaluationTypePrometheusAlert EvaluationType = "mdai/prometheus_alert"

	VariableUpdateSetAddElement    VariableUpdateOperation = "mdai/add_element"
	VariableUpdateSetRemoveElement VariableUpdateOperation = "mdai/remove_element"
	VariableUpdateSet              VariableUpdateOperation = "mdai/set"
	VariableUpdateDelete           VariableUpdateOperation = "mdai/delete"
	VariableUpdateIntIncrBy        VariableUpdateOperation = "mdai/incr_by"
	VariableUpdateIntDecrBy        VariableUpdateOperation = "mdai/decr_by"
	VariableUpdateSetMapEntry      VariableUpdateOperation = "mdai/map_set_entry"
	VariableUpdateRemoveMapEntry   VariableUpdateOperation = "mdai/map_remove_entry"
	VariableUpdateBulkSetKeyValue  VariableUpdateOperation = "mdai/bulk_set_key_value"

	TransformerTypeJoin TransformerType = "join"

	ObserverResourceTypeWatcherCollector ObserverResourceType = "mdai-watcher"
)
