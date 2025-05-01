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

type MDAILogStream string

type S3ExporterConfig struct {
	S3Uploader S3UploaderConfig `mapstructure:"s3uploader"`
}

type S3UploaderConfig struct {
	Region            string `yaml:"region"`
	S3Bucket          string `yaml:"s3_bucket"`
	S3Prefix          string `yaml:"s3_prefix"`
	S3PartitionFormat string `yaml:"s3_partition_format"`
	FilePrefix        string `yaml:"file_prefix"`
	DisableSSL        bool   `yaml:"disable_ssl"`
}

const (
	MdaiCollectorHubComponent = "mdai-collector"
	mdaiCollectorAppLabel     = "mdai-collector"

	mdaiCollectorDeploymentName         = "mdai-collector"
	mdaiCollectorConfigConfigMapName    = "mdai-collector-config"
	mdaiCollectorEnvVarConfigMapName    = "mdai-collector-env"
	mdaiCollectorServiceName            = "mdai-collector-service"
	mdaiCollectorServiceAccountName     = "mdai-collector-sa"
	mdaiCollectorClusterRoleName        = "mdai-collector-role"
	mdaiCollectorClusterRoleBindingName = "mdai-collector-rb"

	AuditLogstream     MDAILogStream = "audit"
	CollectorLogstream MDAILogStream = "collector"
	HubLogstream       MDAILogStream = "hub"
	OtherLogstream     MDAILogStream = "other"
)

