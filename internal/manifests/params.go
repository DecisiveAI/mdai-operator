// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package manifests

import (
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
)

// Params holds the reconciliation-specific parameters.
type Params struct {
	Client              client.Client
	Scheme              *runtime.Scheme
	Log                 *zap.Logger
	OtelMdaiIngressComb mdaiv1.OtelMdaiIngressComb
}
