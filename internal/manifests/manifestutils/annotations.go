// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package manifestutils

import (
	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
)

// Annotations return the annotations for OpenTelemetryCollector resources.
func Annotations(instance v1beta1.OpenTelemetryCollector, filterAnnotations []string) (map[string]string, error) {
	// new map every time, so that we don't touch the instance's annotations
	annotations := map[string]string{}

	if instance.ObjectMeta.Annotations != nil {
		for k, v := range instance.ObjectMeta.Annotations {
			if !IsFilteredSet(k, filterAnnotations) {
				annotations[k] = v
			}
		}
	}

	return annotations, nil
}

// mydecisive
// GrpcServiceAnnotations return the LbServiceAnnotations for OpenTelemetryCollector resources.
func GrpcServiceAnnotations(instanceComb hubv1.OtelMdaiIngressComb, filterAnnotations []string) (map[string]string, error) {
	// new map every time, so that we don't touch the instance's annotations
	annotations := map[string]string{}

	if nil != instanceComb.MdaiIngress.Spec.GrpcService {
		if nil != instanceComb.MdaiIngress.Spec.GrpcService.Annotations {
			for k, v := range instanceComb.MdaiIngress.Spec.GrpcService.Annotations {
				if !IsFilteredSet(k, filterAnnotations) {
					annotations[k] = v
				}
			}
		}
	}

	return annotations, nil
}

// mydecisive
// NonGrpcServiceAnnotations return the LbServiceAnnotations for OpenTelemetryCollector resources.
func NonGrpcServiceAnnotations(instanceComb hubv1.OtelMdaiIngressComb, filterAnnotations []string) (map[string]string, error) {
	// new map every time, so that we don't touch the instance's annotations
	annotations := map[string]string{}

	if nil != instanceComb.MdaiIngress.Spec.NonGrpcService {
		if nil != instanceComb.MdaiIngress.Spec.NonGrpcService.Annotations {
			for k, v := range instanceComb.MdaiIngress.Spec.NonGrpcService.Annotations {
				if !IsFilteredSet(k, filterAnnotations) {
					annotations[k] = v
				}
			}
		}
	}

	return annotations, nil
}

// mydecisive
// IngressAnnotations return the ingress annotations for OpenTelemetryCollector resources.
func IngressAnnotations(instance v1beta1.OpenTelemetryCollector, filterAnnotations []string) (map[string]string, error) {
	// new map every time, so that we don't touch the instance's annotations
	annotations := map[string]string{}

	if nil != instance.Spec.Ingress.Annotations {
		for k, v := range instance.Spec.Ingress.Annotations {
			if !IsFilteredSet(k, filterAnnotations) {
				annotations[k] = v
			}
		}
	}

	return annotations, nil
}
