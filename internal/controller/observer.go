package controller

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/DecisiveAI/mdai-operator/api/v1"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const observerDefaultImage = "public.ecr.aws/decisiveai/watcher-collector:0.1.3"

const observerResourceLabel = "mdai_observer_resource"

//go:embed config/base_collector.yaml
var baseCollectorYAML string

func (c HubAdapter) getScopedObserverResourceName(observerResource v1.ObserverResource, postfix string) string {
	if postfix != "" {
		return fmt.Sprintf("%s-%s-%s", c.mdaiCR.Name, observerResource.Name, postfix)
	}
	return fmt.Sprintf("%s-%s", c.mdaiCR.Name, observerResource.Name)
}

func (c HubAdapter) createOrUpdateObserverResourceService(ctx context.Context, namespace string, observerResource v1.ObserverResource) error {
	name := c.getScopedObserverResourceName(observerResource, "service")
	appLabel := c.getScopedObserverResourceName(observerResource, "")

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		service.Labels["app"] = appLabel
		service.Labels[hubNameLabel] = c.mdaiCR.Name
		service.Labels[observerResourceLabel] = observerResource.Name

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
		return fmt.Errorf("failed to create or update watcher-collector-service: %w", err)
	}

	c.logger.Info("Successfully created or updated watcher-collector-service", "service", name, "namespace", namespace, "operation", operationResult)
	return nil
}

func (c HubAdapter) createOrUpdateObserverResourceConfigMap(ctx context.Context, observerResource v1.ObserverResource, observers []v1.Observer) (string, error) {
	namespace := c.mdaiCR.Namespace
	configMapName := c.getScopedObserverResourceName(observerResource, "config")

	collectorYAML, err := c.getObserverCollectorConfig(observers, observerResource.GrpcReceiverMaxMsgSize)
	if err != nil {
		return "", fmt.Errorf("failed to build observer configuration: %w", err)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                 c.getScopedObserverResourceName(observerResource, ""),
				hubNameLabel:          c.mdaiCR.Name,
				observerResourceLabel: observerResource.Name,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}
	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
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

func (c HubAdapter) createOrUpdateObserverResourceDeployment(ctx context.Context, namespace string, hash string, observerResource v1.ObserverResource) error {
	name := c.getScopedObserverResourceName(observerResource, "")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
		deployment.Labels[observerResourceLabel] = observerResource.Name

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
		deployment.Spec.Template.Annotations["mdai_component_type"] = "mdai-watcher"
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash

		containerSpec := corev1.Container{
			Name:  name,
			Image: observerDefaultImage,
			Ports: []corev1.ContainerPort{
				{ContainerPort: 8888, Name: "otelcol-metrics"},
				// FIXME: update name away from watcher
				{ContainerPort: 8899, Name: "watcher-metrics"},
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
				// FIXME: update name away from watcher
				"/mdai-watcher-collector",
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
							Name: c.getScopedObserverResourceName(observerResource, "config"),
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

func (c HubAdapter) ensureObserversSynchronized(ctx context.Context) (OperationResult, error) {
	observers := c.mdaiCR.Spec.Observers
	observerResources := c.mdaiCR.Spec.ObserverResources

	if observerResources == nil {
		// TODO: Check if resource exists and needs to be removed!
		c.logger.Info("No observerResources found in the CR, skipping observer synchronization")
		return ContinueProcessing()
	}
	if observers == nil {
		c.logger.Info("No observers found in the CR, skipping observer synchronization")
		return ContinueProcessing()
	}

	configObserverResources := make([]string, 0)
	for _, observerResource := range *observerResources {
		configObserverResources = append(configObserverResources, observerResource.Name)
		observersForResource := make([]v1.Observer, 0)
		for _, observer := range *observers {
			if observer.ResourceRef == observerResource.Name {
				observersForResource = append(observersForResource, observer)
			}
		}

		if len(observersForResource) == 0 {
			// TODO: Check if resource exists and needs to be removed!
			c.logger.Info("No observers configured using observerResource, skipping this observerResource", "observerResource", observerResource.Name)
			continue
		}

		hash, err := c.createOrUpdateObserverResourceConfigMap(ctx, observerResource, observersForResource)
		if err != nil {
			return OperationResult{}, err
		}

		if err := c.createOrUpdateObserverResourceDeployment(ctx, c.mdaiCR.Namespace, hash, observerResource); err != nil {
			if errors.ReasonForError(err) == metav1.StatusReasonConflict {
				c.logger.Info("re-queuing due to resource conflict")
				return Requeue()
			}
			return OperationResult{}, err
		}

		if err := c.createOrUpdateObserverResourceService(ctx, c.mdaiCR.Namespace, observerResource); err != nil {
			return OperationResult{}, err
		}
	}

	if err := c.cleanupOrphanedObserverResources(ctx, configObserverResources); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c HubAdapter) getObserverCollectorConfig(observers []v1.Observer, grpcReceiverMaxMsgSize *uint64) (string, error) {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(baseCollectorYAML), &config); err != nil {
		c.logger.Error(err, "Failed to unmarshal base collector config")
		return "", err
	}

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

		dataVolumeReceivers = append(dataVolumeReceivers, dvKey)
	}

	config["service"].(map[string]any)["pipelines"].(map[string]any)["metrics/observeroutput"] = map[string]any{
		"receivers":  dataVolumeReceivers,
		"processors": []string{"deltatocumulative"},
		"exporters":  []string{"prometheus"},
	}

	raw, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func (c HubAdapter) cleanupOrphanedObserverResources(ctx context.Context, resources []string) error {
	labelSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			hubNameLabel: c.mdaiCR.Name,
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      observerResourceLabel,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   resources,
			},
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		c.logger.Error(err, "could not build selector to delete orphaned hub observer resources")
		return err
	}

	serviceList := corev1.ServiceList{}
	if err := c.client.List(ctx, &serviceList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		c.logger.Error(err, "could not query for orphaned hub observer resource services")
		return err
	}

	deploymentList := appsv1.DeploymentList{}
	if err := c.client.List(ctx, &deploymentList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		c.logger.Error(err, "could not query for orphaned hub observer resource deployments")
		return err
	}

	configMapList := corev1.ConfigMapList{}
	if err := c.client.List(ctx, &configMapList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		c.logger.Error(err, "could not query for orphaned hub observer resource configmaps")
		return err
	}

	for _, service := range serviceList.Items {
		c.logger.Info("Deleting orphaned observer resource service", "service", service.Name)
		if err := c.client.Delete(ctx, &service); err != nil {
			c.logger.Error(err, "could not delete orphaned observer resource service %s", "service", service.Name)
			return err
		}
	}

	for _, deployment := range deploymentList.Items {
		c.logger.Info("Deleting orphaned observer resource deployment", "deployment", deployment.Name)
		if err := c.client.Delete(ctx, &deployment); err != nil {
			c.logger.Error(err, "could not delete orphaned observer resource deployment", "deployment", deployment.Name)
			return err
		}
	}

	for _, configMap := range configMapList.Items {
		c.logger.Info("Deleting orphaned observer resource configMap", "configMap", configMap.Name)
		if err := c.client.Delete(ctx, &configMap); err != nil {
			c.logger.Error(err, "could not delete orphaned observer resource configMap %s", "configMap", configMap.Name)
			return err
		}
	}

	return nil
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