var (
	//go:embed config/mdai_collector_base_config.yaml
	baseMdaiCollectorYAML string
	logstreams            = []MDAILogStream{AuditLogstream, CollectorLogstream, HubLogstream, OtherLogstream}
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

	awsAccessKeySecret := awsConfig.AWSAccessKeySecret

	collectorConfigMapName, hash, err := c.createOrUpdateMdaiCollectorConfigMap(ctx, namespace, logsConfig, awsAccessKeySecret)
	if err != nil {
		return OperationResult{}, err
	}
	collectorEnvConfigMapName, err := c.createOrUpdateMdaiCollectorEnvVarConfigMap(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	serviceAccountName, err := c.createOrUpdateMdaiCollectorServiceAccount(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	roleName, err := c.createOrUpdateMdaiCollectorRole(ctx)
	if err != nil {
		return OperationResult{}, err
	}
	err = c.createOrUpdateMdaiCollectorRoleBinding(ctx, namespace, roleName, serviceAccountName)
	if err != nil {
		return OperationResult{}, err
	}

	if err := c.createOrUpdateMdaiCollectorDeployment(ctx, namespace, collectorConfigMapName, collectorEnvConfigMapName, serviceAccountName, *awsAccessKeySecret, hash); err != nil {
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

func (c HubAdapter) getMdaiCollectorConfig(logsConfig *v1.LogsConfig, awsAccessKeySecret *string) (string, error) {
	var mdaiCollectorConfig map[string]any
	if err := yaml.Unmarshal([]byte(baseMdaiCollectorYAML), &mdaiCollectorConfig); err != nil {
		c.logger.Error(err, "Failed to unmarshal base mdai collector config")
		return "", err
	}

	s3Config := logsConfig.S3
	if s3Config != nil && awsAccessKeySecret != nil {
		exporters := mdaiCollectorConfig["exporters"].(map[string]any)
		serviceBlock := mdaiCollectorConfig["service"].(map[string]any)
		pipelines := serviceBlock["pipelines"].(map[string]any)
		for _, logstream := range logstreams {
			s3ExporterName, s3Exporter := c.getS3ExporterForLogstream(logstream, *s3Config)
			exporters[s3ExporterName] = s3Exporter
			pipelineName := fmt.Sprintf("logs/%s", logstream)
			pipeline := pipelines[pipelineName].(map[string]any)
			pipelines[pipelineName] = c.getPipelineWithS3Exporter(pipeline, s3ExporterName)
		}
	} else {
		c.logger.Info("Skipped adding s3 components due to missing s3 configuration", "s3LogsConfig", s3Config, "awsAccessKeySecret", awsAccessKeySecret)
	}

	collectorConfigBytes, err := yaml.Marshal(mdaiCollectorConfig)
	if err != nil {
		c.logger.Error(err, "Failed to marshal mdai-collector config", "mdaiCollectorConfig", mdaiCollectorConfig)
	}
	collectorConfig := string(collectorConfigBytes)

	return collectorConfig, nil
}

func (c HubAdapter) getS3ExporterForLogstream(logstream MDAILogStream, s3LogsConfig v1.S3LogsConfig) (string, S3ExporterConfig) {
	hubName := c.mdaiCR.Name
	s3Prefix := fmt.Sprintf("%s-%s", hubName, logstream)
	exporterKey := fmt.Sprintf("awss3/%s", logstream)
	filePrefix := fmt.Sprintf("%s-", logstream)
	exporter := S3ExporterConfig{
		S3Uploader: S3UploaderConfig{
			Region:            *s3LogsConfig.S3Region,
			S3Bucket:          *s3LogsConfig.S3Bucket,
			S3Prefix:          s3Prefix,
			FilePrefix:        filePrefix,
			S3PartitionFormat: "%Y/%m/%d/%H",
			DisableSSL:        true,
		},
	}
	return exporterKey, exporter
}

func (c HubAdapter) getPipelineWithS3Exporter(pipeline map[string]any, exporterName string) map[string]any {
	exporters := pipeline["exporters"].([]any)
	newPipeline := map[string]any{
		"receivers":  pipeline["receivers"],
		"processors": pipeline["processors"],
		"exporters":  append(exporters, exporterName),
	}
	return newPipeline
}

func (c HubAdapter) createOrUpdateMdaiCollectorConfigMap(ctx context.Context, namespace string, logsConfig *v1.LogsConfig, awsAccessKeySecret *string) (string, string, error) {
	collectorYAML, err := c.getMdaiCollectorConfig(logsConfig, awsAccessKeySecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to build mdai-collector configuration: %w", err)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorConfigConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.mdaiCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}
	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", mdaiCollectorConfigConfigMapName)
		return "", "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", mdaiCollectorConfigConfigMapName)
		return "", "", err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", mdaiCollectorConfigConfigMapName, "operation", operationResult)
	sha, err := getConfigMapSHA(*desiredConfigMap)
	return mdaiCollectorConfigConfigMapName, sha, err
}

func (c HubAdapter) createOrUpdateMdaiCollectorEnvVarConfigMap(ctx context.Context, namespace string) (string, error) {
	data := map[string]string{
		"LOG_SEVERITY":  "SEVERITY_NUMBER_WARN",
		"K8S_NAMESPACE": namespace,
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorEnvVarConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.mdaiCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
		Data: data,
	}
	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", mdaiCollectorEnvVarConfigMapName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data = data
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", mdaiCollectorEnvVarConfigMapName)
		return "", err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", mdaiCollectorEnvVarConfigMapName, "operation", operationResult)
	return mdaiCollectorEnvVarConfigMapName, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorDeployment(ctx context.Context, namespace string, collectorConfigMapName string, collectorEnvConfigMapName string, serviceAccountName string, awsAccessKeySecret string, hash string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
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
		deployment.Labels["app"] = mdaiCollectorDeploymentName
		deployment.Labels[hubNameLabel] = c.mdaiCR.Name

		deployment.Spec.Replicas = int32Ptr(1)
		if deployment.Spec.Selector == nil {
			deployment.Spec.Selector = &metav1.LabelSelector{}
		}
		if deployment.Spec.Selector.MatchLabels == nil {
			deployment.Spec.Selector.MatchLabels = make(map[string]string)
		}
		deployment.Spec.Selector.MatchLabels["app"] = mdaiCollectorDeploymentName

		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		deployment.Spec.Template.Labels["app"] = mdaiCollectorDeploymentName
		deployment.Spec.Template.Labels["app.kubernetes.io/component"] = mdaiCollectorDeploymentName

		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash

		deployment.Spec.Template.Spec.ServiceAccountName = serviceAccountName

		containerSpec := corev1.Container{
			Name:  mdaiCollectorDeploymentName,
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

		if awsAccessKeySecret != "" {
			secretEnvSource := corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: awsAccessKeySecret,
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
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorServiceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", mdaiCollectorServiceName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		service.Labels["app"] = mdaiCollectorAppLabel
		service.Labels[hubNameLabel] = c.mdaiCR.Name

		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": mdaiCollectorAppLabel,
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

	c.logger.Info("Successfully created or updated watcher-collector-service", "service", mdaiCollectorServiceName, "namespace", namespace, "operation", operationResult)
	return mdaiCollectorServiceName, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorServiceAccount(ctx context.Context, namespace string) (string, error) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorServiceAccountName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, serviceAccount, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ServiceAccount", "serviceAccount", mdaiCollectorServiceAccountName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, serviceAccount, func() error {
		return nil
	})

	if err != nil {
		return "", err
	}
	c.logger.Info("ServiceAccount created or updated successfully", "serviceAccount", serviceAccount.Name, "operationResult", operationResult)

	return mdaiCollectorServiceAccountName, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorRole(ctx context.Context) (string, error) {
	// BEWARE: If you're thinking about making a yaml and unmarshaling it, to extract all this out... there's a weird problem where apiGroups won't unmarshal from yaml.
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: mdaiCollectorClusterRoleName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
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

	return mdaiCollectorClusterRoleName, nil
}

func (c HubAdapter) createOrUpdateMdaiCollectorRoleBinding(ctx context.Context, namespace string, roleName string, serviceAccountName string) error {
	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: mdaiCollectorClusterRoleBindingName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
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
