package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type (
	MDAILogStream string
	SeverityLevel string
)

const (
	AuditLogstream     MDAILogStream = "audit"
	CollectorLogstream MDAILogStream = "collector"
	HubLogstream       MDAILogStream = "hub"
	OtherLogstream     MDAILogStream = "other"

	DebugSeverityLevel SeverityLevel = "debug"
	InfoSeverityLevel  SeverityLevel = "info"
	WarnSeverityLevel  SeverityLevel = "warn"
	ErrorSeverityLevel SeverityLevel = "error"
)

type AWSConfig struct {
	AWSAccessKeySecret *string `json:"awsAccessKeySecret,omitempty" yaml:"awsAccessKeySecret,omitempty"`
}

type AuditLogstreamConfig struct {
	// +optional
	Disabled bool `json:"disabled" yaml:"disabled"`
}

type LogstreamConfig struct {
	// +optional
	MinSeverity *SeverityLevel `json:"minSeverity,omitempty" yaml:"minSeverity,omitempty"`
	// +optional
	Disabled bool `json:"disabled" yaml:"disabled"`
}

type S3LogsConfig struct {
	// +optional
	AuditLogs *AuditLogstreamConfig `json:"auditLogs,omitempty" yaml:"auditLogs,omitempty"`
	// +optional
	CollectorLogs *LogstreamConfig `json:"collectorLogs,omitempty" yaml:"collectorLogs,omitempty"`
	// +optional
	HubLogs *LogstreamConfig `json:"hubLogs,omitempty" yaml:"hubLogs,omitempty"`
	// +optional
	OtherLogs *LogstreamConfig `json:"otherLogs,omitempty" yaml:"otherLogs,omitempty"`
	S3Region  string           `json:"s3Region" yaml:"s3Region"`
	S3Bucket  string           `json:"s3Bucket" yaml:"s3Bucket"`
}

type OtlpLogsConfig struct {
	// +optional
	AuditLogs *AuditLogstreamConfig `json:"auditLogs,omitempty" yaml:"auditLogs,omitempty"`
	// +optional
	CollectorLogs *LogstreamConfig `json:"collectorLogs,omitempty" yaml:"collectorLogs,omitempty"`
	// +optional
	HubLogs *LogstreamConfig `json:"hubLogs,omitempty" yaml:"hubLogs,omitempty"`
	// +optional
	OtherLogs *LogstreamConfig `json:"otherLogs,omitempty" yaml:"otherLogs,omitempty"`
	Endpoint  string           `json:"endpoint" yaml:"endpoint"`
	// TODO: Support TLS. Need integration w/ cert manager
	// TlsConfig *OtlpTlsConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

type LogsConfig struct {
	// +optional
	S3 *S3LogsConfig `json:"s3,omitempty" yaml:"s3,omitempty"`
	// +optional
	Otlp *OtlpLogsConfig `json:"otlp,omitempty" yaml:"otlp,omitempty"`
}

// MdaiCollectorSpec defines the desired state of MdaiCollector.
type MdaiCollectorSpec struct {
	// +optional
	AWSConfig *AWSConfig `json:"aws,omitempty"`
	// +optional
	Logs *LogsConfig `json:"logs,omitempty"`
	// +kubebuilder:default="public.ecr.aws/mydecisive/mdai-collector:0.1.6"
	// +optional
	Image string `json:"image,omitempty"`
	// +kubebuilder:default=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// +kubebuilder:default={}
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
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
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
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

	Items []MdaiCollector `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&MdaiCollector{}, &MdaiCollectorList{})
}
