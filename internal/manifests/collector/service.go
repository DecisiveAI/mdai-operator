// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

type PortNumberKey struct {
	Port     int32
	Protocol corev1.Protocol
}

func newPortNumberKeyByPort(port int32) PortNumberKey {
	return PortNumberKey{Port: port, Protocol: corev1.ProtocolTCP}
}

func newPortNumberKey(port int32, protocol corev1.Protocol) PortNumberKey {
	if protocol == "" {
		// K8s defaults to TCP if protocol is empty, so evaluate the port the same
		protocol = corev1.ProtocolTCP
	}
	return PortNumberKey{Port: port, Protocol: protocol}
}

// filterPort filters service ports to avoid conflicts with user-specified ports.
// If the candidate port number is already in use, returns nil.
// If the candidate port name conflicts with an existing name, attempts to use a fallback name of format "port-{number}".
// If both the original name and fallback name are taken, returns nil with a warning log.
// Otherwise returns the (potentially renamed) candidate port.
func filterPort(logger *zap.Logger, candidate corev1.ServicePort, portNumbers map[PortNumberKey]bool, portNames map[string]bool) *corev1.ServicePort {
	if portNumbers[newPortNumberKey(candidate.Port, candidate.Protocol)] {
		return nil
	}

	// do we have the port name there already?
	if portNames[candidate.Name] {
		// there's already a port with the same name! do we have a 'port-%d' already?
		fallbackName := fmt.Sprintf("port-%d", candidate.Port)
		if portNames[fallbackName] {
			// that wasn't expected, better skip this port
			logger.Info("a port name specified in the CR clashes with an inferred port name, and the fallback port name clashes with another port name! Skipping this port.",
				zap.String("inferred-port-name", candidate.Name),
				zap.String("fallback-port-name", fallbackName),
				zap.String("type", "warning"),
			)
			return nil
		}

		candidate.Name = fallbackName
		return &candidate
	}

	// this port is unique, return as is
	return &candidate
}

func extractPortNumbersAndNames(ports []v1beta1.PortsSpec) (map[PortNumberKey]bool, map[string]bool) {
	numbers := map[PortNumberKey]bool{}
	names := map[string]bool{}

	for _, port := range ports {
		numbers[newPortNumberKey(port.Port, port.Protocol)] = true
		names[port.Name] = true
	}

	return numbers, names
}
