// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/decisiveai/mdai-operator/internal/manifests"
)

const (
	ComponentOpenTelemetryCollector = "opentelemetry-collector"
)

// Build creates the manifest for the collector resource.
func Build(params manifests.Params) ([]client.Object, error) {
	var resourceManifests []client.Object
	var manifestFactories []manifests.K8sManifestFactory[manifests.Params]
	manifestFactories = append(manifestFactories, []manifests.K8sManifestFactory[manifests.Params]{
		manifests.Factory(Ingress),
		manifests.Factory(GrpcService),
		manifests.Factory(NonGrpcService),
	}...)

	for _, factory := range manifestFactories {
		res, err := factory(params)
		if err != nil {
			return nil, err
		} else if manifests.ObjectIsNotNil(res) {
			resourceManifests = append(resourceManifests, res)
		}
	}

	return resourceManifests, nil
}
