// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package components

import (
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/decisiveai/mdai-operator/internal/naming"
)

const DefaultRecAddress = "0.0.0.0"

var (
	_ Parser = &GenericParser[*SingleEndpointConfig]{}
)

// SingleEndpointConfig represents the minimal struct for a given YAML configuration input containing either
// endpoint or listen_address.
type SingleEndpointConfig struct {
	Endpoint      string `mapstructure:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	ListenAddress string `mapstructure:"listen_address,omitempty" yaml:"listen_address,omitempty"`
}

func (g *SingleEndpointConfig) GetPortNumOrDefault(logger *zap.Logger, p int32) int32 {
	num, err := g.GetPortNum()
	if err != nil {
		logger.Info("no port set, using default", zap.Int32("port", p))
		return p
	}
	return num
}

// GetPortNum attempts to get the port for the given config. If it cannot, the UnsetPort and the given missingPortError
// are returned.
func (g *SingleEndpointConfig) GetPortNum() (int32, error) {
	if len(g.Endpoint) > 0 {
		return PortFromEndpoint(g.Endpoint)
	} else if len(g.ListenAddress) > 0 {
		return PortFromEndpoint(g.ListenAddress)
	}
	return UnsetPort, PortNotFoundErr
}

func ParseSingleEndpointSilent(logger *zap.Logger, name string, defaultPort *corev1.ServicePort, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	return internalParseSingleEndpoint(logger, name, true, defaultPort, singleEndpointConfig)
}

func ParseSingleEndpoint(logger *zap.Logger, name string, defaultPort *corev1.ServicePort, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	return internalParseSingleEndpoint(logger, name, false, defaultPort, singleEndpointConfig)
}

func internalParseSingleEndpoint(logger *zap.Logger, name string, failSilently bool, defaultPort *corev1.ServicePort, singleEndpointConfig *SingleEndpointConfig) ([]corev1.ServicePort, error) {
	if singleEndpointConfig == nil {
		return nil, nil
	}
	if _, err := singleEndpointConfig.GetPortNum(); err != nil && defaultPort.Port == UnsetPort {
		if failSilently {
			logger.Info("couldn't parse the endpoint's port and no default port set", zap.String("receiver", defaultPort.Name), zap.String("ierror:", err.Error()))
			err = nil
		} else {
			logger.Error("couldn't parse the endpoint's port and no default port set", zap.String("receiver", defaultPort.Name), zap.Error(err))
		}
		return []corev1.ServicePort{}, err
	}
	port := singleEndpointConfig.GetPortNumOrDefault(logger, defaultPort.Port)
	svcPort := defaultPort
	svcPort.Name = naming.PortName(name, port)
	return []corev1.ServicePort{ConstructServicePort(svcPort, port)}, nil
}

func NewSinglePortParserBuilder(name string, port int32) Builder[*SingleEndpointConfig] {
	return NewBuilder[*SingleEndpointConfig]().WithPort(port).WithName(name).WithPortParser(ParseSingleEndpoint).WithDefaultsApplier(AddressDefaulter).WithDefaultRecAddress(DefaultRecAddress)
}

func NewSilentSinglePortParserBuilder(name string, port int32) Builder[*SingleEndpointConfig] {
	return NewBuilder[*SingleEndpointConfig]().WithPort(port).WithName(name).WithPortParser(ParseSingleEndpointSilent).WithDefaultsApplier(AddressDefaulter).WithDefaultRecAddress(DefaultRecAddress)
}

func AddressDefaulter(logger *zap.Logger, defaultRecAddr string, port int32, config *SingleEndpointConfig) (map[string]interface{}, error) {
	if config == nil {
		config = &SingleEndpointConfig{}
	}

	if config.Endpoint == "" {
		config.Endpoint = fmt.Sprintf("%s:%d", defaultRecAddr, port)
	} else {
		v := strings.Split(config.Endpoint, ":")
		if len(v) < 2 || v[0] == "" {
			config.Endpoint = fmt.Sprintf("%s:%s", defaultRecAddr, v[len(v)-1])
		}
	}

	res := make(map[string]interface{})
	err := mapstructure.Decode(config, &res)
	return res, err
}
