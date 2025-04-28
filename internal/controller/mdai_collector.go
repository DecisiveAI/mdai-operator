package controller

import (
	"context"
	_ "embed"
	"fmt"
	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	mdaiCollectorHubComponent string = "mdai-collector"
)

var (
	//go:embed config/mdai_collector_base_config.yaml
	baseMdaiCollectorYAML string
)

func (c HubAdapter) ensureMdaiCollectorSynchronized(ctx context.Context) (OperationResult, error) {
	namespace := c.mdaiCR.Namespace
	hubConfig := c.mdaiCR.Spec.Config
	// TODO: Clean up these
	if hubConfig == nil {
		c.logger.Info("No hub config found, skipping mdai-collector synchronization")
		return ContinueProcessing()
	}

	telemetryConfig := hubConfig.Telemetry
	if telemetryConfig == nil {
		c.logger.Info("No mdai-collector telemetry config found, skipping mdai-collector synchronization")
		return ContinueProcessing()
	}

	awsConfig := telemetryConfig.AWSConfig
	if awsConfig == nil {
		c.logger.Info("No mdai-collector AWS telemetry config found, skipping mdai-collector synchronization")
		return ContinueProcessing()
	}

	logsConfig := telemetryConfig.Logs
	if logsConfig == nil {
		c.logger.Info("No mdai-collector logs config found, skipping mdai-collector synchronization")
		return ContinueProcessing()
	}

	collectorConfigMapName, hash, err := c.createOrUpdateMdaiCollectorConfigMap(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	collectorEnvConfigMapName, err := c.createOrUpdateMdaiCollectorEnvVarConfigMap(ctx, namespace, logsConfig.S3)
	if err != nil {
		return OperationResult{}, err
	}
	serviceAccountName, err := c.createOrUpdateMdaiCollectorServiceAccount(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	roleName, err := c.createOrUpdateMdaiCollectorRole(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	err = c.createOrUpdateMdaiCollectorRoleBinding(ctx, namespace, roleName, serviceAccountName)
	if err != nil {
		return OperationResult{}, err
	}

	if err := c.createOrUpdateMdaiCollectorDeployment(ctx, namespace, collectorConfigMapName, collectorEnvConfigMapName, serviceAccountName, hash); err != nil {
		if errors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		return OperationResult{}, err
	}

	if _, err := c.createOrUpdateMdaiCollectorService(ctx, namespace); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c HubAdapter) getMdaiCollectorConfig() (string, error) {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(baseMdaiCollectorYAML), &config); err != nil {
		c.logger.Error(err, "Failed to unmarshal base mdai collector config")
		return "", err
	}
	// TODO: Remove S3 portions if not used, or vice versa
	// we currently don't alter this at all, but will in the future, so we're still going to unmarshal it to make sure it is valid
	return baseMdaiCollectorYAML, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorConfigMap(ctx context.Context, namespace string) (string, string, error) {
	collectorYAML, err := c.getMdaiCollectorConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to build mdai-collector configuration: %w", err)
	}

	name := "mdai-collector-config"
	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.mdaiCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}
	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", name)
		return "", "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", name)
		return "", "", err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", name, "operation", operationResult)
	sha, err := getConfigMapSHA(*desiredConfigMap)
	return name, sha, err
}

