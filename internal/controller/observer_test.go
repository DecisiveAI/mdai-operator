package controller

import (
	"testing"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/mdai-operator/internal/builder"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestGetObserverCollectorConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc         string
		observers    []mdaiv1.Observer
		observerSpec mdaiv1.MdaiObserverSpec
		check        func(t *testing.T, resultConfig string, err error)
	}{
		{
			desc:      "no observers provided",
			observers: []mdaiv1.Observer{},
			observerSpec: mdaiv1.MdaiObserverSpec{
				GrpcReceiverMaxMsgSize: lo.ToPtr(uint64(123)),
				OwnLogsOtlpEndpoint:    lo.ToPtr("otlp://my.endpoint:4317"),
			},
			check: func(t *testing.T, resultConfig string, err error) {
				t.Helper()
				require.NoError(t, err)

				var config builder.ConfigBlock
				require.NoError(t, yaml.Unmarshal([]byte(resultConfig), &config))

				serviceBlock := config.MustMap("service")
				require.Len(t, serviceBlock.MustMap("pipelines"), 1) // only the metrics pipeline, no logs
				assert.NotNil(t, serviceBlock.MustMap("pipelines").MustMap("metrics/observeroutput"))

				grpcReceiverMaxMsgSize := config.MustMap("receivers").MustMap("otlp").MustMap("protocols").MustMap("grpc").MustFloat("max_recv_msg_size_mib")
				assert.Equal(t, 123, int(grpcReceiverMaxMsgSize)) // yaml unmarshal converts ints to floats

				telemetryProcessors := serviceBlock.MustMap("telemetry").MustMap("logs").MustSlice("processors")
				require.Len(t, telemetryProcessors, 1)
				telemetryProcessor, ok := telemetryProcessors[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "otlp://my.endpoint:4317", builder.ConfigBlock(telemetryProcessor).MustMap("batch").MustMap("exporter").MustMap("otlp").MustString("endpoint"))
			},
		},
		{
			desc: "observers present",
			observers: []mdaiv1.Observer{
				{
					Name:                    "observer-in",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_received_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_received_by_service_total"),
					Filter: &mdaiv1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &mdaiv1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "received"`},
						},
					},
				},
				{
					Name:                    "observer-out",
					LabelResourceAttributes: []string{"mdai_service"},
					CountMetricName:         lo.ToPtr("items_sent_by_service_total"),
					BytesMetricName:         lo.ToPtr("bytes_sent_by_service_total"),
					Filter: &mdaiv1.ObserverFilter{
						ErrorMode: lo.ToPtr("ignore"),
						Logs: &mdaiv1.ObserverLogsFilter{
							LogRecord: []string{`resource.attributes["observer_direction"] != "exported"`},
						},
					},
				},
			},
			observerSpec: mdaiv1.MdaiObserverSpec{
				GrpcReceiverMaxMsgSize: lo.ToPtr(uint64(123)),
				OwnLogsOtlpEndpoint:    lo.ToPtr("otlp://my.endpoint:4317"),
			},
			check: func(t *testing.T, resultConfig string, err error) {
				t.Helper()
				require.NoError(t, err)

				var config builder.ConfigBlock
				require.NoError(t, yaml.Unmarshal([]byte(resultConfig), &config))

				serviceBlock := config.MustMap("service")
				// pipelines: 1 metric, 2 logs, 2 traces
				pipelines := serviceBlock.MustMap("pipelines")
				require.Len(t, pipelines, 5)
				assert.NotNil(t, pipelines.MustMap("metrics/observeroutput"))
				assert.NotNil(t, pipelines.MustMap("logs/observer-in"))
				assert.NotNil(t, pipelines.MustMap("logs/observer-out"))
				assert.NotNil(t, pipelines.MustMap("traces/observer-in"))
				assert.NotNil(t, pipelines.MustMap("traces/observer-out"))

				grpcReceiverMaxMsgSize := config.MustMap("receivers").MustMap("otlp").MustMap("protocols").MustMap("grpc").MustFloat("max_recv_msg_size_mib")
				assert.Equal(t, 123, int(grpcReceiverMaxMsgSize)) // yaml unmarshal converts ints to floats

				telemetryProcessors := serviceBlock.MustMap("telemetry").MustMap("logs").MustSlice("processors")
				require.Len(t, telemetryProcessors, 1)
				telemetryProcessor, ok := telemetryProcessors[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "otlp://my.endpoint:4317", builder.ConfigBlock(telemetryProcessor).MustMap("batch").MustMap("exporter").MustMap("otlp").MustString("endpoint"))

				// now, validate the observer config was added
				processors := config.MustMap("processors")
				require.Len(t, processors, 6)

				require.NotNil(t, processors.MustMap("groupbyattrs/observer-in"))
				assert.ElementsMatch(t, []string{"mdai_service"}, processors.MustMap("groupbyattrs/observer-in").MustSlice("keys"))

				require.NotNil(t, processors.MustMap("filter/observer-in"))
				require.NotNil(t, processors.MustMap("filter/observer-out"))

				connectors := config.MustMap("connectors")
				require.NotNil(t, connectors.MustMap("datavolume/observer-in"))
				assert.Equal(t, []any{"mdai_service"}, connectors.MustMap("datavolume/observer-in").MustSlice("label_resource_attributes"))
				assert.Equal(t, "items_received_by_service_total", connectors.MustMap("datavolume/observer-in").MustString("count_metric_name"))
				assert.Equal(t, "bytes_received_by_service_total", connectors.MustMap("datavolume/observer-in").MustString("bytes_metric_name"))

				require.NotNil(t, connectors.MustMap("datavolume/observer-out"))
				assert.Equal(t, []any{"mdai_service"}, connectors.MustMap("datavolume/observer-out").MustSlice("label_resource_attributes"))
				assert.Equal(t, "items_sent_by_service_total", connectors.MustMap("datavolume/observer-out").MustString("count_metric_name"))
				assert.Equal(t, "bytes_sent_by_service_total", connectors.MustMap("datavolume/observer-out").MustString("bytes_metric_name"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			testObj := NewObserverAdapter(nil, logr.Discard(), nil, nil, nil)

			config, err := testObj.getObserverCollectorConfig(tc.observers, tc.observerSpec)
			tc.check(t, config, err)
		})
	}
}
