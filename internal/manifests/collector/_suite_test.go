// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"fmt"
	"os"

	go_yaml "github.com/goccy/go-yaml"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
)

var (
	testLogger  = logf.Log.WithName("unit-tests")
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
	cfg2 := config.Config{}

	return manifests.Params{
		Config: cfg2,
		OtelCol: v1beta1.OpenTelemetryCollector{
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
		Log:      testLogger,
		Recorder: record.NewFakeRecorder(10),
	}
}

func newParams(file string, cfg *config.Config) (manifests.Params, error) {
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

	if cfg == nil {
		cfg = &config.Config{}
	}

	params := manifests.Params{
		Config: *cfg,
		OtelCol: v1beta1.OpenTelemetryCollector{
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
				Mode: v1beta1.ModeStatefulSet,
				TargetAllocator: v1beta1.TargetAllocatorEmbedded{
					Enabled: true,
				},
				Config: colCfg,
			},
		},
		Log: testLogger,
	}
	return params, nil
}
