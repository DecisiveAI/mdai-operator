// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	_ "embed"
	"errors"
	"sort"
	"testing"

	"github.com/decisiveai/mdai-operator/internal/components/config"
	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
)

const testFileIngressAws = "testdata/ingress_aws_testdata.yaml"

func TestDesiredIngressesAws(t *testing.T) {
	t.Run("should return nil invalid ingress type", func(t *testing.T) {
		params := manifests.Params{
			Config: config.Config{},
			Log:    testLogger,
			OtelCol: v1beta1.OpenTelemetryCollector{
				Spec: v1beta1.OpenTelemetryCollectorSpec{
					Ingress: v1beta1.Ingress{
						Type: v1beta1.IngressType("unknown"),
					},
				},
			},
		}

		actual, err := IngressAws(params)
		assert.Nil(t, actual)
		assert.NoError(t, err)
	})

	t.Run("should return nil, no ingress set", func(t *testing.T) {
		params := manifests.Params{
			Config: config.Config{},
			Log:    testLogger,
			OtelCol: v1beta1.OpenTelemetryCollector{
				Spec: v1beta1.OpenTelemetryCollectorSpec{
					Mode: "Deployment",
				},
			},
		}

		actual, err := IngressAws(params)
		assert.Nil(t, actual)
		assert.NoError(t, err)
	})

	t.Run("should return nil unable to parse receiver ports", func(t *testing.T) {
		params := manifests.Params{
			Config: config.Config{},
			Log:    testLogger,
			OtelCol: v1beta1.OpenTelemetryCollector{
				Spec: v1beta1.OpenTelemetryCollectorSpec{
					Config: v1beta1.Config{},
					Ingress: v1beta1.Ingress{
						Type: v1beta1.IngressTypeIngress,
					},
				},
			},
		}

		actual, err := IngressAws(params)
		assert.Nil(t, actual)
		assert.NoError(t, err)
	})

	t.Run("multiple grpc receivers", func(t *testing.T) {
		var (
			ns               = "test"
			hostname         = "example.com"
			ingressClassName = "aws"
		)

		params, err := newParams("something:tag", testFileIngressAws)
		if err != nil {
			t.Fatal(err)
		}

		params.OtelCol.Namespace = ns
		params.OtelCol.Spec.Ingress = v1beta1.Ingress{
			Type:             v1beta1.IngressTypeIngress,
			Hostname:         hostname,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
				"otlp/2": "otlp-2.some.domain.io",
				"jaeger": "jaeger.some.domain.io",
			},
		}

		got, err := IngressAws(params)
		assert.NoError(t, err)

		pathType := networkingv1.PathTypePrefix

		ingressRulesGot := got.Spec.Rules
		ingressRulesExpected := []networkingv1.IngressRule{
			{
				Host: "otlp-1.some.domain.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/opentelemetry.proto.collector.logs.v1.LogsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.traces.v1.TracesService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.metrics.v1.MetricsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Host: "otlp-2.some.domain.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/opentelemetry.proto.collector.logs.v1.LogsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.traces.v1.TracesService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.metrics.v1.MetricsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Host: "jaeger.some.domain.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/jaeger.api_v2/CollectorService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "jaeger-grpc",
										},
									},
								},
							},
							{
								Path:     "/jaeger.api_v3/QueryService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "jaeger-grpc",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		assert.True(t, len(ingressRulesExpected) == len(ingressRulesGot))
		for i := 0; i < len(ingressRulesExpected); i++ {
			sort.Slice(ingressRulesGot, func(i, j int) bool { return ingressRulesGot[i].Host < ingressRulesGot[j].Host })
			sort.Slice(ingressRulesExpected, func(i, j int) bool { return ingressRulesExpected[i].Host < ingressRulesExpected[j].Host })
			assert.True(t, ingressRulesExpected[i].Host == ingressRulesGot[i].Host)
		}

		for i := 0; i < len(ingressRulesExpected); i++ {
			pathsGot := ingressRulesGot[i].IngressRuleValue.HTTP.Paths
			pathsExpected := ingressRulesExpected[i].IngressRuleValue.HTTP.Paths
			sort.Slice(pathsGot, func(i, j int) bool { return pathsGot[i].Path < pathsGot[j].Path })
			sort.Slice(pathsExpected, func(i, j int) bool { return pathsExpected[i].Path < pathsExpected[j].Path })

			assert.True(t, ingressRulesExpected[i].Host == ingressRulesGot[i].Host)
			assert.Equal(t, ingressRulesExpected, ingressRulesGot)
		}
	})
	t.Run("multiple grpc receivers, ONE host-to-component mapping record absent", func(t *testing.T) {
		var (
			ns               = "test"
			hostname         = "example.com"
			ingressClassName = "aws"
		)

		params, err := newParams("something:tag", testFileIngressAws)
		if err != nil {
			t.Fatal(err)
		}

		params.OtelCol.Namespace = ns
		params.OtelCol.Spec.Ingress = v1beta1.Ingress{
			Type:             v1beta1.IngressTypeIngress,
			Hostname:         hostname,
			Annotations:      map[string]string{"some.key": "some.value"},
			IngressClassName: &ingressClassName,
			CollectorEndpoints: map[string]string{
				"otlp/1": "otlp-1.some.domain.io",
				"otlp/2": "otlp-2.some.domain.io",
			},
		}

		got, err := IngressAws(params)
		assert.NoError(t, err)

		pathType := networkingv1.PathTypePrefix

		ingressRulesGot := got.Spec.Rules
		ingressRulesExpected := []networkingv1.IngressRule{
			{
				Host: "otlp-1.some.domain.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/opentelemetry.proto.collector.logs.v1.LogsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.traces.v1.TracesService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.metrics.v1.MetricsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-1-grpc",
										},
									},
								},
							},
						},
					},
				},
			},
			{
				Host: "otlp-2.some.domain.io",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/opentelemetry.proto.collector.logs.v1.LogsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.traces.v1.TracesService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
							{
								Path:     "/opentelemetry.proto.collector.metrics.v1.MetricsService",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test-collector-grpc",
										Port: networkingv1.ServiceBackendPort{
											Name: "otlp-2-grpc",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		assert.True(t, len(ingressRulesExpected) == len(ingressRulesGot))
		for i := 0; i < len(ingressRulesExpected); i++ {
			sort.Slice(ingressRulesGot, func(i, j int) bool { return ingressRulesGot[i].Host < ingressRulesGot[j].Host })
			sort.Slice(ingressRulesExpected, func(i, j int) bool { return ingressRulesExpected[i].Host < ingressRulesExpected[j].Host })
			assert.True(t, ingressRulesExpected[i].Host == ingressRulesGot[i].Host)
		}

		for i := 0; i < len(ingressRulesExpected); i++ {
			pathsGot := ingressRulesGot[i].IngressRuleValue.HTTP.Paths
			pathsExpected := ingressRulesExpected[i].IngressRuleValue.HTTP.Paths
			sort.Slice(pathsGot, func(i, j int) bool { return pathsGot[i].Path < pathsGot[j].Path })
			sort.Slice(pathsExpected, func(i, j int) bool { return pathsExpected[i].Path < pathsExpected[j].Path })

			assert.True(t, ingressRulesExpected[i].Host == ingressRulesGot[i].Host)
			assert.Equal(t, ingressRulesExpected, ingressRulesGot)
		}
	})
	t.Run("multiple grpc receivers, ALL host-to-component mapping records absent", func(t *testing.T) {
		var (
			ns               = "test"
			hostname         = "example.com"
			ingressClassName = "aws"
		)

		params, err := newParams("something:tag", testFileIngressAws)
		if err != nil {
			t.Fatal(err)
		}

		params.OtelCol.Namespace = ns
		params.OtelCol.Spec.Ingress = v1beta1.Ingress{
			Type:               v1beta1.IngressTypeIngress,
			Hostname:           hostname,
			Annotations:        map[string]string{"some.key": "some.value"},
			IngressClassName:   &ingressClassName,
			CollectorEndpoints: map[string]string{},
		}

		got, err := IngressAws(params)
		assert.Error(t, err)
		assert.Equal(t, err, errors.New("empty components to hostnames mapping"))
		assert.Nil(t, got)
	})
}
