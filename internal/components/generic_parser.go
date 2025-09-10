// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// nolint:gofumpt
package components

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

var (
	_ Parser = &GenericParser[SingleEndpointConfig]{}
)

// GenericParser serves as scaffolding for custom parsing logic by isolating
// functionality to idempotent functions.
type GenericParser[T any] struct {
	name            string
	settings        *Settings[T]
	portParser      PortParser[T]
	rbacGen         RBACRuleGenerator[T]
	envVarGen       EnvVarGenerator[T]
	livenessGen     ProbeGenerator[T]
	readinessGen    ProbeGenerator[T]
	defaultsApplier Defaulter[T]
}

func (g *GenericParser[T]) GetDefaultConfig(logger *zap.Logger, config any) (any, error) {
	if g.settings == nil || g.defaultsApplier == nil {
		return config, nil
	}

	if g.settings.defaultRecAddr == "" || g.settings.port == 0 {
		return config, nil
	}

	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.defaultsApplier(logger, g.settings.defaultRecAddr, g.settings.port, parsed)
}

func (g *GenericParser[T]) GetLivenessProbe(logger *zap.Logger, config any) (*corev1.Probe, error) {
	if g.livenessGen == nil {
		return nil, nil //nolint:nilnil
	}
	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.livenessGen(logger, parsed)
}

func (g *GenericParser[T]) GetReadinessProbe(logger *zap.Logger, config any) (*corev1.Probe, error) {
	if g.readinessGen == nil {
		return nil, nil //nolint:nilnil
	}
	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.readinessGen(logger, parsed)
}

func (g *GenericParser[T]) GetRBACRules(logger *zap.Logger, config any) ([]rbacv1.PolicyRule, error) {
	if g.rbacGen == nil {
		return nil, nil
	}
	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.rbacGen(logger, parsed)
}

func (g *GenericParser[T]) GetEnvironmentVariables(logger *zap.Logger, config any) ([]corev1.EnvVar, error) {
	if g.envVarGen == nil {
		return nil, nil
	}
	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.envVarGen(logger, parsed)
}

func (g *GenericParser[T]) Ports(logger *zap.Logger, name string, config any) ([]corev1.ServicePort, error) {
	if g.portParser == nil {
		return nil, nil
	}
	var parsed T
	if err := mapstructure.Decode(config, &parsed); err != nil {
		return nil, err
	}
	return g.portParser(logger, name, g.settings.GetServicePort(), parsed)
}

// mydecisive
//
//revive:disable-next-line:unused-receiver
func (g *GenericParser[T]) PortsWithUrlPaths(logger *zap.Logger, name string, config any) ([]PortUrlPaths, error) {
	return []PortUrlPaths{}, nil
}

func (g *GenericParser[T]) ParserType() string {
	return ComponentType(g.name)
}

func (g *GenericParser[T]) ParserName() string {
	// nolint:perfsprint
	return fmt.Sprintf("__%s", g.name)
}
