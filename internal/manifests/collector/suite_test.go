// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"fmt"
	"os"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	go_yaml "github.com/goccy/go-yaml"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

var (
	testLogger  = zap.NewNop()
	instanceUID = uuid.NewUUID()
)

const (
	defaultCollectorImage    = "default-collector"
	defaultTaAllocationImage = "default-ta-allocator"
)

func deploymentParams() manifests.Params {
	return paramsWithMode(v1beta1.ModeDeployment)
}

func paramsWithMode(mode v1beta1.Mode) manifests.Params {
	replicas := int32(2)
	configYAML, err := os.ReadFile("testdata/test.yaml")
	if err != nil {
		fmt.Printf("Error getting yaml file: %v", err)
	}
	cfg := v1beta1.Config{}
	err = go_yaml.Unmarshal(configYAML, &cfg)
	if err != nil {
		fmt.Printf("Error unmarshalling YAML: %v", err)
	}

	return manifests.Params{
		OtelMdaiIngressComb: mdaiv1.OtelMdaiIngressComb{
			Otelcol: v1beta1.OpenTelemetryCollector{
				TypeMeta: metav1.TypeMeta{
					Kind:       "opentelemetry.io",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					UID:       instanceUID,
				},
				Spec: v1beta1.OpenTelemetryCollectorSpec{
					OpenTelemetryCommonFields: v1beta1.OpenTelemetryCommonFields{

						Image: "ghcr.io/open-telemetry/opentelemetry-operator/opentelemetry-operator:0.47.0",
						Ports: []v1beta1.PortsSpec{
							{
								ServicePort: v1.ServicePort{
									Name: "web",
									Port: 80,
									TargetPort: intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: 80,
									},
									NodePort: 0,
								},
							},
						},
						Replicas: &replicas,
					},
					Config: cfg,
					Mode:   mode,
				},
			},
			MdaiIngress: mdaiv1.MdaiIngress{},
		},
		Log: testLogger,
	}
}

func newParams(file string) (manifests.Params, error) {
	replicas := int32(1)
	var configYAML []byte
	var err error

	if file == "" {
		configYAML, err = os.ReadFile("testdata/test.yaml")
	} else {
		configYAML, err = os.ReadFile(file)
	}
	if err != nil {
		return manifests.Params{}, fmt.Errorf("error getting yaml file: %w", err)
	}

	colCfg := v1beta1.Config{}
	err = go_yaml.Unmarshal(configYAML, &colCfg)
	if err != nil {
		return manifests.Params{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	params := manifests.Params{
		OtelMdaiIngressComb: mdaiv1.OtelMdaiIngressComb{
			Otelcol: v1beta1.OpenTelemetryCollector{
				TypeMeta: metav1.TypeMeta{
					Kind:       "opentelemetry.io",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					UID:       instanceUID,
				},
				Spec: v1beta1.OpenTelemetryCollectorSpec{
					OpenTelemetryCommonFields: v1beta1.OpenTelemetryCommonFields{
						Ports: []v1beta1.PortsSpec{
							{
								ServicePort: v1.ServicePort{
									Name: "web",
									Port: 80,
									TargetPort: intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: 80,
									},
									NodePort: 0,
								},
							},
						},

						Replicas: &replicas,
					},
					Mode:   v1beta1.ModeDeployment,
					Config: colCfg,
				},
			},
			MdaiIngress: mdaiv1.MdaiIngress{
				Spec: mdaiv1.MdaiIngressSpec{
					GrpcService: &mdaiv1.IngressService{
						Type: v1.ServiceTypeClusterIP,
					},
					NonGrpcService: &mdaiv1.IngressService{
						Type: v1.ServiceTypeClusterIP,
					},
				}},
		},
		Log: testLogger,
	}
	return params, nil
}
