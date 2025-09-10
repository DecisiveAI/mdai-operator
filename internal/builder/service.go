package builder

import (
	corev1 "k8s.io/api/core/v1"
)

type ServiceBuilder struct {
	svc *corev1.Service
}

func Service(svc *corev1.Service) *ServiceBuilder {
	return &ServiceBuilder{svc: svc}
}

func (b *ServiceBuilder) WithLabel(key, value string) *ServiceBuilder {
	if b.svc.Labels == nil {
		b.svc.Labels = make(map[string]string)
	}
	b.svc.Labels[key] = value
	return b
}

func (b *ServiceBuilder) WithSelectorLabel(key, value string) *ServiceBuilder {
	if b.svc.Spec.Selector == nil {
		b.svc.Spec.Selector = make(map[string]string)
	}
	b.svc.Spec.Selector[key] = value
	return b
}

func (b *ServiceBuilder) WithPorts(ports ...corev1.ServicePort) *ServiceBuilder {
	b.svc.Spec.Ports = ports
	return b
}

func (b *ServiceBuilder) WithType(svcType corev1.ServiceType) *ServiceBuilder {
	b.svc.Spec.Type = svcType
	return b
}
