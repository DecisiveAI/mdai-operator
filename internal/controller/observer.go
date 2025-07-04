package controller

import (
	"context"
	_ "embed"
	"fmt"

	v1 "github.com/decisiveai/mdai-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

const (
	observerDefaultImage         = "public.ecr.aws/decisiveai/observer-collector:0.1"
	mdaiObserverHubComponent     = "mdai-observer"
	mdaiObserverResourceBaseName = "mdai-observer"
)

//go:embed config/observer_base_collector_config.yaml
var baseObserverCollectorYAML string

func (c ObserverAdapter) ensureObserversSynchronized(ctx context.Context) (OperationResult, error) {
	observers := c.observerCR.Spec.Observers
	observerResource := c.observerCR.Spec.ObserverResource

	if len(observers) == 0 {
		c.logger.Info("No observers found in the CR, skipping observer synchronization")
		return ContinueProcessing()
	}

	hash, err := c.createOrUpdateObserverResourceConfigMap(ctx, observerResource, observers)
	if err != nil {
		return OperationResult{}, err
	}

	if err := c.createOrUpdateObserverResourceDeployment(ctx, c.observerCR.Namespace, hash, observerResource); err != nil {
		if errors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		return OperationResult{}, err
	}

	if err := c.createOrUpdateObserverResourceService(ctx, c.observerCR.Namespace); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c ObserverAdapter) getScopedObserverResourceName(postfix string) string {
	if postfix != "" {
		return fmt.Sprintf("%s-%s-%s", c.observerCR.Name, mdaiObserverResourceBaseName, postfix)
	}
	return fmt.Sprintf("%s-%s", c.observerCR.Name, mdaiObserverResourceBaseName)
}

func (c ObserverAdapter) createOrUpdateObserverResourceService(ctx context.Context, namespace string) error {
	name := c.getScopedObserverResourceName("service")
	appLabel := c.getScopedObserverResourceName("")

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := controllerutil.SetControllerReference(c.observerCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = map[string]string{
				"app":             appLabel,
				hubNameLabel:      c.observerCR.Name,
				HubComponentLabel: mdaiObserverHubComponent,
			}
		}

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
		return fmt.Errorf("failed to create or update observer-collector-service: %w", err)
	}

	c.logger.Info("Successfully created or updated observer-collector-service", "service", name, "namespace", namespace, "operation", operationResult)
	return nil
}

func (c ObserverAdapter) createOrUpdateObserverResourceConfigMap(ctx context.Context, observerResource v1.ObserverResource, observers []v1.Observer) (string, error) {
	namespace := c.observerCR.Namespace
	configMapName := c.getScopedObserverResourceName("config")

	collectorYAML, err := c.getObserverCollectorConfig(observers, observerResource)
	if err != nil {
		return "", fmt.Errorf("failed to build observer configuration: %w", err)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":             c.getScopedObserverResourceName(""),
				HubComponentLabel: mdaiObserverHubComponent,
				hubNameLabel:      c.observerCR.Name,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}
	if err := controllerutil.SetControllerReference(c.observerCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", configMapName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", configMapName)
		return "", err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", configMapName, "operation", operationResult)
	return getConfigMapSHA(*desiredConfigMap)
}

