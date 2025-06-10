package controller

import (
	"fmt"
	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"k8s.io/utils/ptr"
)

var BaseOtlpReceiverName, BaseOtlpReceiverConfig = "otlp", OtlpReceiverConfig{
	Protocols: OtlpReceiverProtocolsConfig{
		Grpc: OtlpReceiverProtocolConfig{
			Endpoint: "0.0.0.0:4317",
		},
		Http: OtlpReceiverProtocolConfig{
			Endpoint: "0.0.0.0:4318"},
	},
}

var BaseK8sEventsReceiverName, BaseK8sEventsReceiverConfig = "k8s_events", K8sEventsReceiverConfig{
	Namespaces: []string{"${env:K8S_NAMESPACE}"},
}

var BaseBatchProcessorName, BaseBatchProcessorConfig = "batch", BatchProcessorConfig{
	SendBatchSize:    1000,
	SendBatchMaxSize: 10000,
	Timeout:          "1m",
}

var SeverityFilterMap = map[v1.SeverityLevel]string{
	v1.ErrorSeverityLevel: "SEVERITY_NUMBER_ERROR",
	v1.WarnSeverityLevel:  "SEVERITY_NUMBER_WARN",
	v1.InfoSeverityLevel:  "SEVERITY_NUMBER_INFO",
	v1.DebugSeverityLevel: "SEVERITY_NUMBER_DEBUG",
}

func getSeverityFilterForLevel(level v1.SeverityLevel) string {
	return fmt.Sprintf("severity_number < %s and attributes[\"mdai-logstream\"] != \"audit\"", SeverityFilterMap[level])
}

var DebugSeverityFilterName, DebugSeverityFilter = "filter/severity_min_debug", FilterProcessorConfig{
	ErrorMode: "ignore",
	Logs: LogsFilterConfig{
		LogRecord: []string{
			getSeverityFilterForLevel(v1.DebugSeverityLevel),
		},
	},
}
var InfoSeverityFilterName, InfoSeverityFilter = "filter/severity_min_info", FilterProcessorConfig{
	ErrorMode: "ignore",
	Logs: LogsFilterConfig{
		LogRecord: []string{
			getSeverityFilterForLevel(v1.InfoSeverityLevel),
		},
	},
}
var WarnSeverityFilterName, WarnSeverityFilter = "filter/severity_min_warn", FilterProcessorConfig{
	ErrorMode: "ignore",
	Logs: LogsFilterConfig{
		LogRecord: []string{
			getSeverityFilterForLevel(v1.WarnSeverityLevel),
		},
	},
}
var ErrorSeverityFilterName, ErrorSeverityFilter = "filter/severity_min_error", FilterProcessorConfig{
	ErrorMode: "ignore",
	Logs: LogsFilterConfig{
		LogRecord: []string{
			getSeverityFilterForLevel(v1.ErrorSeverityLevel),
		},
	},
}

var K8sLogstreamResourceProcessorName, K8sLogstreamResourceProcessor = "resource/k8slogstream", ResourceProcessorConfig{
	Attributes: []ResourceProcessorAttributeConfig{
		{
			Key:    "mdai-logstream",
			Value:  "hub",
			Action: "upsert",
		},
	},
}
var HubToAuditResourceProcessorName, HubToAuditResourceProcessor = "resource/hub-to-audit", ResourceProcessorConfig{
	Attributes: []ResourceProcessorAttributeConfig{
		{
			Key:    "mdai-logstream",
			Value:  "audit",
			Action: "upsert",
		},
	},
}

var VerboseDebugExporterConfigName, VerboseDebugExporterConfig = "debug/verbose", DebugExporterConfig{
	verbosity: ptr.To("detailed"),
}

var LogstreamRoutingConnectorConfigName, DebugRoutingConnectorConfig = "routing/logstream", RoutingConnectorConfig{
	DefaultPipelines: []string{"logs/debug_other"},
	Table: []RoutingConnectorTableEntry{
		{
			Context:   "log",
			Condition: "attributes[\"mdai-logstream\"] == \"audit\"",
			Pipelines: []string{"logs/debug_audit"},
		},
		{
			Context:   "resource",
			Condition: "attributes[\"mdai-logstream\"] == \"audit\"",
			Pipelines: []string{"logs/debug_collector"},
		},
		{
			Context:   "resource",
			Condition: "attributes[\"mdai-logstream\"] == \"audit\"",
			Pipelines: []string{"logs/debug_hub"},
		},
	},
}

var BaseCGroupRuntimeExtensionConfigName, BaseCGroupRuntimeExtensionConfig = "cgroupruntime", CGroupRuntimeConfig{
	GoMaxProcs: GoMaxProcsConfig{enabled: true},
	GoMemLimit: GoMemLimitConfig{enabled: true},
}

var OtlpInputPipelineName, OtlpInputPipeline = "logs/input", PipelineConfig{
	Receivers:  []string{LogstreamRoutingConnectorConfigName},
	Processors: []string{},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
var BaseK8sEventsPipelineName, BaseK8sEventsPipeline = "logs/k8s_events", PipelineConfig{
	Receivers:  []string{BaseK8sEventsReceiverName},
	Processors: []string{K8sLogstreamResourceProcessorName},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
var DebugAuditPipelineName, DebugAuditPipeline = "logs/input", PipelineConfig{
	Receivers:  []string{LogstreamRoutingConnectorConfigName},
	Processors: []string{BaseBatchProcessorName},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
var DebugHubPipelineName, DebugHubPipeline = "logs/input", PipelineConfig{
	Receivers:  []string{LogstreamRoutingConnectorConfigName},
	Processors: []string{BaseBatchProcessorName},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
var DebugCollectorPipelineName, DebugCollectorPipeline = "logs/input", PipelineConfig{
	Receivers:  []string{LogstreamRoutingConnectorConfigName},
	Processors: []string{BaseBatchProcessorName},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
var DebugOtherPipelineName, DebugOtherPipeline = "logs/input", PipelineConfig{
	Receivers:  []string{LogstreamRoutingConnectorConfigName},
	Processors: []string{BaseBatchProcessorName},
	Exporters:  []string{LogstreamRoutingConnectorConfigName},
}
