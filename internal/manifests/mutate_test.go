// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package manifests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoImmutableLabelChange(t *testing.T) {
	existingSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/part-of":    "opentelemetry",
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/part-of":    "opentelemetry",
		"extra-label":                  "true",
	}
	err := hasImmutableLabelChange(existingSelectorLabels, desiredLabels)
	require.NoError(t, err)
	assert.NoError(t, err)
}

func TestHasImmutableLabelChange(t *testing.T) {
	existingSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/part-of":    "opentelemetry",
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/part-of":    "not-opentelemetry",
	}
	err := hasImmutableLabelChange(existingSelectorLabels, desiredLabels)
	assert.Error(t, err)
}

func TestMissingImmutableLabelChange(t *testing.T) {
	existingSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
		"app.kubernetes.io/part-of":    "opentelemetry",
	}
	desiredLabels := map[string]string{
		"app.kubernetes.io/component":  "opentelemetry-collector",
		"app.kubernetes.io/instance":   "default.deployment",
		"app.kubernetes.io/managed-by": "opentelemetry-operator",
	}
	err := hasImmutableLabelChange(existingSelectorLabels, desiredLabels)
	assert.Error(t, err)
}
