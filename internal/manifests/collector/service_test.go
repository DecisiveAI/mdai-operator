// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"testing"

	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestExtractPortNumbersAndNames(t *testing.T) {
	t.Run("should return extracted port names and numbers", func(t *testing.T) {
		ports := []v1beta1.PortsSpec{
			{ServicePort: v1.ServicePort{Name: "web", Port: 8080}},
			{ServicePort: v1.ServicePort{Name: "tcp", Port: 9200}},
			{ServicePort: v1.ServicePort{Name: "web-explicit", Port: 80, Protocol: v1.ProtocolTCP}},
			{ServicePort: v1.ServicePort{Name: "syslog-udp", Port: 514, Protocol: v1.ProtocolUDP}},
		}
		expectedPortNames := map[string]bool{"web": true, "tcp": true, "web-explicit": true, "syslog-udp": true}
		expectedPortNumbers := map[PortNumberKey]bool{
			newPortNumberKey(8080, v1.ProtocolTCP): true,
			newPortNumberKey(9200, v1.ProtocolTCP): true,
			newPortNumberKey(80, v1.ProtocolTCP):   true,
			newPortNumberKey(514, v1.ProtocolUDP):  true,
		}

		actualPortNumbers, actualPortNames := extractPortNumbersAndNames(ports)
		assert.Equal(t, expectedPortNames, actualPortNames)
		assert.Equal(t, expectedPortNumbers, actualPortNumbers)

	})
}

func TestFilterPort(t *testing.T) {

	tests := []struct {
		name        string
		candidate   v1.ServicePort
		portNumbers map[PortNumberKey]bool
		portNames   map[string]bool
		expected    v1.ServicePort
	}{
		{
			name:      "should filter out duplicate port",
			candidate: v1.ServicePort{Name: "web", Port: 8080},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
		},

		{
			name:      "should filter out duplicate port, protocol specified (TCP)",
			candidate: v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolTCP},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
		},

		{
			name:      "should filter out duplicate port, protocol specified (UDP)",
			candidate: v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolUDP},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKey(8080, v1.ProtocolUDP): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
		},

		{
			name:      "should not filter unique port",
			candidate: v1.ServicePort{Name: "web", Port: 8090},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
			expected:  v1.ServicePort{Name: "web", Port: 8090},
		},

		{
			name:      "should not filter same port with different protocols",
			candidate: v1.ServicePort{Name: "web", Port: 8080},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKey(8080, v1.ProtocolUDP): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
			expected:  v1.ServicePort{Name: "web", Port: 8080},
		},

		{
			name:      "should not filter same port with different protocols, candidate has specified port (TCP vs UDP)",
			candidate: v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolTCP},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKey(8080, v1.ProtocolUDP): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
			expected:  v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolTCP},
		},

		{
			name:      "should not filter same port with different protocols, candidate has specified port (UDP vs TCP)",
			candidate: v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolUDP},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"test": true, "metrics": true},
			expected:  v1.ServicePort{Name: "web", Port: 8080, Protocol: v1.ProtocolUDP},
		},

		{
			name:      "should change the duplicate portName",
			candidate: v1.ServicePort{Name: "web", Port: 8090},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"web": true, "metrics": true},
			expected:  v1.ServicePort{Name: "port-8090", Port: 8090},
		},

		{
			name:      "should return nil if fallback name clashes with existing portName",
			candidate: v1.ServicePort{Name: "web", Port: 8090},
			portNumbers: map[PortNumberKey]bool{
				newPortNumberKeyByPort(8080): true, newPortNumberKeyByPort(9200): true},
			portNames: map[string]bool{"web": true, "port-8090": true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := filterPort(testLogger, test.candidate, test.portNumbers, test.portNames)
			if test.expected != (v1.ServicePort{}) {
				assert.Equal(t, test.expected, *actual)
				return
			}
			assert.Nil(t, actual)

		})

	}
}
