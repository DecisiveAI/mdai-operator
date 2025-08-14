package v1

import (
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type Serializer struct {
	// Name The environment variable name to be used to access the variable's value.
	// +kubeuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// + required
	Name string `json:"name" yaml:"name"`
	// Transformers The transformation applied to the value of the variable in the order provided before it is assigned
	// as an environment variable.
	// +optional
	Transformers []VariableTransformer `json:"transformers,omitempty" yaml:"transformers,omitempty"`
}

type VariableTransformer struct {
	Type TransformerType `json:"type" yaml:"type"`
	// Join For use with "set" or "array" type variables, joins the items of the collection into a string.
	// +optional
	Join *JoinTransformer `json:"join,omitempty" yaml:"join,omitempty"`
}

type JoinTransformer struct {
	// Delimiter The delimiter inserted between each item in the collection during the Join
	// +kubebuilder:validation:Required
	Delimiter string `json:"delimiter" yaml:"delimiter"`
}

// Variable defines mdai variable
// +kubebuilder:validation:XValidation:rule="self.type == 'meta' ? self.dataType in ['metaHashSet', 'metaPriorityList'] : self.dataType in ['string', 'int', 'boolean', 'set', 'map']",messageExpression="\"variable '\" + self.key + \"': dataType is not allowed for type specified\"",reason="FieldValueInvalid"
type Variable struct {
	// Key The key for which this variable's managed value is assigned. Will also be used as the environment variable name for variables of type "string"
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Required
	Key string `json:"key" yaml:"key"`
	// Type for the variable, defaults to "computed" if not provided
	// +kubebuilder:validation:Required
	// +kubebuilder:default="computed"
	// +kubebuilder:validation:Enum:=manual;computed;meta
	Type VariableType `json:"type" yaml:"type"`
	// DataType Data type for the managed variable value
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:=string;int;set;map;boolean;metaHashSet;metaPriorityList
	DataType VariableDataType `json:"dataType" yaml:"dataType"`
	// StorageType defaults to "mdai-valkey" if not provided
	// +kubebuilder:default="mdai-valkey"
	// +kubebuilder:validation:Enum:=mdai-valkey
	StorageType VariableStorageType `json:"storageType" yaml:"storageType"`
	// VariableRefs name references to other managed variables to be included in meta calculation. Listed variables should be of the same data type.
	// +kubebuilder:validation:Optional
	VariableRefs []string `json:"variableRefs,omitempty" yaml:"variableRefs,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	SerializeAs []Serializer `json:"serializeAs" yaml:"serializeAs"`
}

type PrometheusAlert struct {
	// Name How this evaluation will be referred to elsewhere in the config. Also, the name applied to the Prometheus Alert
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name" yaml:"name"`
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
}

type Config struct {
	// EvaluationInterval Specify the interval at which all evaluations within this hub are assessed in the Prometheus infrastructure.
	// +kubebuilder:validation:Optional
	EvaluationInterval *prometheusv1.Duration `json:"evaluation_interval,omitempty" yaml:"evaluation_interval,omitempty"`
}

type AutomationRule struct {
	// Name How this rule will be referred to elsewhere in the config and audit.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// When specifies the conditions under which the rule is triggered.
	// +kubebuilder:validation:Required
	When When `json:"when"`
	// Then specifies the actions to be taken when the rule is triggered.
	// +kubebuilder:validation:Required
	Then []Action `json:"then"`
}

type Action struct {
	// Payloads (one required depending on Type)
	AddToSet      *SetAction `json:"addToSet,omitempty"`
	RemoveFromSet *SetAction `json:"removeFromSet,omitempty"`

	SetVariable *ScalarAction `json:"setVariable,omitempty"`

	// TODO add more actions to update variables here

	CallWebhook *CallWebhookAction `json:"callWebhook,omitempty"`
}

type SetAction struct {
	// Target set name
	// +kubebuilder:validation:MinLength=1
	Set string `json:"set"`
	// Value to add (templated string allowed)
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}

type ScalarAction struct {
	// Target set name
	// +kubebuilder:validation:MinLength=1
	Scalar string `json:"scalar"`
	// Value to add (templated string allowed)
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
}
type CallWebhookAction struct {
	// URL may be a template
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// +kubebuilder:default=POST
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH;DELETE
	Method string `json:"method,omitempty"`

	// Arbitrary JSON body (map/array/string/number), templates allowed in strings.
	Body *apiextensionsv1.JSON `json:"body,omitempty"`

	// Optional headers
	Headers map[string]string `json:"headers,omitempty"`
}

// When represents one of two trigger variants:
//
//  1. alertName + status
//  2. variableUpdated + condition
//
// Exactly one variant must be set. TODO check the validation logic for this.
// +kubebuilder:validation:XValidation:rule="(has(self.alertName) && has(self.status)) != (has(self.variableUpdated) && has(self.condition))",message="exactly one variant must be set"
type When struct {
	// Variant 1: alert
	// +optional
	AlertName *string `json:"alertName,omitempty"`
	// +optional
	Status *string `json:"status,omitempty"`

	// Variant 2: variable-driven
	// +optional
	VariableUpdated *string `json:"variableUpdated,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum=added;removed;set
	UpdateType *string `json:"updateType,omitempty"`
	// Condition a template for conditions on variable changes
	// +optional
	Condition *string `json:"condition,omitempty"`
}

// MdaiHubSpec defines the desired state of MdaiHub.
type MdaiHubSpec struct {
	// +optional
	Config *Config `json:"config,omitempty" yaml:"config,omitempty"`
	// +optional
	Variables []Variable `json:"variables,omitempty"`
	// +optional
	PrometheusAlert []PrometheusAlert `json:"prometheusAlert,omitempty"` // evaluation configuration (alerting rules)
	// +optional
	Automations []AutomationRule `json:"automations,omitempty"`
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
	VariableUpdateOperation string
	ObserverResourceType    string
	TransformerType         string
)

const (
	VariableSourceTypeBuiltInValkey VariableStorageType = "mdai-valkey"

	// VariableTypeManual Variable type that is managed externally by the user, not attached to any OODA loop
	VariableTypeManual VariableType = "manual"
	// VariableTypeComputed Variable type that is computed internally by MDAI, attached to an OODA loop
	VariableTypeComputed VariableType = "computed"
	// VariableTypeMeta Variable type that is derived from manual and computed variables
	VariableTypeMeta VariableType = "meta"

	// computed variable types
	VariableDataTypeInt     VariableDataType = "int"     // internally stored in string
	VariableDataTypeFloat   VariableDataType = "float"   // internally stored in string, disabled for now
	VariableDataTypeBoolean VariableDataType = "boolean" // 0 or 1 as string in valkey
	// VariableDataTypeString the string variable, could be used to store yaml or any other string
	VariableDataTypeString VariableDataType = "string"
	VariableDataTypeSet    VariableDataType = "set" // valkey set
	// VariableDataTypeMap implemented as hash map. Order is not guaranteed. Keys and values are strings.
	VariableDataTypeMap VariableDataType = "map" // valkey hashes

	// MetaVariableDataTypeHashSet LookupTable takes an input/key variable and a lookup variable. Returns a string.
	MetaVariableDataTypeHashSet VariableDataType = "metaHashSet"
	// MetaVariableDataTypePriorityList takes a list of variable refs, and will evaluate to the first one that is not empty. Returns an array of strings.
	MetaVariableDataTypePriorityList VariableDataType = "metaPriorityList"

	TransformerTypeJoin TransformerType = "join"
)
