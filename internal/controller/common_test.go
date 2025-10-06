// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"testing"

	goyaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

var (
	basePolicy     = corev1.ServiceInternalTrafficPolicyCluster
	pathTypePrefix = networkingv1.PathTypePrefix

	selectorLabels = map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "test.test",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
	}
	grpcServiceLabels = map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/instance":   "test.test",
		"app.kubernetes.io/version":    "latest",
		"app.kubernetes.io/name":       "test-collector-grpc",
	}
	nonGrpcServiceLabels = map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/instance":   "test.test",
		"app.kubernetes.io/version":    "latest",
		"app.kubernetes.io/name":       "test-collector-non-grpc",
	}

	http = "http"
	grpc = "grpc"
)

// nolint: gofumpt
func TestBuildCollector(t *testing.T) {
	var goodConfigYaml = `receivers:
  jaeger:
     protocols:
        grpc:
          endpoint: 0.0.0.0:14260
  otlp:
     protocols:
        http:
          endpoint: 0.0.0.0:4318
exporters:
  debug:
service:
  pipelines:
    metrics:
      receivers: [otlp, jaeger]
      exporters: [debug]
`

	var ingressYaml = `apiVersion: hub.mydecisive.ai/v1
kind: MdaiIngress
metadata:
  name: test
  namespace: test
spec:
  cloudType: aws
  annotations:
    ingress.annotation: ingress_annotation_value
  grpcService:
    type: NodePort
    annotations:
      grpc.service.annotation: grpc_service_annotation_value
  nonGrpcService:
    type: LoadBalancer
    annotations:
      non.grpc.service.annotation: non_grpc_service_annotation_value
  collectorEndpoints:
    otlp: otlp.mdai.io
    jaeger: jaeger.mdai.io
`

	goodConfig := v1beta1.Config{}
	err := goyaml.Unmarshal([]byte(goodConfigYaml), &goodConfig)
	require.NoError(t, err)

	mdaiIngress := mdaiv1.MdaiIngress{}
	err = goyaml.Unmarshal([]byte(ingressYaml), &mdaiIngress)
	require.NoError(t, err)

	one := int32(1)
	type args struct {
		instance mdaiv1.OtelMdaiIngressComb
	}
	tests := []struct {
		name    string
		args    args
		want    []client.Object
		wantErr bool
	}{
		{
			name: "base case",
			args: args{
				instance: mdaiv1.OtelMdaiIngressComb{
					Otelcol: v1beta1.OpenTelemetryCollector{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "test",
						},
						Spec: v1beta1.OpenTelemetryCollectorSpec{
							OpenTelemetryCommonFields: v1beta1.OpenTelemetryCommonFields{
								Image:    "test",
								Replicas: &one,
							},
							Mode:   "deployment",
							Config: goodConfig,
						},
					},
					MdaiIngress: mdaiIngress,
				},
			},
			want: []client.Object{
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-collector-grpc",
						Namespace: "test",
						Annotations: map[string]string{
							"grpc.service.annotation": "grpc_service_annotation_value",
						},
						Labels: grpcServiceLabels,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "jaeger-grpc",
								Port: 14260,
								TargetPort: intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 14260,
									StrVal: "",
								},
								Protocol:    corev1.ProtocolTCP,
								AppProtocol: &grpc,
								NodePort:    0,
							},
						},
						Selector:              selectorLabels,
						ClusterIP:             "",
						InternalTrafficPolicy: &basePolicy,
						Type:                  corev1.ServiceTypeNodePort,
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-collector-non-grpc",
						Namespace: "test",
						Annotations: map[string]string{
							"non.grpc.service.annotation": "non_grpc_service_annotation_value",
						},
						Labels: nonGrpcServiceLabels,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "otlp-http",
								Port: 4318,
								TargetPort: intstr.IntOrString{
									Type:   intstr.Int,
									IntVal: 4318,
									StrVal: "",
								},
								Protocol:    "",
								AppProtocol: &http,
							},
						},
						Selector:              selectorLabels,
						ClusterIP:             "",
						InternalTrafficPolicy: &basePolicy,
						Type:                  corev1.ServiceTypeLoadBalancer,
					},
				},
				&networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ingress",
						Namespace: "test",
						Annotations: map[string]string{
							"ingress.annotation": "ingress_annotation_value",
						},
					},
					Spec: networkingv1.IngressSpec{
						Rules: []networkingv1.IngressRule{
							{
								Host: "jaeger.mdai.io",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												Path:     "/jaeger.api_v2/CollectorService",
												PathType: &pathTypePrefix,
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
												PathType: &pathTypePrefix,
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
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := manifests.Params{
				Log:                 zap.NewNop(),
				OtelMdaiIngressComb: tt.args.instance,
			}
			got, err := BuildCollector(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.ElementsMatch(t, tt.want, got)
		})
	}
}
