// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// nolint:goconst
package collector

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServicePortsFromCfg(t *testing.T) {
	t.Run("should return nil invalid ingress type", func(t *testing.T) {
		logger := zap.NewNop()

		http := "http"
		grpc := "grpc"

		params, err := newParams("testdata/ingress_aws_testdata_2_and_2.yaml")
		require.NoError(t, err)

		expected := []corev1.ServicePort{
			{
				Name:        "web",
				Protocol:    "",
				AppProtocol: nil,
				Port:        80,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 80,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "jaeger-grpc",
				Protocol:    "TCP",
				AppProtocol: &grpc,
				Port:        14260,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 14260,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "otlp-1-grpc",
				Protocol:    "",
				AppProtocol: &grpc,
				Port:        12345,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 12345,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "otlp-1-http",
				Protocol:    "",
				AppProtocol: &http,
				Port:        12121,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 12121,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "otlp-2-grpc",
				Protocol:    "",
				AppProtocol: &grpc,
				Port:        98765,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 98765,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "otlp-2-http",
				Protocol:    "",
				AppProtocol: &http,
				Port:        4318,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 4318,
					StrVal: "",
				},
				NodePort: 0,
			},
			{
				Name:        "port-14268",
				Protocol:    "TCP",
				AppProtocol: &http,
				Port:        14268,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 14268,
					StrVal: "",
				},
				NodePort: 0,
			},
		}

		actual, err := servicePortsFromCfg(logger, params.OtelMdaiIngressComb.Otelcol)
		require.NoError(t, err)
		assert.ElementsMatch(t, expected, actual)
	})
}
