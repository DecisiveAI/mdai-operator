// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"testing"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild1(t *testing.T) {
	t.Run("2 grpc 2 non-grpc receivers", func(t *testing.T) {
		var (
			ns               = "test"
			ingressClassName = "aws"
			expectedObjects  = 3
		)

		params, err := newParams("testdata/ingress_aws_testdata_2_and_2.yaml")
		require.NoError(t, err)

		params.OtelMdaiIngressComb.Otelcol.Namespace = ns
		params.OtelMdaiIngressComb.MdaiIngress.Spec = mdaiv1.MdaiIngressSpec{
			CloudType:        mdaiv1.CloudProviderAws,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
				"otlp/2": "otlp-2.some.domain.io",
				"jaeger": "jaeger.some.domain.io",
			},
		}

		objects, err := Build(params)
		require.NoError(t, err)
		assert.Len(t, objects, expectedObjects)
	})
	t.Run("1 grpc 1 non-grpc receivers", func(t *testing.T) {
		var (
			ns               = "test"
			ingressClassName = "aws"
			expectedObjects  = 3
		)

		params, err := newParams("testdata/ingress_aws_testdata_1_and_1.yaml")
		require.NoError(t, err)

		params.OtelMdaiIngressComb.Otelcol.Namespace = ns
		params.OtelMdaiIngressComb.MdaiIngress.Spec = mdaiv1.MdaiIngressSpec{
			CloudType:        mdaiv1.CloudProviderAws,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
				"jaeger": "jaeger.some.domain.io",
			},
		}

		objects, err := Build(params)
		require.NoError(t, err)
		assert.Len(t, objects, expectedObjects)
	})
	t.Run("1 grpc receiver", func(t *testing.T) {
		var (
			ns               = "test"
			ingressClassName = "aws"
			// 1 grpc service, 1 ingress, 1 non-grpc service from the additional port coming from newParams() call below
			expectedObjects = 3
		)

		params, err := newParams("testdata/ingress_aws_testdata_1_grpc.yaml")
		require.NoError(t, err)

		params.OtelMdaiIngressComb.Otelcol.Namespace = ns
		params.OtelMdaiIngressComb.MdaiIngress.Spec = mdaiv1.MdaiIngressSpec{
			CloudType:        mdaiv1.CloudProviderAws,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			GrpcService:      &mdaiv1.IngressService{},
			NonGrpcService:   &mdaiv1.IngressService{},
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
			},
		}

		objects, err := Build(params)
		require.NoError(t, err)
		assert.Len(t, objects, expectedObjects)
	})
	t.Run("1 non-grpc receiver", func(t *testing.T) {
		var (
			ns               = "test"
			ingressClassName = "aws"
			expectedObjects  = 1
		)

		params, err := newParams("testdata/ingress_aws_testdata_1_non_grpc.yaml")
		require.NoError(t, err)

		params.OtelMdaiIngressComb.Otelcol.Namespace = ns
		params.OtelMdaiIngressComb.MdaiIngress.Spec = mdaiv1.MdaiIngressSpec{
			CloudType:        mdaiv1.CloudProviderAws,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			GrpcService:      &mdaiv1.IngressService{},
			NonGrpcService:   &mdaiv1.IngressService{},
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
			},
		}

		objects, err := Build(params)
		require.NoError(t, err)
		assert.Len(t, objects, expectedObjects)
	})
}
