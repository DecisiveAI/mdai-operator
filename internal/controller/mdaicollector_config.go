package controller

/* --- RECEIVERS --- */

type OtlpReceiverConfig struct {
	Protocols OtlpReceiverProtocolsConfig `yaml:"protocols" json:"protocols"`
}
type OtlpReceiverProtocolsConfig struct {
	Grpc OtlpReceiverProtocolConfig `yaml:"grpc" json:"grpc"`
	Http OtlpReceiverProtocolConfig `yaml:"http" json:"http"`
}
type OtlpReceiverProtocolConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

type K8sEventsReceiverConfig struct {
	Namespaces []string `yaml:"namespaces" json:"namespaces"`
}

/* --- PROCESSORS --- */

type BatchProcessorConfig struct {
	SendBatchSize    int    `yaml:"send_batch_size" json:"send_batch_size"`
	SendBatchMaxSize int    `yaml:"send_batch_max_size" json:"send_batch_max_size"`
	Timeout          string `yaml:"timeout" json:"timeout"`
}
type FilterProcessorConfig struct {
	ErrorMode string           `yaml:"error_mode" json:"error_mode"`
	Logs      LogsFilterConfig `yaml:"logs" json:"logs"`
}
type LogsFilterConfig struct {
	LogRecord []string `yaml:"log_record" json:"log_record"`
}
type ResourceProcessorConfig struct {
	Attributes []ResourceProcessorAttributeConfig `yaml:"attributes" json:"attributes"`
}
type ResourceProcessorAttributeConfig struct {
	Key    string `yaml:"key" json:"key"`
	Value  string `yaml:"value" json:"value"`
	Action string `yaml:"action" json:"action"`
}

/* --- EXPORTERS --- */

type DebugExporterConfig struct {
	verbosity *string `yaml:"verbosity,omitempty" json:"verbosity,omitempty"`
}

type AwsS3ExporterConfig struct {
	s3uploader *S3UploaderConfig `mapstructure:"s3uploader" json:"s3uploader"`
}
type AWSS3UploaderConfig struct {
	Region            string `yaml:"region" json:"region"`
	S3Bucket          string `yaml:"s3_bucket" json:"s3_bucket"`
	S3Prefix          string `yaml:"s3_prefix" json:"s3_prefix"`
	S3PartitionFormat string `yaml:"s3_partition_format" json:"s3_partition_format"`
	FilePrefix        string `yaml:"file_prefix" json:"file_prefix"`
	DisableSSL        bool   `yaml:"disable_ssl" json:"disable_ssl"`
}

type TODOOtlpExporterConfig struct {
	Endpoint  string        `yaml:"endpoint" json:"endpoint"`
	TlsConfig OtlpTlsConfig `yaml:"tls" json:"tls"`
}
type TODOOtlpTlsConfig struct {
	Insecure bool `yaml:"insecure" json:"insecure"`
}

/* --- CONNECTORS --- */

type RoutingConnectorConfig struct {
	DefaultPipelines []string                     `yaml:"default_pipelines" json:"default_pipelines"`
	Table            []RoutingConnectorTableEntry `yaml:"table" json:"table"`
}
type TODORoutingConnectorTableEntry struct {
	Context   string   `yaml:"context" json:"context"`
	Condition string   `yaml:"condition" json:"condition"`
	Pipelines []string `yaml:"pipelines" json:"pipelines"`
}

/* --- EXTENSIONS --- */

type ExtensionsConfig struct {
	CGroupRuntime CGroupRuntimeConfig `yaml:"cgroupruntime" json:"cgroupruntime"`
}
type CGroupRuntimeConfig struct {
	GoMaxProcs GoMaxProcsConfig `yaml:"gomaxprocs" json:"gomaxprocs"`
	GoMemLimit GoMemLimitConfig `yaml:"gomemlimit" json:"gomemlimit"`
}
type GoMaxProcsConfig struct {
	enabled bool `yaml:"enabled" json:"enabled"`
}
type GoMemLimitConfig struct {
	enabled bool `yaml:"enabled" json:"enabled"`
}

/* --- SERVICE --- */

type ServiceConfig struct {
	Pipelines map[string]PipelineConfig `yaml:"pipelines" json:"pipelines"`
}
type PipelineConfig struct {
	Receivers  []string `yaml:"receivers" json:"receivers"`
	Processors []string `yaml:"processors" json:"processors"`
	Exporters  []string `yaml:"exporters" json:"exporters"`
}
