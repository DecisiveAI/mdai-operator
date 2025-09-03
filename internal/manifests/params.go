// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package manifests

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
)

// Params holds the reconciliation-specific parameters.
type Params struct {
	Client client.Client
	Scheme *runtime.Scheme
	// TODO: replace with zap logger
	Log logr.Logger
	//Log     *zap.Logger
	OtelMdaiIngressComb mdaiv1.OtelMdaiIngressComb
}
