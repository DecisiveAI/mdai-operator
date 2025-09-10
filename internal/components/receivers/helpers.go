// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// nolint:gofumpt
package receivers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/decisiveai/mdai-operator/internal/components"
)

const (
	DefaultStatsdPort              = 8125
	DefaultOtlpGrpcPort            = 4317
	DefaultOtlpHttpPort            = 4318
	DefaultSkyWalkingGrpcPort      = 11800
	DefaultSkyWalkingHttpPort      = 12800
	DefaultJaegerGrpcPort          = 14250
	DefaultJaegerThriftHttpPort    = 14268
	DefaultJaegerThriftCompactPort = 6831
	DefaultJaegerThriftBinaryPort  = 6832
	DefaultLokiGrpcPort            = 9095
	DefaultLokiHttpPort            = 3100
	DefaultAwsXrayPort             = 2000
	DefaultCarbonPort              = 2003
	DefaultCollectdPort            = 8081
	DefaultFluentForwardPort       = 8006
	DefaultInfluxDBPort            = 8086
	DefaultOpencensusPort          = 55678
	DefaultSapmPort                = 7276
	DefaultSignalFxPort            = 9943
	DefaultSplunkHecPort           = 8088
	DefaultWaveFrontPort           = 2003
	DefaultZipkinPort              = 9411
	DefaultZipkinTargetPort        = 3100
)

// registry holds a record of all known receiver parsers.
var registry = make(map[string]components.Parser)

// Register adds a new parser builder to the list of known builders.
func Register(name string, p components.Parser) {
	registry[name] = p
}

// IsRegistered checks whether a parser is registered with the given name.
func IsRegistered(name string) bool {
	_, ok := registry[components.ComponentType(name)]
	return ok
}

// ReceiverFor returns a parser builder for the given exporter name.
func ReceiverFor(name string) components.Parser {
	if parser, ok := registry[components.ComponentType(name)]; ok {
		return parser
	}
	return components.NewSilentSinglePortParserBuilder(components.ComponentType(name), components.UnsetPort).MustBuild()
}

// NewScraperParser is an instance of a generic parser that returns nothing when called and never fails.
func NewScraperParser(name string) *components.GenericParser[any] {
	return components.NewBuilder[any]().WithName(name).WithPort(components.UnsetPort).MustBuild()
}

