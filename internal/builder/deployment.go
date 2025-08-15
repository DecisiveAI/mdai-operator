package builder

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentBuilder struct {
	deploy *appsv1.Deployment
}

func Deployment(d *appsv1.Deployment) *DeploymentBuilder {
	return &DeploymentBuilder{deploy: d}
}

// WithLabel ensures Labels is non-nil and sets a key/value.
func (b *DeploymentBuilder) WithLabel(key, value string) *DeploymentBuilder {
	if b.deploy.Labels == nil {
		b.deploy.Labels = make(map[string]string)
	}
	b.deploy.Labels[key] = value
	return b
}

// WithSelectorLabel ensures Spec.Selector.MatchLabels is non-nil and sets a key/value.
func (b *DeploymentBuilder) WithSelectorLabel(key, value string) *DeploymentBuilder {
	if b.deploy.Spec.Selector == nil {
		b.deploy.Spec.Selector = &metav1.LabelSelector{}
	}
	if b.deploy.Spec.Selector.MatchLabels == nil {
		b.deploy.Spec.Selector.MatchLabels = make(map[string]string)
	}
	b.deploy.Spec.Selector.MatchLabels[key] = value
	return b
}

func (b *DeploymentBuilder) WithTemplateLabel(key, value string) *DeploymentBuilder {
	if b.deploy.Spec.Template.Labels == nil {
		b.deploy.Spec.Template.Labels = make(map[string]string)
	}
	b.deploy.Spec.Template.Labels[key] = value
	return b
}

func (b *DeploymentBuilder) WithTemplateAnnotation(key, value string) *DeploymentBuilder {
	if b.deploy.Spec.Template.Annotations == nil {
		b.deploy.Spec.Template.Annotations = make(map[string]string)
	}
	b.deploy.Spec.Template.Annotations[key] = value
	return b
}

func (b *DeploymentBuilder) WithReplicas(n int32) *DeploymentBuilder {
	b.deploy.Spec.Replicas = &n
	return b
}

func (b *DeploymentBuilder) WithServiceAccount(name string) *DeploymentBuilder {
	b.deploy.Spec.Template.Spec.ServiceAccountName = name
	return b
}

func (b *DeploymentBuilder) WithContainers(containers ...corev1.Container) *DeploymentBuilder {
	b.deploy.Spec.Template.Spec.Containers = containers
	return b
}

func (b *DeploymentBuilder) WithVolumes(volumes ...corev1.Volume) *DeploymentBuilder {
	b.deploy.Spec.Template.Spec.Volumes = volumes
	return b
}