func (c HubAdapter) createOrUpdateMdaiCollectorEnvVarConfigMap(ctx context.Context, namespace string, s3Config *v1.S3LogsConfig) (string, error) {
	name := "mdai-collector-env"

	data := map[string]string{
		"LOG_SEVERITY":  "SEVERITY_NUMBER_WARN",
		"K8S_NAMESPACE": namespace,
	}
	if s3Config != nil {
		data["AWS_LOG_REGION"] = *(s3Config.S3Region)
		data["AWS_LOG_BUCKET"] = *(s3Config.S3Bucket)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.mdaiCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
		Data: data,
	}
	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", name)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data = data
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", name)
		return "", err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", name, "operation", operationResult)
	return name, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorDeployment(ctx context.Context, namespace string, collectorConfigMapName string, collectorEnvConfigMapName string, serviceAccountName string, hash string) error {
	name := "mdai-collector"

	hubConfig := c.mdaiCR.Spec.Config
	if hubConfig == nil {
	}

	telemetryConfig := hubConfig.Telemetry
	if telemetryConfig == nil {
	}

	awsConfig := telemetryConfig.AWSConfig
	if awsConfig == nil {
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if err := controllerutil.SetControllerReference(c.mdaiCR, deployment, c.scheme); err != nil {
			c.logger.Error(err, "Failed to set owner reference on Deployment", "deployment", deployment.Name)
			return err
		}

		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels["app"] = name
		deployment.Labels[hubNameLabel] = c.mdaiCR.Name

		deployment.Spec.Replicas = int32Ptr(1)
		if deployment.Spec.Selector == nil {
			deployment.Spec.Selector = &metav1.LabelSelector{}
		}
		if deployment.Spec.Selector.MatchLabels == nil {
			deployment.Spec.Selector.MatchLabels = make(map[string]string)
		}
		deployment.Spec.Selector.MatchLabels["app"] = name

		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		deployment.Spec.Template.Labels["app"] = name
		deployment.Spec.Template.Labels["app.kubernetes.io/component"] = name

		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash

		deployment.Spec.Template.Spec.ServiceAccountName = serviceAccountName

		containerSpec := corev1.Container{
			Name:  name,
			Image: "public.ecr.aws/decisiveai/mdai-collector:0.1.4",
			Command: []string{
				"/mdai-collector",
				"--config=/conf/collector.yaml",
			},
			Ports: []corev1.ContainerPort{
				{ContainerPort: 4317, Name: "otlp-grpc"},
				{ContainerPort: 4318, Name: "otlp-http"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "config-volume",
					MountPath: "/conf/collector.yaml",
					SubPath:   "collector.yaml",
				},
			},
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: collectorEnvConfigMapName,
						},
					},
				},
			},
			SecurityContext: &corev1.SecurityContext{
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				RunAsNonRoot: ptr.To(true),
			},
		}
		deployment.Spec.Template.Spec.Containers = []corev1.Container{containerSpec}

		if awsConfig.AWSAccessKeySecret != nil {
			secretEnvSource := corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: *(awsConfig.AWSAccessKeySecret),
					},
				},
			}
			containerSpec.EnvFrom = append(containerSpec.EnvFrom, secretEnvSource)
		}

		deployment.Spec.Template.Spec.Containers = []corev1.Container{
			containerSpec,
		}

		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "config-volume",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: collectorConfigMapName,
						},
					},
				},
			},
		}

		return nil
	})
	if err != nil {
		return err
	}
	c.logger.Info("Deployment created or updated successfully", "deployment", deployment.Name, "operationResult", operationResult)

	return nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorService(ctx context.Context, namespace string) (string, error) {
	name := "mdai-collector-service"
	appLabel := "mdai-collector"

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", name)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		service.Labels["app"] = appLabel
		service.Labels[hubNameLabel] = c.mdaiCR.Name

		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabel,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "otlp-grpc",
					Protocol:   corev1.ProtocolTCP,
					Port:       4317,
					TargetPort: intstr.FromString("otlp-grpc"),
				},
				{
					Name:       "otlp-http",
					Protocol:   corev1.ProtocolTCP,
					Port:       4318,
					TargetPort: intstr.FromString("otlp-http"),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to create or update watcher-collector-service: %w", err)
	}

	c.logger.Info("Successfully created or updated watcher-collector-service", "service", name, "namespace", namespace, "operation", operationResult)
	return name, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorServiceAccount(ctx context.Context, namespace string) (string, error) {
	name := "mdai-collector-sa"

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, serviceAccount, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ServiceAccount", "serviceAccount", name)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, serviceAccount, func() error {
		return nil
	})

	if err != nil {
		return "", err
	}
	c.logger.Info("ServiceAccount created or updated successfully", "serviceAccount", serviceAccount.Name, "operationResult", operationResult)

	return name, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorRole(ctx context.Context, namespace string) (string, error) {
	name := "mdai-collector-role"

	// BEWARE: If you're thinking about making a yaml and unmarshaling it, to extract all this out... there's a weird problem where apiGroups won't unmarshal from yaml.
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			//Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{
					"events",
					"namespaces",
					"namespaces/status",
					"nodes",
					"nodes/spec",
					"pods",
					"pods/status",
					"replicationcontrollers",
					"replicationcontrollers/status",
					"resourcequotas",
					"services",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{
					"daemonsets",
					"deployments",
					"replicasets",
					"statefulsets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"extensions"},
				Resources: []string{
					"daemonsets",
					"deployments",
					"replicasets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{
					"jobs",
					"cronjobs",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"autoscaling"},
				Resources: []string{
					"horizontalpodautoscalers",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
		}

		return nil
	})

	if err != nil {
		return "", err
	}
	c.logger.Info("Role created or updated successfully", "role", role.Name, "operationResult", operationResult)

	return name, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorRoleBinding(ctx context.Context, namespace string, roleName string, serviceAccountName string) error {
	name := "mdai-collector-rb"

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              mdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, roleBinding, func() error {
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		}
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		}
		return nil
	})

	if err != nil {
		return err
	}
	c.logger.Info("RoleBinding created or updated successfully", "roleBinding", roleBinding.Name, "operationResult", operationResult)

	return nil
}