var (
	componentParsers = []components.Parser{
		components.NewMultiPortReceiverBuilder("otlp").
			AddPortMapping(components.NewProtocolBuilder("grpc", DefaultOtlpGrpcPort).
				WithAppProtocol(&components.GrpcProtocol).
				WithTargetPort(DefaultOtlpGrpcPort).
				// mydecisive
				WithUrlPaths(
					[]string{
						"/opentelemetry.proto.collector.logs.v1.LogsService",
						"/opentelemetry.proto.collector.traces.v1.TracesService",
						"/opentelemetry.proto.collector.metrics.v1.MetricsService",
					})).
			AddPortMapping(components.NewProtocolBuilder("http", DefaultOtlpHttpPort).
				WithAppProtocol(&components.HttpProtocol).
				WithTargetPort(DefaultOtlpHttpPort)).
			MustBuild(),
		components.NewMultiPortReceiverBuilder("skywalking").
			AddPortMapping(components.NewProtocolBuilder(components.GrpcProtocol, DefaultSkyWalkingGrpcPort).
				WithTargetPort(DefaultSkyWalkingGrpcPort).
				WithAppProtocol(&components.GrpcProtocol).
				// mydecisive
				WithUrlPaths(
					[]string{
						"/skywalking.v3/ManagementService",
						"/skywalking.v3/TraceSegmentReportService",
						"/skywalking.v3/JVMMetricReportService",
					})).
			AddPortMapping(components.NewProtocolBuilder(components.HttpProtocol, DefaultSkyWalkingHttpPort).
				WithTargetPort(DefaultSkyWalkingHttpPort).
				WithAppProtocol(&components.HttpProtocol)).
			MustBuild(),
		components.NewMultiPortReceiverBuilder("jaeger").
			AddPortMapping(components.NewProtocolBuilder(components.GrpcProtocol, DefaultJaegerGrpcPort).
				WithTargetPort(DefaultJaegerGrpcPort).
				WithProtocol(corev1.ProtocolTCP).
				WithAppProtocol(&components.GrpcProtocol).
				// mydecisive
				WithUrlPaths(
					[]string{
						"/jaeger.api_v2/CollectorService",
						"/jaeger.api_v3/QueryService",
					})).
			AddPortMapping(components.NewProtocolBuilder("thrift_http", DefaultJaegerThriftHttpPort).
				WithTargetPort(DefaultJaegerThriftHttpPort).
				WithProtocol(corev1.ProtocolTCP).
				WithAppProtocol(&components.HttpProtocol)).
			AddPortMapping(components.NewProtocolBuilder("thrift_compact", DefaultJaegerThriftCompactPort).
				WithTargetPort(DefaultJaegerThriftCompactPort).
				WithProtocol(corev1.ProtocolUDP)).
			AddPortMapping(components.NewProtocolBuilder("thrift_binary", DefaultJaegerThriftBinaryPort).
				WithTargetPort(DefaultJaegerThriftBinaryPort).
				WithProtocol(corev1.ProtocolUDP)).
			MustBuild(),
		components.NewMultiPortReceiverBuilder("loki").
			AddPortMapping(components.NewProtocolBuilder(components.GrpcProtocol, DefaultLokiGrpcPort).
				WithTargetPort(DefaultLokiGrpcPort).
				WithAppProtocol(&components.GrpcProtocol).
				// mydecisive
				WithUrlPaths(
					[]string{
						"/logproto.Pusher",
					})).
			AddPortMapping(components.NewProtocolBuilder(components.HttpProtocol, DefaultLokiHttpPort).
				WithTargetPort(DefaultLokiHttpPort).
				WithAppProtocol(&components.HttpProtocol)).
			MustBuild(),
		components.NewSinglePortParserBuilder("awsxray", DefaultAwsXrayPort).
			WithTargetPort(DefaultAwsXrayPort).
			WithProtocol(corev1.ProtocolUDP).
			MustBuild(),
		components.NewSinglePortParserBuilder("carbon", DefaultCarbonPort).
			WithTargetPort(DefaultCarbonPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("collectd", DefaultCollectdPort).
			WithTargetPort(DefaultCollectdPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("fluentforward", DefaultFluentForwardPort).
			WithTargetPort(DefaultFluentForwardPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("influxdb", DefaultInfluxDBPort).
			WithTargetPort(DefaultInfluxDBPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("opencensus", DefaultOpencensusPort).
			WithAppProtocol(nil).
			WithTargetPort(DefaultOpencensusPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("sapm", DefaultSapmPort).
			WithTargetPort(DefaultSapmPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("signalfx", DefaultSignalFxPort).
			WithTargetPort(DefaultSignalFxPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("splunk_hec", DefaultSplunkHecPort).
			WithTargetPort(DefaultSplunkHecPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("statsd", DefaultStatsdPort).
			WithProtocol(corev1.ProtocolUDP).
			WithTargetPort(DefaultStatsdPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("tcplog", components.UnsetPort).
			WithProtocol(corev1.ProtocolTCP).
			MustBuild(),
		components.NewSinglePortParserBuilder("udplog", components.UnsetPort).
			WithProtocol(corev1.ProtocolUDP).
			MustBuild(),
		components.NewSinglePortParserBuilder("wavefront", DefaultWaveFrontPort).
			WithTargetPort(DefaultWaveFrontPort).
			MustBuild(),
		components.NewSinglePortParserBuilder("zipkin", DefaultZipkinPort).
			WithAppProtocol(&components.HttpProtocol).
			WithProtocol(corev1.ProtocolTCP).
			WithTargetPort(DefaultZipkinTargetPort).
			MustBuild(),
		components.NewBuilder[kubeletStatsConfig]().WithName("kubeletstats").
			WithRbacGen(generateKubeletStatsRbacRules).
			WithEnvVarGen(generateKubeletStatsEnvVars).
			MustBuild(),
		components.NewBuilder[k8seventsConfig]().WithName("k8s_events").
			WithRbacGen(generatek8seventsRbacRules).
			MustBuild(),
		components.NewBuilder[k8sclusterConfig]().WithName("k8s_cluster").
			WithRbacGen(generatek8sclusterRbacRules).
			MustBuild(),
		components.NewBuilder[k8sobjectsConfig]().WithName("k8sobjects").
			WithRbacGen(generatek8sobjectsRbacRules).
			MustBuild(),
		NewScraperParser("prometheus"),
		NewScraperParser("sshcheck"),
		NewScraperParser("cloudfoundry"),
		NewScraperParser("vcenter"),
		NewScraperParser("oracledb"),
		NewScraperParser("snmp"),
		NewScraperParser("googlecloudpubsub"),
		NewScraperParser("chrony"),
		NewScraperParser("jmx"),
		NewScraperParser("podman_stats"),
		NewScraperParser("pulsar"),
		NewScraperParser("docker_stats"),
		NewScraperParser("aerospike"),
		NewScraperParser("zookeeper"),
		NewScraperParser("prometheus_simple"),
		NewScraperParser("saphana"),
		NewScraperParser("riak"),
		NewScraperParser("redis"),
		NewScraperParser("rabbitmq"),
		NewScraperParser("purefb"),
		NewScraperParser("postgresql"),
		NewScraperParser("nsxt"),
		NewScraperParser("nginx"),
		NewScraperParser("mysql"),
		NewScraperParser("memcached"),
		NewScraperParser("httpcheck"),
		NewScraperParser("haproxy"),
		NewScraperParser("flinkmetrics"),
		NewScraperParser("couchdb"),
		NewScraperParser("filelog"),
	}
)

func init() { //nolint:gochecknoinits
	for _, parser := range componentParsers {
		Register(parser.ParserType(), parser)
	}
}
