// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/decisiveai/mdai-operator/internal/manifests"
	//"github.com/decisiveai/mdai-operator/internal/manifests/manifestutils"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

func Ingress(params manifests.Params) (*networkingv1.Ingress, error) {
	// mydecisive
	if params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.Type == "" && params.OtelMdaiIngressComb.MdaiIngress.Spec.CloudType == mdaiv1.CloudProviderAws {
		return IngressAws(params)
	} else {
		return nil, nil
	}

}

func servicePortsFromCfg(logger *zap.Logger, otelcol v1beta1.OpenTelemetryCollector) ([]corev1.ServicePort, error) {
	logrLogger := zapr.NewLogger(logger)
	ports, err := otelcol.Spec.Config.GetReceiverPorts(logrLogger)
	if err != nil {
		logger.Error("couldn't build the ingress for this instance", zap.Error(err))
		return nil, err
	}

	if len(otelcol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(otelcol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(logger, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}

		ports = append(toServicePorts(otelcol.Spec.Ports), resultingInferredPorts...)
	}

	return ports, nil
}

func toServicePorts(spec []v1beta1.PortsSpec) []corev1.ServicePort {
	var ports []corev1.ServicePort
	for _, p := range spec {
		ports = append(ports, p.ServicePort)
	}

	return ports
}
