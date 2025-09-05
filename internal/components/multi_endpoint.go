// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package components

import (
	"fmt"

	//"github.com/go-logr/logr"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/decisiveai/mdai-operator/internal/naming"
)

var _ Parser = &MultiPortReceiver{}

// MultiProtocolEndpointConfig represents the minimal struct for a given YAML configuration input containing a map to
// a struct with either endpoint or listen_address.
type MultiProtocolEndpointConfig struct {
	Protocols map[string]*SingleEndpointConfig `mapstructure:"protocols"`
}

// MultiPortOption allows the setting of options for a MultiPortReceiver.
type MultiPortOption func(parser *MultiPortReceiver)

// MultiPortReceiver is a special parser for components with endpoints for each protocol.
type MultiPortReceiver struct {
	name           string
	defaultRecAddr string

	addrMappings    map[string]string
	portMappings    map[string]*corev1.ServicePort
	urlPathsMapping map[string]*[]string
}

func (m *MultiPortReceiver) Ports(logger *zap.Logger, name string, config interface{}) ([]corev1.ServicePort, error) {
	multiProtoEndpointCfg := &MultiProtocolEndpointConfig{}
	if err := mapstructure.Decode(config, multiProtoEndpointCfg); err != nil {
		return nil, err
	}
	var ports []corev1.ServicePort
	for protocol, ec := range multiProtoEndpointCfg.Protocols {
		if defaultSvc, ok := m.portMappings[protocol]; ok {
			port := defaultSvc.Port
			if ec != nil {
				port = ec.GetPortNumOrDefault(logger, port)
			}
			defaultSvc.Name = naming.PortName(fmt.Sprintf("%s-%s", name, protocol), port)
			ports = append(ports, ConstructServicePort(defaultSvc, port))
		} else {
			return nil, fmt.Errorf("unknown protocol set: %s", protocol)
		}
	}
	return ports, nil
}

func (m *MultiPortReceiver) ParserType() string {
	return ComponentType(m.name)
}

func (m *MultiPortReceiver) ParserName() string {
	return fmt.Sprintf("__%s", m.name)
}

func (m *MultiPortReceiver) GetDefaultConfig(logger *zap.Logger, config interface{}) (interface{}, error) {
	multiProtoEndpointCfg := &MultiProtocolEndpointConfig{}
	if err := mapstructure.Decode(config, multiProtoEndpointCfg); err != nil {
		return nil, err
	}
	defaultedConfig := map[string]interface{}{}
	for protocol, ec := range multiProtoEndpointCfg.Protocols {
		if defaultSvc, ok := m.portMappings[protocol]; ok {
			port := defaultSvc.Port
			if ec != nil {
				port = ec.GetPortNumOrDefault(logger, port)
			}
			addr := m.defaultRecAddr
			if defaultAddr, ok := m.addrMappings[protocol]; ok {
				addr = defaultAddr
			}
			conf, err := AddressDefaulter(logger, addr, port, ec)
			if err != nil {
				return nil, err
			}
			defaultedConfig[protocol] = conf
		} else {
			return nil, fmt.Errorf("unknown protocol set: %s", protocol)
		}
	}
	return map[string]interface{}{
		"protocols": defaultedConfig,
	}, nil
}

func (m *MultiPortReceiver) GetLivenessProbe(logger *zap.Logger, config interface{}) (*corev1.Probe, error) {
	return nil, nil
}

func (m *MultiPortReceiver) GetReadinessProbe(logger *zap.Logger, config interface{}) (*corev1.Probe, error) {
	return nil, nil
}

func (m *MultiPortReceiver) GetRBACRules(*zap.Logger, interface{}) ([]rbacv1.PolicyRule, error) {
	return nil, nil
}

func (m *MultiPortReceiver) GetEnvironmentVariables(logger *zap.Logger, config interface{}) ([]corev1.EnvVar, error) {
	return nil, nil
}

type MultiPortBuilder[ComponentConfigType any] []Builder[ComponentConfigType]

func NewMultiPortReceiverBuilder(name string) MultiPortBuilder[*MultiProtocolEndpointConfig] {
	return append(MultiPortBuilder[*MultiProtocolEndpointConfig]{}, NewBuilder[*MultiProtocolEndpointConfig]().WithName(name).WithDefaultRecAddress(DefaultRecAddress))
}

func NewProtocolBuilder(name string, port int32) Builder[*MultiProtocolEndpointConfig] {
	return NewBuilder[*MultiProtocolEndpointConfig]().WithName(name).WithPort(port).WithDefaultRecAddress(DefaultRecAddress)
}

func (mp MultiPortBuilder[ComponentConfigType]) AddPortMapping(builder Builder[ComponentConfigType]) MultiPortBuilder[ComponentConfigType] {
	return append(mp, builder)
}

func (mp MultiPortBuilder[ComponentConfigType]) Build() (*MultiPortReceiver, error) {
	if len(mp) < 1 {
		return nil, fmt.Errorf("must provide at least one port mapping")
	}

	mb := mp[0].MustBuild()
	multiReceiver := &MultiPortReceiver{
		name:            mb.name,
		defaultRecAddr:  mb.settings.defaultRecAddr,
		addrMappings:    map[string]string{},
		portMappings:    map[string]*corev1.ServicePort{},
		urlPathsMapping: map[string]*[]string{},
	}
	for _, bu := range mp[1:] {
		built, err := bu.Build()
		if err != nil {
			return nil, err
		}
		if built.settings != nil {
			multiReceiver.portMappings[built.name] = built.settings.GetServicePort()
			multiReceiver.addrMappings[built.name] = built.settings.defaultRecAddr
			multiReceiver.urlPathsMapping[built.name] = &built.settings.urlPaths
		}
	}
	return multiReceiver, nil
}

func (mp MultiPortBuilder[ComponentConfigType]) MustBuild() *MultiPortReceiver {
	if p, err := mp.Build(); err != nil {
		panic(err)
	} else {
		return p
	}
}

func (m *MultiPortReceiver) PortsWithUrlPaths(logger *zap.Logger, name string, config interface{}) ([]PortUrlPaths, error) {
	multiProtoEndpointCfg := &MultiProtocolEndpointConfig{}
	if err := mapstructure.Decode(config, multiProtoEndpointCfg); err != nil {
		return []PortUrlPaths{}, nil
	}
	portsUrlParts := []PortUrlPaths{}
	for protocol, ec := range multiProtoEndpointCfg.Protocols {
		if defaultSvc, ok := m.portMappings[protocol]; ok {
			port := defaultSvc.Port
			if ec != nil {
				port = ec.GetPortNumOrDefault(logger, port)
			}
			defaultSvc.Name = naming.PortName(fmt.Sprintf("%s-%s", name, protocol), port)
			// TODO since this is used for gRPC only, we actually dont need to construct this, as gRPC urls are hardcoded
			portsUrlParts = append(portsUrlParts, PortUrlPaths{ConstructServicePort(defaultSvc, port), *m.urlPathsMapping[protocol]})
		} else {
			return nil, fmt.Errorf("unknown protocol set: %s", protocol)
		}
	}
	return portsUrlParts, nil
}
