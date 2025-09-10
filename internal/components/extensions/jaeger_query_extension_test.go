// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package extensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestJaegerQueryExtensionParser(t *testing.T) {
	jaegerBuilder := NewJaegerQueryExtensionParserBuilder()
	genericBuilder, err := jaegerBuilder.Build()
	require.NoError(t, err)

	assert.Equal(t, "jaeger_query", genericBuilder.ParserType())
	assert.Equal(t, "__jaeger_query", genericBuilder.ParserName())

	defaultCfg, err := genericBuilder.GetDefaultConfig(zap.NewNop(), nil)
	require.NoError(t, err)

	ports, err := genericBuilder.Ports(zap.NewNop(), "jaeger_query", defaultCfg)
	require.NoError(t, err)
	assert.Equal(t, []corev1.ServicePort{{
		Name:       "jaeger-query",
		Port:       16686,
		TargetPort: intstr.FromInt32(16686),
	}}, ports)
}

func TestJaegerQueryExtensionParser_config(t *testing.T) {
	jaegerBuilder := NewJaegerQueryExtensionParserBuilder()
	genericBuilder, err := jaegerBuilder.Build()
	require.NoError(t, err)

	tests := []struct {
		name   string
		config any
		want   any
	}{
		{
			name:   "valid config",
			config: map[string]any{"http": map[string]any{"endpoint": "127.0.0.0:16686"}},
			want:   map[string]any{"http": map[string]any{"endpoint": "127.0.0.0:16686"}},
		},
		{
			name: "missing config",
			want: map[string]any{"http": map[string]any{"endpoint": "0.0.0.0:16686"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, errCfg := genericBuilder.GetDefaultConfig(zap.NewNop(), test.config)
			assert.Equal(t, test.want, cfg)
			require.NoError(t, errCfg)
		})
	}
}
