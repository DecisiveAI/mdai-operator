package builder

import corev1 "k8s.io/api/core/v1"

type ContainerBuilder struct {
	container corev1.Container
}

func Container(name, image string) *ContainerBuilder {
	return &ContainerBuilder{
		container: corev1.Container{
			Name:  name,
			Image: image,
		},
	}
}

func (b *ContainerBuilder) WithCommand(cmd ...string) *ContainerBuilder {
	b.container.Command = cmd
	return b
}

func (b *ContainerBuilder) WithPorts(ports ...corev1.ContainerPort) *ContainerBuilder {
	b.container.Ports = append(b.container.Ports, ports...)
	return b
}

func (b *ContainerBuilder) WithVolumeMounts(mounts ...corev1.VolumeMount) *ContainerBuilder {
	b.container.VolumeMounts = append(b.container.VolumeMounts, mounts...)
	return b
}

func (b *ContainerBuilder) WithEnvFrom(envFrom ...corev1.EnvFromSource) *ContainerBuilder {
	b.container.EnvFrom = append(b.container.EnvFrom, envFrom...)
	return b
}

func (b *ContainerBuilder) WithSecurityContext(sc *corev1.SecurityContext) *ContainerBuilder {
	b.container.SecurityContext = sc
	return b
}

func (b *ContainerBuilder) WithEnvFromSecret(secretName string) *ContainerBuilder {
	b.container.EnvFrom = append(b.container.EnvFrom, corev1.EnvFromSource{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
		},
	})
	return b
}

func (b *ContainerBuilder) WithAWSSecret(secretName *string) *ContainerBuilder {
	if secretName != nil {
		b.container.EnvFrom = append(b.container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: *secretName,
				},
			},
		})
	}
	return b
}

func (b *ContainerBuilder) Build() corev1.Container {
	return b.container
}
