package collector

import (
	"errors"
	"maps"
	"sort"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/mdai-operator/internal/components"
	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/decisiveai/mdai-operator/internal/naming"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"go.uber.org/zap"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func IngressAws(params manifests.Params) (*networkingv1.Ingress, error) {
	name := naming.Ingress(params.OtelMdaiIngressComb.Otelcol.Name)
	// TODO: rework labels & annotations
	labels := params.OtelMdaiIngressComb.MdaiIngress.Labels

	var annotations map[string]string
	annotations = maps.Clone(params.OtelMdaiIngressComb.Otelcol.Annotations)
	ingressAnnotations := params.OtelMdaiIngressComb.MdaiIngress.Spec.Annotations
	if annotations != nil && ingressAnnotations != nil {
		maps.Copy(annotations, ingressAnnotations)
	}

	var rules []networkingv1.IngressRule
	compPortsEndpoints, err := servicePortsUrlPathsFromCfg(params)
	if len(compPortsEndpoints) == 0 || err != nil {
		params.Log.Info(
			"the instance's configuration didn't yield any ports to open, skipping ingress",
			zap.String("instance.name", params.OtelMdaiIngressComb.Otelcol.Name),
			zap.String("instance.namespace", params.OtelMdaiIngressComb.Otelcol.Namespace),
		)
		return nil, err
	}
	for comp, portsEndpoints := range compPortsEndpoints {
		// deleting all non-grpc ports
		for i := len(portsEndpoints) - 1; i >= 0; i-- {
			if portsEndpoints[i].Port.AppProtocol == nil || (portsEndpoints[i].Port.AppProtocol != nil && *portsEndpoints[i].Port.AppProtocol != "grpc") {
				portsEndpoints = append(portsEndpoints[:i], portsEndpoints[i+1:]...)
			}
		}
		// if component does not have grpc ports, delete it from the result map
		if len(portsEndpoints) > 0 {
			compPortsEndpoints[comp] = portsEndpoints
		} else {
			delete(compPortsEndpoints, comp)

		}
	}
	// if we have no grpc ports, we don't need an ingress entry
	if len(compPortsEndpoints) == 0 {
		params.Log.Info(
			"the instance's configuration didn't yield any grpc ports to open, skipping ingress",
			zap.String("instance.name", params.OtelMdaiIngressComb.Otelcol.Name),
			zap.String("instance.namespace", params.OtelMdaiIngressComb.Otelcol.Namespace),
		)
		return nil, nil
	}

	switch params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.RuleType {
	case v1beta1.IngressRuleTypePath, "":
		if params.OtelMdaiIngressComb.MdaiIngress.Spec.CollectorEndpoints == nil || len(params.OtelMdaiIngressComb.MdaiIngress.Spec.CollectorEndpoints) == 0 {
			return nil, errors.New("empty components to hostnames mapping")
		} else {
			rules = createPathIngressRulesUrlPaths(params.Log, params.OtelMdaiIngressComb.Otelcol.Name, params.OtelMdaiIngressComb.MdaiIngress.Spec.CollectorEndpoints, compPortsEndpoints)
		}
	case v1beta1.IngressRuleTypeSubdomain:
		params.Log.Info("Only  IngressRuleType = \"path\" is supported for AWS",
			zap.String("ingress.type", string(hubv1.CloudProviderAws)),
			zap.String("ingress.ruleType", string(v1beta1.IngressRuleTypeSubdomain)),
		)
		return nil, err
	}
	if rules == nil || len(rules) == 0 {
		params.Log.Info(
			"could not configure any ingress rules for the instance's configuration, skipping ingress",
			zap.String("instance.name", params.OtelMdaiIngressComb.Otelcol.Name),
			zap.String("instance.namespace", params.OtelMdaiIngressComb.Otelcol.Namespace),
		)
		return nil, nil
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Host < rules[j].Host
	})

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelMdaiIngressComb.Otelcol.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: networkingv1.IngressSpec{
			TLS:              params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.TLS,
			Rules:            rules,
			IngressClassName: params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.IngressClassName,
		},
	}, nil
}

// mydecisive.
func createPathIngressRulesUrlPaths(logger *zap.Logger, otelcol string, colEndpoints map[string]string, compPortsUrlPaths components.ComponentsPortsUrlPaths) []networkingv1.IngressRule {
	pathType := networkingv1.PathTypePrefix
	var ingressRules []networkingv1.IngressRule
	for comp, portsUrlPaths := range compPortsUrlPaths {
		var totalPaths = 0
		for _, portUrlPaths := range portsUrlPaths {
			totalPaths += len(portUrlPaths.UrlPaths)
		}
		paths := make([]networkingv1.HTTPIngressPath, totalPaths)
		var i = 0
		for _, portUrlPaths := range portsUrlPaths {
			portName := naming.PortName(portUrlPaths.Port.Name, portUrlPaths.Port.Port)
			for _, endpoint := range portUrlPaths.UrlPaths {
				paths[i] = networkingv1.HTTPIngressPath{
					Path:     endpoint,
					PathType: &pathType,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: naming.GrpcService(otelcol),
							Port: networkingv1.ServiceBackendPort{
								Name: portName,
							},
						},
					},
				}
				i++
			}
			if host, ok := colEndpoints[comp]; ok {
				ingressRule := networkingv1.IngressRule{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: paths,
						},
					},
				}
				ingressRules = append(ingressRules, ingressRule)
			} else {
				err := errors.New("missing or invalid mapping")
				logger.Error("missing or invalid mapping for", zap.String("component", comp), zap.Error(err))
				continue
			}
		}
	}
	return ingressRules
}

// mydecisive.
func servicePortsUrlPathsFromCfg(params manifests.Params) (components.ComponentsPortsUrlPaths, error) {
	logger := params.Log
	portsUrlPaths, err := params.OtelMdaiIngressComb.GetReceiverPortsWithUrlPaths(logger)
	if err != nil {
		logger.Error("couldn't build the ingress for this instance", zap.Error(err))
		return nil, err
	}
	return portsUrlPaths, nil
}
