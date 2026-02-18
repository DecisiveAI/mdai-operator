// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package receivers_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/mydecisive/mdai-operator/internal/components/receivers"
	"github.com/mydecisive/mdai-operator/internal/naming"
)

var logger = zap.NewNop()

func TestParseEndpoint(t *testing.T) {
	// prepare
	// there's no parser registered to handle "myreceiver", so, it falls back to the generic parser
	parser := receivers.ReceiverFor("myreceiver")

	// test
	ports, err := parser.Ports(logger, "myreceiver", map[string]any{
		"endpoint": "0.0.0.0:1234",
	})

	// verify
	require.NoError(t, err)
	assert.Len(t, ports, 1)
	assert.EqualValues(t, 1234, ports[0].Port)
}

func TestFailedToParseEndpoint(t *testing.T) {
	// prepare
	// there's no parser registered to handle "myreceiver", so, it falls back to the generic parser
	parser := receivers.ReceiverFor("myreceiver")

	// test
	ports, err := parser.Ports(logger, "myreceiver", map[string]any{
		"endpoint": "0.0.0.0",
	})

	// verify
	require.NoError(t, err)
	assert.Empty(t, ports)
}

func TestDownstreamParsers(t *testing.T) {
	for _, tt := range []struct {
		desc             string
		receiverName     string
		parserName       string
		defaultPort      int32
		listenAddrParser bool
	}{
		{"zipkin", "zipkin", "__zipkin", 9411, false},
		{"opencensus", "opencensus", "__opencensus", 55678, false},

		// contrib receivers
		{"carbon", "carbon", "__carbon", 2003, false},
		{"collectd", "collectd", "__collectd", 8081, false},
		{"sapm", "sapm", "__sapm", 7276, false},
		{"signalfx", "signalfx", "__signalfx", 9943, false},
		{"wavefront", "wavefront", "__wavefront", 2003, false},
		{"fluentforward", "fluentforward", "__fluentforward", 8006, false},
		{"statsd", "statsd", "__statsd", 8125, false},
		{"influxdb", "influxdb", "__influxdb", 8086, false},
		{"splunk_hec", "splunk_hec", "__splunk_hec", 8088, false},
		{"awsxray", "awsxray", "__awsxray", 2000, false},
		{"tcplog", "tcplog", "__tcplog", 0, true},
		{"udplog", "udplog", "__udplog", 0, true},
	} {
		t.Run(tt.receiverName, func(t *testing.T) {
			t.Run("builds successfully", func(t *testing.T) {
				// test
				parser := receivers.ReceiverFor(tt.receiverName)

				// verify
				assert.Equal(t, tt.parserName, parser.ParserName())
			})
			t.Run("bad config errors", func(t *testing.T) {
				// prepare
				parser := receivers.ReceiverFor(tt.receiverName)

				// test throwing in pure junk
				_, err := parser.Ports(logger, tt.receiverName, func() {})

				// verify
				assert.ErrorContains(t, err, "expected a map, got 'func'")
			})

			t.Run("assigns the expected port", func(t *testing.T) {
				// prepare
				parser := receivers.ReceiverFor(tt.receiverName)

				// test
				ports, err := parser.Ports(logger, tt.receiverName, map[string]any{})

				if tt.defaultPort == 0 {
					assert.Empty(t, ports)
					return
				}
				// verify
				require.NoError(t, err)
				assert.Len(t, ports, 1)
				assert.Equal(t, tt.defaultPort, ports[0].Port)
				assert.Equal(t, naming.PortName(tt.receiverName, tt.defaultPort), ports[0].Name)
			})

			t.Run("allows port to be overridden", func(t *testing.T) {
				// prepare
				parser := receivers.ReceiverFor(tt.receiverName)

				// test
				var ports []corev1.ServicePort
				var err error
				if tt.listenAddrParser {
					ports, err = parser.Ports(logger, tt.receiverName, map[string]any{
						"listen_address": "0.0.0.0:65535",
					})
				} else {
					ports, err = parser.Ports(logger, tt.receiverName, map[string]any{
						"endpoint": "0.0.0.0:65535",
					})
				}

				// verify
				require.NoError(t, err)
				assert.Len(t, ports, 1)
				assert.EqualValues(t, 65535, ports[0].Port)
				assert.Equal(t, naming.PortName(tt.receiverName, tt.defaultPort), ports[0].Name)
			})

			t.Run("returns a default config", func(t *testing.T) {
				// prepare
				parser := receivers.ReceiverFor(tt.receiverName)

				// test
				config, err := parser.GetDefaultConfig(logger, map[string]any{})

				// verify
				require.NoError(t, err)
				configMap, ok := config.(map[string]any)
				assert.True(t, ok)
				if tt.defaultPort == 0 {
					assert.Empty(t, configMap, 0)
					return
				}
				if tt.listenAddrParser {
					assert.Equal(t, configMap["listen_address"], fmt.Sprintf("0.0.0.0:%d", tt.defaultPort))
				} else {
					assert.Equal(t, configMap["endpoint"], fmt.Sprintf("0.0.0.0:%d", tt.defaultPort))
				}
			})
		})
	}
}
