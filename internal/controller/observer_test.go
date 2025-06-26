package controller

import (
	"testing"

	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestGetObserverCollectorConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc             string
		observers        []v1.Observer
		observerResource v1.ObserverResource
		check            func(t *testing.T, resultConfig string, err error)
	}{
		{
			desc:      "no observers provided",
			observers: []v1.Observer{},
			observerResource: v1.ObserverResource{
				GrpcReceiverMaxMsgSize: lo.ToPtr(uint64(123)),
				OwnLogsOtlpEndpoint:    lo.ToPtr("otlp://my.endpoint:4317"),
			},
			check: func(t *testing.T, resultConfig string, err error) {
				require.NoError(t, err)

				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(resultConfig), &config))

				serviceBlock := config["service"].(map[string]any)
				require.Len(t, serviceBlock["pipelines"], 1) // only the metrics pipeline, no logs
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["metrics/observeroutput"])

				grpcReceiverMaxMsgSize := config["receivers"].(map[string]any)["otlp"].(map[string]any)["protocols"].(map[string]any)["grpc"].(map[string]any)["max_recv_msg_size_mib"]
				assert.Equal(t, float64(123), grpcReceiverMaxMsgSize) // yaml unmarshal converts ints to floats

				telemetryProcessors := serviceBlock["telemetry"].(map[string]any)["logs"].(map[string]any)["processors"].([]any)
				require.Len(t, telemetryProcessors, 1)
				assert.Equal(t, "otlp://my.endpoint:4317", telemetryProcessors[0].(map[string]any)["batch"].(map[string]any)["exporter"].(map[string]any)["otlp"].(map[string]any)["endpoint"])
			},
		},
		{
			desc: "observers present",
			observers: []v1.Observer{
				{
					Name:                    "observer-in",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_received_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_received_by_service_total"),
					Filter: &v1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &v1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "received"`},
						},
					},
				},
				{
					Name:                    "observer-out",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_sent_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_sent_by_service_total"),
					Filter: &v1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &v1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "exported"`},
						},
					},
				},
			},
			observerResource: v1.ObserverResource{
				GrpcReceiverMaxMsgSize: lo.ToPtr(uint64(123)),
				OwnLogsOtlpEndpoint:    lo.ToPtr("otlp://my.endpoint:4317"),
			},
			check: func(t *testing.T, resultConfig string, err error) {
				require.NoError(t, err)

				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(resultConfig), &config))

				serviceBlock := config["service"].(map[string]any)
				// pipelines: 1 metric, 2 logs, 2 traces
				require.Len(t, serviceBlock["pipelines"], 5)
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["metrics/observeroutput"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["logs/observer-in"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["logs/observer-out"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["traces/observer-in"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["traces/observer-out"])

				grpcReceiverMaxMsgSize := config["receivers"].(map[string]any)["otlp"].(map[string]any)["protocols"].(map[string]any)["grpc"].(map[string]any)["max_recv_msg_size_mib"]
				assert.Equal(t, float64(123), grpcReceiverMaxMsgSize) // yaml unmarshal converts ints to floats

				telemetryProcessors := serviceBlock["telemetry"].(map[string]any)["logs"].(map[string]any)["processors"].([]any)
				require.Len(t, telemetryProcessors, 1)
				assert.Equal(t, "otlp://my.endpoint:4317", telemetryProcessors[0].(map[string]any)["batch"].(map[string]any)["exporter"].(map[string]any)["otlp"].(map[string]any)["endpoint"])

				// now, validate the observer config was added
				processors := config["processors"].(map[string]any)
				require.Len(t, processors, 6)

				require.NotNil(t, processors["groupbyattrs/observer-in"])
				assert.ElementsMatch(t, []string{"mdai_service"}, processors["groupbyattrs/observer-in"].(map[string]any)["keys"])

				require.NotNil(t, processors["filter/observer-in"])
				require.NotNil(t, processors["filter/observer-out"])

				connectors := config["connectors"].(map[string]any)
				require.NotNil(t, connectors["datavolume/observer-in"])
				assert.Equal(t, []any{"mdai_service"}, connectors["datavolume/observer-in"].(map[string]any)["label_resource_attributes"])
				assert.Equal(t, "items_received_by_service_total", connectors["datavolume/observer-in"].(map[string]any)["count_metric_name"])
				assert.Equal(t, "bytes_received_by_service_total", connectors["datavolume/observer-in"].(map[string]any)["bytes_metric_name"])

				require.NotNil(t, connectors["datavolume/observer-out"])
				assert.Equal(t, []any{"mdai_service"}, connectors["datavolume/observer-out"].(map[string]any)["label_resource_attributes"])
				assert.Equal(t, "items_sent_by_service_total", connectors["datavolume/observer-out"].(map[string]any)["count_metric_name"])
				assert.Equal(t, "bytes_sent_by_service_total", connectors["datavolume/observer-out"].(map[string]any)["bytes_metric_name"])
			},
		}, {
			desc: "observers with severity present",
			observers: []v1.Observer{
				{
					Name:                    "observer-in",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_received_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_received_by_service_total"),
					Logs: &v1.ObserverLogsConfig{
						CountSeverityBy: "severity_attr",
					},
					Filter: &v1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &v1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "received"`},
						},
					},
				},
				{
					Name:                    "observer-out",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_sent_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_sent_by_service_total"),
					Logs: &v1.ObserverLogsConfig{
						CountSeverityBy: "logs.severity_text",
					},
					Filter: &v1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &v1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "exported"`},
						},
					},
				},
			},
			observerResource: v1.ObserverResource{
				GrpcReceiverMaxMsgSize: lo.ToPtr(uint64(123)),
				OwnLogsOtlpEndpoint:    lo.ToPtr("otlp://my.endpoint:4317"),
			},
			check: func(t *testing.T, resultConfig string, err error) {
				require.NoError(t, err)

				var config map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(resultConfig), &config))

				serviceBlock := config["service"].(map[string]any)
				// pipelines: 1 metric, 2 logs, 2 traces
				require.Len(t, serviceBlock["pipelines"], 5)
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["metrics/observeroutput"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["logs/observer-in"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["logs/observer-out"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["traces/observer-in"])
				assert.NotNil(t, serviceBlock["pipelines"].(map[string]any)["traces/observer-out"])

				grpcReceiverMaxMsgSize := config["receivers"].(map[string]any)["otlp"].(map[string]any)["protocols"].(map[string]any)["grpc"].(map[string]any)["max_recv_msg_size_mib"]
				assert.Equal(t, float64(123), grpcReceiverMaxMsgSize) // yaml unmarshal converts ints to floats

				telemetryProcessors := serviceBlock["telemetry"].(map[string]any)["logs"].(map[string]any)["processors"].([]any)
				require.Len(t, telemetryProcessors, 1)
				assert.Equal(t, "otlp://my.endpoint:4317", telemetryProcessors[0].(map[string]any)["batch"].(map[string]any)["exporter"].(map[string]any)["otlp"].(map[string]any)["endpoint"])

				// now, validate the observer config was added
				processors := config["processors"].(map[string]any)
				require.Len(t, processors, 6)

				require.NotNil(t, processors["groupbyattrs/observer-in"])
				assert.ElementsMatch(t, []string{"mdai_service"}, processors["groupbyattrs/observer-in"].(map[string]any)["keys"])

				require.NotNil(t, processors["filter/observer-in"])
				require.NotNil(t, processors["filter/observer-out"])

				connectors := config["connectors"].(map[string]any)
				require.NotNil(t, connectors["datavolume/observer-in"])
				assert.Equal(t, []any{"mdai_service"}, connectors["datavolume/observer-in"].(map[string]any)["label_resource_attributes"])
				assert.Equal(t, "items_received_by_service_total", connectors["datavolume/observer-in"].(map[string]any)["count_metric_name"])
				assert.Equal(t, "bytes_received_by_service_total", connectors["datavolume/observer-in"].(map[string]any)["bytes_metric_name"])
				assert.Equal(t, "severity_attr", connectors["datavolume/observer-in"].(map[string]any)["logs"].(map[string]any)["count_severity_by"])

				require.NotNil(t, connectors["datavolume/observer-out"])
				assert.Equal(t, []any{"mdai_service"}, connectors["datavolume/observer-out"].(map[string]any)["label_resource_attributes"])
				assert.Equal(t, "items_sent_by_service_total", connectors["datavolume/observer-out"].(map[string]any)["count_metric_name"])
				assert.Equal(t, "bytes_sent_by_service_total", connectors["datavolume/observer-out"].(map[string]any)["bytes_metric_name"])
				assert.Equal(t, "logs.severity_text", connectors["datavolume/observer-out"].(map[string]any)["logs"].(map[string]any)["count_severity_by"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			testObj := NewObserverAdapter(nil, logr.Discard(), nil, nil, nil)

			config, err := testObj.getObserverCollectorConfig(tc.observers, tc.observerResource)
			tc.check(t, config, err)
		})
	}
}
