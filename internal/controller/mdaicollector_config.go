package controller

/* --- RECEIVERS --- */

type OtlpReceiverConfig struct {
	Protocols OtlpReceiverProtocolsConfig `yaml:"protocols"`
}
type OtlpReceiverProtocolsConfig struct {
	Grpc OtlpReceiverProtocolConfig `yaml:"grpc"`
	Http OtlpReceiverProtocolConfig `yaml:"http"`
}
type OtlpReceiverProtocolConfig struct {
	Endpoint string `yaml:"endpoint"`
}

type K8sEventsReceiverConfig struct {
	Namespaces []string `yaml:"namespaces"`
}

/* --- PROCESSORS --- */

type BatchProcessorConfig struct {
	SendBatchSize    int    `yaml:"send_batch_size"`
	SendBatchMaxSize int    `yaml:"send_batch_max_size"`
	Timeout          string `yaml:"timeout"`
}
type FilterProcessorConfig struct {
	ErrorMode string           `yaml:"error_mode"`
	Logs      LogsFilterConfig `yaml:"logs"`
}
type LogsFilterConfig struct {
	LogRecord []string `yaml:"log_record"`
}
type ResourceProcessorConfig struct {
	Attributes []ResourceProcessorAttributeConfig `yaml:"attributes"`
}
type ResourceProcessorAttributeConfig struct {
	Key    string `yaml:"key"`
	Value  string `yaml:"value"`
	Action string `yaml:"action"`
}

/* --- EXPORTERS --- */

type DebugExporterConfig struct {
	verbosity *string `yaml:"verbosity,omitempty"`
}

type AwsS3ExporterConfig struct {
	s3uploader *S3UploaderConfig `mapstructure:"s3uploader"`
}
type S3UploaderConfig struct {
	Region            string `yaml:"region"`
	S3Bucket          string `yaml:"s3_bucket"`
	S3Prefix          string `yaml:"s3_prefix"`
	S3PartitionFormat string `yaml:"s3_partition_format"`
	FilePrefix        string `yaml:"file_prefix"`
	DisableSSL        bool   `yaml:"disable_ssl"`
}

type OtlpExporterConfig struct {
	Endpoint  string        `yaml:"endpoint"`
	TlsConfig OtlpTlsConfig `yaml:"tls"`
}
type OtlpTlsConfig struct {
	Insecure bool `yaml:"insecure"`
}

/* --- CONNECTORS --- */

type RoutingConnectorConfig struct {
	DefaultPipelines []string                     `yaml:"default_pipelines"`
	Table            []RoutingConnectorTableEntry `yaml:"table"`
}
type RoutingConnectorTableEntry struct {
	Context   string   `yaml:"context"`
	Condition string   `yaml:"condition"`
	Pipelines []string `yaml:"pipelines"`
}

/* --- EXTENSIONS --- */

type ExtensionsConfig struct {
	CGroupRuntime CGroupRuntimeConfig `yaml:"cgroupruntime"`
}
type CGroupRuntimeConfig struct {
	GoMaxProcs GoMaxProcsConfig `yaml:"gomaxprocs"`
	GoMemLimit GoMemLimitConfig `yaml:"gomemlimit"`
}
type GoMaxProcsConfig struct {
	enabled bool `yaml:"enabled"`
}
type GoMemLimitConfig struct {
	enabled bool `yaml:"enabled"`
}

/* --- SERVICE --- */

type ServiceConfig struct {
	Pipelines map[string]PipelineConfig `yaml:"pipelines"`
}
type PipelineConfig struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}