func (c ObserverAdapter) createOrUpdateObserverResourceDeployment(ctx context.Context, namespace string, hash string, observerResource v1.ObserverResource) error {
	name := c.getScopedObserverResourceName("")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if err := controllerutil.SetControllerReference(c.observerCR, deployment, c.scheme); err != nil {
			c.logger.Error(err, "Failed to set owner reference on Deployment", "deployment", deployment.Name)
			return err
		}

		if deployment.Labels == nil {
			deployment.Labels = map[string]string{
				"app":             name,
				HubComponentLabel: mdaiObserverHubComponent,
				hubNameLabel:      c.observerCR.Name,
			}
		}

		deployment.Spec.Replicas = int32Ptr(1)
		if observerResource.Replicas != nil {
			deployment.Spec.Replicas = observerResource.Replicas
		}
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
		deployment.Spec.Template.Annotations["prometheus.io/path"] = "/metrics"
		deployment.Spec.Template.Annotations["prometheus.io/port"] = "8899"
		deployment.Spec.Template.Annotations["prometheus.io/scrape"] = "true"
		// FIXME: replace this annotation with mdai_observer_resource in other hub components (prometheus scraping config)
		deployment.Spec.Template.Annotations["mdai_component_type"] = "mdai-observer"
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash

		containerSpec := corev1.Container{
			Name:  name,
			Image: observerDefaultImage,
			Ports: []corev1.ContainerPort{
				{ContainerPort: 8888, Name: "otelcol-metrics"},
				{ContainerPort: 8899, Name: "observe-metrics"},
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
			Command: []string{
				// FIXME: update name away from observer
				"/mdai-observer-collector",
				"--config=/conf/collector.yaml",
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

		if observerResource.Image != nil && *observerResource.Image != "" {
			containerSpec.Image = *observerResource.Image
		}

		if observerResource.Resources != nil {
			containerSpec.Resources = *observerResource.Resources
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
							Name: c.getScopedObserverResourceName("config"),
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

func (c ObserverAdapter) getObserverCollectorConfig(observers []v1.Observer, observerResource v1.ObserverResource) (string, error) {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(baseObserverCollectorYAML), &config); err != nil {
		c.logger.Error(err, "Failed to unmarshal base collector config")
		return "", fmt.Errorf(`unmarshal base collector config: %w`, err)
	}
	grpcReceiverMaxMsgSize := observerResource.GrpcReceiverMaxMsgSize
	if grpcReceiverMaxMsgSize != nil {
		config["receivers"].(map[string]any)["otlp"].(map[string]any)["protocols"].(map[string]any)["grpc"].(map[string]any)["max_recv_msg_size_mib"] = *grpcReceiverMaxMsgSize
	}

	dataVolumeReceivers := make([]string, 0)
	for _, obs := range observers {
		observerName := obs.Name

		groupByKey := "groupbyattrs/" + observerName
		config["processors"].(map[string]any)[groupByKey] = map[string]any{
			"keys": obs.LabelResourceAttributes,
		}

		dvKey := "datavolume/" + observerName
		dvSpec := map[string]any{
			"label_resource_attributes": obs.LabelResourceAttributes,
		}
		if obs.CountMetricName != nil {
			dvSpec["count_metric_name"] = *obs.CountMetricName
		}
		if obs.BytesMetricName != nil {
			dvSpec["bytes_metric_name"] = *obs.BytesMetricName
		}
		config["connectors"].(map[string]any)[dvKey] = dvSpec

		filterName := ""
		if obs.Filter != nil {
			filterName = "filter/" + observerName
			config["processors"].(map[string]any)[filterName] = getObserverFilterProcessorConfig(obs.Filter)
		}

		var pipelineProcessors []string
		if filterName != "" {
			pipelineProcessors = append(pipelineProcessors, filterName)
		}
		pipelineProcessors = append(pipelineProcessors, "batch", groupByKey)

		logsPipelineName := "logs/" + observerName
		config["service"].(map[string]any)["pipelines"].(map[string]any)[logsPipelineName] = map[string]any{
			"receivers":  []string{"otlp"},
			"processors": pipelineProcessors,
			"exporters":  []string{dvKey},
		}

		tracesPipelineName := "traces/" + observerName
		config["service"].(map[string]any)["pipelines"].(map[string]any)[tracesPipelineName] = map[string]any{
			"receivers":  []string{"otlp"},
			"processors": pipelineProcessors,
			"exporters":  []string{dvKey},
		}

		dataVolumeReceivers = append(dataVolumeReceivers, dvKey)
	}

	serviceBlock := config["service"].(map[string]any)
	serviceBlock["pipelines"].(map[string]any)["metrics/observeroutput"] = map[string]any{
		"receivers":  dataVolumeReceivers,
		"processors": []string{"deltatocumulative"},
		"exporters":  []string{"prometheus"},
	}

	ownLogsOtlpEndpoint := observerResource.OwnLogsOtlpEndpoint
	if (ownLogsOtlpEndpoint != nil) && len(*ownLogsOtlpEndpoint) > 0 {
		serviceBlock["telemetry"].(map[string]any)["logs"] = map[string]any{
			"processors": []map[string]any{
				{
					"batch": map[string]any{
						"exporter": map[string]any{
							"otlp": map[string]any{
								"protocol": "http/protobuf",
								"endpoint": *ownLogsOtlpEndpoint,
							},
						},
					},
				},
			},
		}
	}

	raw, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal observer config: %w", err)
	}

	return string(raw), nil
}

func getObserverFilterProcessorConfig(filter *v1.ObserverFilter) map[string]any {
	filterMap := map[string]any{}

	if filter.ErrorMode != nil {
		filterMap["error_mode"] = filter.ErrorMode
	}

	if filter.Logs != nil && len(filter.Logs.LogRecord) > 0 {
		filterMap["logs"] = map[string]any{
			"log_record": filter.Logs.LogRecord,
		}
	}

	// TODO: Add metrics and trace filters

	return filterMap
}
