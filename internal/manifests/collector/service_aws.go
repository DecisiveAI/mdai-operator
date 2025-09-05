package collector

import (
	"maps"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/mdai-operator/internal/manifests"
	"github.com/decisiveai/mdai-operator/internal/manifests/manifestutils"
	"github.com/decisiveai/mdai-operator/internal/naming"
	"github.com/go-logr/zapr"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mydecisive.
func GrpcService(params manifests.Params) (*corev1.Service, error) {
	// we need this service for aws only
	if !(params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.Type == "" && params.OtelMdaiIngressComb.MdaiIngress.Spec.CloudType == hubv1.CloudProviderAws) {
		return nil, nil
	}

	// TODO: rework labels & annotations
	name := naming.GrpcService(params.OtelMdaiIngressComb.Otelcol.Name)
	labels := manifestutils.Labels(params.OtelMdaiIngressComb.Otelcol.ObjectMeta, name, params.OtelMdaiIngressComb.Otelcol.Spec.Image, ComponentOpenTelemetryCollector, []string{})

	var annotations map[string]string
	annotations = maps.Clone(params.OtelMdaiIngressComb.Otelcol.Annotations)
	serviceAnnotations := params.OtelMdaiIngressComb.MdaiIngress.Spec.GrpcService.Annotations
	maps.Copy(annotations, serviceAnnotations)

	ports, err := servicePortsFromCfg(params.Log, params.OtelMdaiIngressComb.Otelcol)
	if err != nil {
		return nil, err
	}

	for i := len(ports) - 1; i >= 0; i-- {
		if ports[i].AppProtocol == nil || (ports[i].AppProtocol != nil && *ports[i].AppProtocol != "grpc") {
			ports = append(ports[:i], ports[i+1:]...)
		}
	}

	if len(params.OtelMdaiIngressComb.Otelcol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(params.OtelMdaiIngressComb.Otelcol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(params.Log, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}
		ports = append(toServicePorts(params.OtelMdaiIngressComb.Otelcol.Spec.Ports), resultingInferredPorts...)
	}

	// if we have no ports, we don't need a service
	if len(ports) == 0 {
		params.Log.Info("the instance's configuration didn't yield any ports to open, skipping service",
			zap.String("instance.name", params.OtelMdaiIngressComb.Otelcol.Name),
			zap.String("instance.namespace", params.OtelMdaiIngressComb.Otelcol.Namespace),
		)
		return nil, err
	}

	trafficPolicy := corev1.ServiceInternalTrafficPolicyCluster
	if params.OtelMdaiIngressComb.Otelcol.Spec.Mode == v1beta1.ModeDaemonSet {
		trafficPolicy = corev1.ServiceInternalTrafficPolicyLocal
	}

	serviceType := corev1.ServiceTypeClusterIP
	if params.OtelMdaiIngressComb.MdaiIngress.Spec.GrpcService != nil {
		if params.OtelMdaiIngressComb.MdaiIngress.Spec.GrpcService.Type != "" {
			serviceType = params.OtelMdaiIngressComb.MdaiIngress.Spec.GrpcService.Type
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelMdaiIngressComb.Otelcol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                  serviceType,
			InternalTrafficPolicy: &trafficPolicy,
			Selector:              manifestutils.SelectorLabels(params.OtelMdaiIngressComb.Otelcol.ObjectMeta, ComponentOpenTelemetryCollector),
			Ports:                 ports,
		},
	}, nil
}

func NonGrpcService(params manifests.Params) (*corev1.Service, error) {

	logrLogger := zapr.NewLogger(params.Log)
	// we need this service for aws only
	if !(params.OtelMdaiIngressComb.Otelcol.Spec.Ingress.Type == "" && params.OtelMdaiIngressComb.MdaiIngress.Spec.CloudType == hubv1.CloudProviderAws) {
		return nil, nil
	}

	name := naming.NonGrpcService(params.OtelMdaiIngressComb.Otelcol.Name)
	labels := manifestutils.Labels(params.OtelMdaiIngressComb.Otelcol.ObjectMeta, name, params.OtelMdaiIngressComb.Otelcol.Spec.Image, ComponentOpenTelemetryCollector, []string{})

	//annotations, err := manifestutils.Annotations(params.OtelMdaiIngressComb.Otelcol, []string{})
	//if err != nil {
	//	return nil, err
	//}
	//
	//ingressAnnotations, err := manifestutils.NonGrpcServiceAnnotations(params.OtelMdaiIngressComb, []string{})
	//if err != nil {
	//	return nil, err
	//}

	var annotations map[string]string
	annotations = maps.Clone(params.OtelMdaiIngressComb.Otelcol.Annotations)
	serviceAnnotations := params.OtelMdaiIngressComb.MdaiIngress.Spec.NonGrpcService.Annotations

	maps.Copy(annotations, serviceAnnotations)

	ports, err := params.OtelMdaiIngressComb.Otelcol.Spec.Config.GetAllPorts(logrLogger)
	if err != nil {
		return nil, err
	}

	for i := len(ports) - 1; i >= 0; i-- {
		if ports[i].AppProtocol != nil && *ports[i].AppProtocol == "grpc" {
			ports = append(ports[:i], ports[i+1:]...)
		}
	}

	if len(params.OtelMdaiIngressComb.Otelcol.Spec.Ports) > 0 {
		// we should add all the ports from the CR
		// there are two cases where problems might occur:
		// 1) when the port number is already being used by a receiver
		// 2) same, but for the port name
		//
		// in the first case, we remove the port we inferred from the list
		// in the second case, we rename our inferred port to something like "port-%d"
		portNumbers, portNames := extractPortNumbersAndNames(params.OtelMdaiIngressComb.Otelcol.Spec.Ports)
		var resultingInferredPorts []corev1.ServicePort
		for _, inferred := range ports {
			if filtered := filterPort(params.Log, inferred, portNumbers, portNames); filtered != nil {
				resultingInferredPorts = append(resultingInferredPorts, *filtered)
			}
		}

		ports = append(toServicePorts(params.OtelMdaiIngressComb.Otelcol.Spec.Ports), resultingInferredPorts...)
	}

	// if we have no ports, we don't need a service
	if len(ports) == 0 {
		params.Log.Info("the instance's configuration didn't yield any ports to open, skipping service",
			zap.String("instance.name", params.OtelMdaiIngressComb.Otelcol.Name),
			zap.String("instance.namespace", params.OtelMdaiIngressComb.Otelcol.Namespace),
		)
		return nil, err
	}

	trafficPolicy := corev1.ServiceInternalTrafficPolicyCluster
	if params.OtelMdaiIngressComb.Otelcol.Spec.Mode == v1beta1.ModeDaemonSet {
		trafficPolicy = corev1.ServiceInternalTrafficPolicyLocal
	}

	serviceType := corev1.ServiceTypeClusterIP
	if params.OtelMdaiIngressComb.MdaiIngress.Spec.NonGrpcService != nil {
		if params.OtelMdaiIngressComb.MdaiIngress.Spec.NonGrpcService.Type != "" {
			serviceType = params.OtelMdaiIngressComb.MdaiIngress.Spec.NonGrpcService.Type
		}
	}

	var spec corev1.ServiceSpec
	spec = corev1.ServiceSpec{
		InternalTrafficPolicy: &trafficPolicy,
		Selector:              manifestutils.SelectorLabels(params.OtelMdaiIngressComb.Otelcol.ObjectMeta, ComponentOpenTelemetryCollector),
		Type:                  serviceType,
		Ports:                 ports,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   params.OtelMdaiIngressComb.Otelcol.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}, nil
}
