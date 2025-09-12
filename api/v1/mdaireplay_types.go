package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type MdaiReplayTelemetryType string
type MdaiReplayS3Partition string

const (
	LogsReplayTelemetryType     MdaiReplayTelemetryType = "Logs"
	MetricssReplayTelemetryType MdaiReplayTelemetryType = "Metrics"
	TracesReplayTelemetryType   MdaiReplayTelemetryType = "Traces"

	S3ReplayMinutePartition MdaiReplayS3Partition = "Minute"
	S3ReplayHourPartition   MdaiReplayS3Partition = "Hour"
)

type MdaiReplayAwsConfig struct {
	AWSAccessKeySecret *string `json:"awsAccessKeySecret,omitempty" yaml:"aws_access_key_secret,omitempty"`
}

type MdaiReplayS3Configuration struct {
	S3Region    string                `json:"s3Region" yaml:"s3_region"`
	S3Bucket    string                `json:"s3Bucket" yaml:"s3_bucket"`
	FilePrefix  string                `json:"filePrefix" yaml:"file_prefix"`
	S3Partition MdaiReplayS3Partition `json:"s3Partition" yaml:"s3_partition"`
}

type MdaiReplaySourceConfiguration struct {
	// +optional
	AWSConfig *MdaiReplayAwsConfig `json:"aws,omitempty" yaml:"aws,omitempty"`
	// +optional
	S3 *MdaiReplayS3Configuration `json:"s3,omitempty" yaml:"s3,omitempty"`
}

type MdaiReplayOtlpHttpDestinationConfiguration struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}

type MdaiReplayDestinationConfiguration struct {
	// +optional
	OtlpHttp *MdaiReplayOtlpHttpDestinationConfiguration `json:"otlpHttp,omitempty" yaml:"otlp_http,omitempty"`
}

// MdaiReplaySpec defines the desired state of MdaiReplay.
type MdaiReplaySpec struct {
	StartTime     *metav1.Time                       `json:"startTime,omitempty" yaml:"start_time,omitempty"`
	EndTime       *metav1.Time                       `json:"endTime,omitempty" yaml:"end_time,omitempty"`
	TelemetryType MdaiReplayTelemetryType            `json:"telemetryType,omitempty" yaml:"telemetry_type,omitempty"`
	Source        MdaiReplaySourceConfiguration      `json:"source,omitempty" yaml:"source,omitempty"`
	Destination   MdaiReplayDestinationConfiguration `json:"destination,omitempty" yaml:"destination,omitempty"`
}

type MdaiReplayStatusType string

const (
	MdaiReplayStatusTypeStarted   MdaiReplayStatusType = "Started"
	MdaiReplayStatusTypeCompleted MdaiReplayStatusType = "Completed"
	MdaiReplayStatusTypeFailed    MdaiReplayStatusType = "Failed"
)

// MdaiReplayStatus defines the observed state of MdaiReplay.
type MdaiReplayStatus struct {
	ReplayStatus MdaiReplayStatusType `json:"replayStatus"`

	// Time when last Replay Configuration change was detected
	// Right now it's updated on each reconcile, we have to skip when reconciliation detects no changes
	// +optional
	LastUpdatedTime *metav1.Time `json:"lastUpdatedTime,omitempty"`

	// Conditions store the status conditions of the Replay instances
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MdaiReplay is the Schema for the mdaireplays API.
type MdaiReplay struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiReplaySpec   `json:"spec,omitempty"`
	Status MdaiReplayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiReplayList contains a list of MdaiReplay.
type MdaiReplayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MdaiReplay `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiReplay{}, &MdaiReplayList{})
}
