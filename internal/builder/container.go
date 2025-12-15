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
	b.container.Ports = ports
	return b
}

func (b *ContainerBuilder) WithVolumeMounts(mounts ...corev1.VolumeMount) *ContainerBuilder {
	b.container.VolumeMounts = append(b.container.VolumeMounts, mounts...)
	return b
}

func (b *ContainerBuilder) WithEnv(env ...corev1.EnvVar) *ContainerBuilder {
	b.container.Env = append(b.container.Env, env...)
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

func (b *ContainerBuilder) WithEnvFromConfigMap(configMapName string) *ContainerBuilder {
	b.container.EnvFrom = append(b.container.EnvFrom, corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
		},
	})
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

func (b *ContainerBuilder) WithEnvFromSecretKey(envVarName, secretName, key string) *ContainerBuilder {
	if secretName == "" || key == "" || envVarName == "" {
		return b
	}

	b.container.Env = append(b.container.Env, corev1.EnvVar{
		Name: envVarName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: key,
			},
		},
	})

	return b
}

func (b *ContainerBuilder) WithLivenessProbe(livenessProbe *corev1.Probe) *ContainerBuilder {
	b.container.LivenessProbe = livenessProbe
	return b
}

func (b *ContainerBuilder) WithReadinessProbe(readinessProbe *corev1.Probe) *ContainerBuilder {
	b.container.ReadinessProbe = readinessProbe
	return b
}

func (b *ContainerBuilder) WithImagePullPolicy(pullPolicy corev1.PullPolicy) *ContainerBuilder {
	b.container.ImagePullPolicy = pullPolicy
	return b
}

func (b *ContainerBuilder) Build() corev1.Container {
	return b.container
}
