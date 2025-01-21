package controller

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/valkey-io/valkey-go"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/go-logr/logr"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	mdaiv1 "mdai.ai/operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// typeAvailableHub represents the status of the Deployment reconciliation
	typeAvailableHub = "Available"
	// typeDegradedHub represents the status used when the custom resource is deleted and the finalizer operations are must occur.
	typeDegradedHub = "Degraded"

	hubFinalizer = "mdai.ai/finalizer"

	ObjectModified  ObjectState = true
	ObjectUnchanged ObjectState = false

	envConfigMapNamePostfix = "-variables"
	watcherConfigMapPostfix = "-watcher-collector-config"
)

type HubAdapter struct {
	mdaiCR       *mdaiv1.MdaiHub
	logger       logr.Logger
	client       client.Client
	recorder     record.EventRecorder
	scheme       *runtime.Scheme
	valKeyClient *valkey.Client
}

type ObjectState bool

func NewHubAdapter(
	cr *mdaiv1.MdaiHub,
	log logr.Logger,
	client client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
	valkeyClient *valkey.Client) *HubAdapter {
	return &HubAdapter{
		mdaiCR:       cr,
		logger:       log,
		client:       client,
		recorder:     recorder,
		scheme:       scheme,
		valKeyClient: valkeyClient,
	}
}

func (c HubAdapter) ensureFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if !controllerutil.ContainsFinalizer(c.mdaiCR, hubFinalizer) {
		c.logger.Info("Adding Finalizer for Engine")
		if ok := controllerutil.AddFinalizer(c.mdaiCR, hubFinalizer); !ok {
			c.logger.Error(nil, "Failed to add finalizer into the custom resource")
			return RequeueWithError(errors.New("failed to add finalizer " + hubFinalizer))
		}

		if err := c.client.Update(ctx, c.mdaiCR); err != nil {
			c.logger.Error(err, "Failed to update custom resource to add finalizer")
			return RequeueWithError(err)
		}
		return StopProcessing() // when finalizer is added it will trigger reconciliation
	}
	return ContinueProcessing()
}

func (c HubAdapter) ensureStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.mdaiCR.Status.Conditions) == 0 {
		meta.SetStatusCondition(&c.mdaiCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err := c.client.Status().Update(ctx, c.mdaiCR); err != nil {
			c.logger.Error(err, "Failed to update Cluster status")
			return RequeueWithError(err)
		}
		c.logger.Info("Re-queued to reconcile with updated status")
		return StopProcessing()
	}
	return ContinueProcessing()
}

// FinalizeHub handles the deletion of a hub
func (c HubAdapter) FinalizeHub(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.mdaiCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for Cluster before delete CR")

	c.logger.Info("Here we are doing some real finalization")

	err := c.deletePrometheusRule(ctx)
	if err != nil {
		c.logger.Info("Failed to delete prometheus rules, will re-try later")
		return ObjectUnchanged, err
	}

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.mdaiCR.Name, Namespace: c.mdaiCR.Namespace}, c.mdaiCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("Cluster has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "Failed to re-fetch Engine")
		return ObjectUnchanged, err
	}

	if meta.SetStatusCondition(&c.mdaiCR.Status.Conditions, metav1.Condition{
		Type:    typeDegradedHub,
		Status:  metav1.ConditionTrue,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.mdaiCR.Name),
	}) {
		if err := c.client.Status().Update(ctx, c.mdaiCR); err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Info("Cluster has been deleted, no need to finalize")
				return ObjectModified, nil
			}
			c.logger.Error(err, "Failed to update Cluster status")

			return ObjectUnchanged, err
		}
	}

	c.logger.Info("Removing Finalizer for Cluster after successfully perform the operations")
	if err := c.EnsureHubFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}
	return ObjectModified, nil
}

// EnsureHubFinalizerDeleted removes finalizer of a Hub
func (c HubAdapter) EnsureHubFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting Cluster Finalizer")
	return c.deleteFinalizer(ctx, c.mdaiCR, hubFinalizer)
}

// deleteFinalizer deletes finalizer of a generic CR
func (c HubAdapter) deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error {
	metadata, err := meta.Accessor(object)
	if err != nil {
		c.logger.Error(err, "Failed to delete finalizer", "finalizer", finalizer)
		return err
	}
	finalizers := metadata.GetFinalizers()
	if Contains(finalizers, finalizer) {
		metadata.SetFinalizers(Filter(finalizers, finalizer))
		return c.client.Update(ctx, object)
	}
	return nil
}

// Contains returns true if a list contains a string.
func Contains(list []string, strToSearch string) bool {
	for _, item := range list {
		if item == strToSearch {
			return true
		}
	}
	return false
}

// Filter filters a list for a string.
func Filter(list []string, strToFilter string) (newList []string) {
	for _, item := range list {
		if item != strToFilter {
			newList = append(newList, item)
		}
	}
	return
}

// EnsurePrometheusRuleSynchronized creates or updates PrometheusFilter CR
func (c HubAdapter) ensureEvaluationsSynchronized(ctx context.Context) (OperationResult, error) {
	evals := c.mdaiCR.Spec.Evaluations
	if evals == nil {
		c.logger.Info("No evaluation found in the CR, skipping PrometheusRule synchronization")
		return ContinueProcessing()
	}

	defaultPrometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	c.logger.Info("EnsurePrometheusRuleSynchronized")

	for _, eval := range *evals {
		if eval.EvaluationType == mdaiv1.EvaluationTypePrometheus {
			c.logger.Info("Evaluation type is Prometheus")

			prometheusRuleCR, err := c.getOrCreatePrometheusRuleCR(ctx, defaultPrometheusRuleName)
			if err != nil {
				c.logger.Error(err, "Failed to get/create PrometheusRule")
				return RequeueAfter(time.Second*10, err)
			}

			if eval.AlertingRules == nil {
				c.logger.Info("No alerting rules found in the evaluation, skipping PrometheusRule synchronization")
				continue
			}

			prometheusRuleCR.Spec.Groups[0].Rules = composePrometheusRule(*eval.AlertingRules, c.mdaiCR.Name)

			if err = c.client.Update(ctx, prometheusRuleCR); err != nil {
				c.logger.Error(err, "Failed to update PrometheusRule")
			}
		}
	}

	return ContinueProcessing()
}

func (c HubAdapter) getOrCreatePrometheusRuleCR(ctx context.Context, defaultPrometheusRuleName string) (*prometheusv1.PrometheusRule, error) {
	prometheusRule := &prometheusv1.PrometheusRule{}
	err := c.client.Get(
		ctx,
		client.ObjectKey{Namespace: c.mdaiCR.Namespace, Name: defaultPrometheusRuleName},
		prometheusRule,
	)

	if apierrors.IsNotFound(err) {
		prometheusRule := &prometheusv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: c.mdaiCR.Namespace,
				Name:      defaultPrometheusRuleName,
			},
			Spec: prometheusv1.PrometheusRuleSpec{
				Groups: []prometheusv1.RuleGroup{
					{
						Name:  "mdai",
						Rules: []prometheusv1.Rule{},
					},
				},
			},
		}
		if err := c.client.Create(ctx, prometheusRule); err != nil {
			c.logger.Error(err, "Failed to create PrometheusRule"+defaultPrometheusRuleName, "prometheus_rule_name", defaultPrometheusRuleName)
			return nil, err
		}
		c.logger.Info("Created new PrometheusRule:"+defaultPrometheusRuleName, "prometheus_rule_name", defaultPrometheusRuleName)
		return prometheusRule, nil
	} else if err != nil {
		c.logger.Error(err, "Failed to get PrometheusRule:"+defaultPrometheusRuleName, "prometheus_rule_name", defaultPrometheusRuleName)
		return nil, err
	}

	return prometheusRule, nil
}

func composePrometheusRule(alertingRules []mdaiv1.AlertingRule, engineName string) []prometheusv1.Rule {
	prometheusRules := make([]prometheusv1.Rule, 0, len(alertingRules))
	for _, alertingRule := range alertingRules {
		prometheusRule := prometheusv1.Rule{
			Expr:  alertingRule.AlertQuery,
			Alert: alertingRule.Name,
			For:   alertingRule.For,
			Annotations: map[string]string{
				"action":      alertingRule.Action,
				"alert_name":  alertingRule.Name, // FIXME we need a relationship between alert and variable
				"engine_name": engineName,
			},
			Labels: map[string]string{
				"severity": alertingRule.Severity,
			},
		}
		prometheusRules = append(prometheusRules, prometheusRule)
	}
	return prometheusRules
}

func (c HubAdapter) deletePrometheusRule(ctx context.Context) error {
	prometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	prometheusRule := &prometheusv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prometheusRuleName,
			Namespace: c.mdaiCR.Namespace,
		},
	}

	err := c.client.Delete(ctx, prometheusRule)
	if err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("PrometheusRule not found, nothing to delete", "prometheus_rule_name", prometheusRuleName)
			return nil
		}
		c.logger.Error(err, "Failed to delete prometheusRule", "prometheus_rule_name", prometheusRuleName)
		return err
	}

	c.logger.Info("Deleted PrometheusRule", "prometheus_rule_name", prometheusRuleName)
	return nil
}

func (c HubAdapter) ensureVariableSynced(ctx context.Context) (OperationResult, error) {
	// current assumption is we have only built-in Valkey storage
	variables := c.mdaiCR.Spec.Variables
	if variables == nil {
		c.logger.Info("No variables found in the CR, skipping variable synchronization")
		return ContinueProcessing()
	}

	if c.valKeyClient == nil {
		c.logger.Info("ValkeyClient not initialized, cannot sync variables")
		return ContinueProcessing()
	}

	envMap := make(map[string]string)
	valkeyClient := *c.valKeyClient
	for _, variable := range *variables {
		// we should test filter processor when the variable is empty and if breaks it we may recommend to use some placeholder as default value
		if variable.StorageType == mdaiv1.VariableSourceTypeBultInValkey {
			valkeyKey := variable.Name
			if variable.Type == mdaiv1.VariableTypeSet {
				valueAsSlice, err := valkeyClient.Do(
					ctx,
					valkeyClient.B().Smembers().Key(valkeyKey).Build(),
				).AsStrSlice()

				if err != nil {
					c.logger.Error(err, "Failed to get set value from Valkey", "key", valkeyKey)
					return RequeueAfter(time.Second*10, err)
				}

				c.logger.Info("Valkey data received", "key", valkeyKey, "valueAsSlice", valueAsSlice)

				if len(valueAsSlice) == 0 {
					if variable.DefaultValue != nil {
						c.logger.Info("Applying default value to variable", "key", valkeyKey, "defaultValue", *variable.DefaultValue)
						valueAsSlice = append(valueAsSlice, *variable.DefaultValue)
					} else {
						c.logger.Info("No value found in Valkey, skipping", "key", valkeyKey)
						continue
					}
				}
				variableWithDelimiter := strings.Join(valueAsSlice, variable.Delimiter)
				envMap[transformKeyToVariableName(valkeyKey)] = variableWithDelimiter
			} else if variable.Type == mdaiv1.VariableTypeScalar {
				valueAsString, err := valkeyClient.Do(
					ctx,
					valkeyClient.B().Get().Key(valkeyKey).Build(),
				).ToString()

				if err != nil {
					if err.Error() == "valkey nil message" {
						if variable.DefaultValue != nil {
							c.logger.Info("Applying default valueAsString to variable", "key", valkeyKey, "defaultValue", *variable.DefaultValue)
							valueAsString = *variable.DefaultValue
						} else {
							c.logger.Info("No valueAsString found in Valkey, skipping", "key", valkeyKey)
							continue
						}
					} else {
						c.logger.Error(err, "Failed to get valueAsString from Valkey", "key", valkeyKey)
						return RequeueAfter(time.Second*10, err)
					}
				}

				c.logger.Info("Valkey data received", "key", valkeyKey, "valueAsString", valueAsString)
				envMap[transformKeyToVariableName(valkeyKey)] = valueAsString
			}
		}
	}

	if len(envMap) == 0 {
		c.logger.Info("No variables need to be updated")
		return ContinueProcessing()
	}

	collectors, err := c.listOtelCollectorsWithLabel(ctx, fmt.Sprintf("%s=%s", LabelMdaiHubName, c.mdaiCR.Name))
	if err != nil {
		return OperationResult{}, err
	}

	// assuming collectors could be running in different namespaces, so we need to update envConfigMap in each namespace
	namespaces := make(map[string]bool)
	for _, collector := range collectors {
		namespaces[collector.Namespace] = true
	}

	namespaceToRestart := make(map[string]bool)
	for namespace := range namespaces {
		operationResult, err := c.createOrUpdateEnvConfigMap(ctx, envMap, namespace)
		if err != nil {
			return OperationResult{}, err
		}
		if operationResult == controllerutil.OperationResultUpdated || operationResult == controllerutil.OperationResultUpdatedStatus {
			namespaceToRestart[namespace] = true
		}
	}

	for _, collector := range collectors {
		if _, shouldRestart := namespaceToRestart[collector.Namespace]; shouldRestart {
			c.logger.Info("Triggering restart of OpenTelemetry Collector", "name", collector.Name)
			collectorCopy := collector.DeepCopy()
			// trigger restart
			if collector.Annotations == nil {
				collector.Annotations = make(map[string]string)
			}
			collectorCopy.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
			if err := c.client.Update(ctx, collectorCopy); err != nil {
				c.logger.Error(err, "Failed to update OpenTelemetry Collector", "name", collectorCopy.Name)
				return OperationResult{}, err
			}
		}
	}

	return ContinueProcessing()
}

func (c HubAdapter) createOrUpdateEnvConfigMap(ctx context.Context, envMap map[string]string, namespace string) (controllerutil.OperationResult, error) {
	envConfigMapName := c.mdaiCR.Name + envConfigMapNamePostfix
	desiredConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envConfigMapName,
			Namespace: namespace,
		},
		Data: envMap,
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", envConfigMapName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data = envMap
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "name", envConfigMapName, "namespace", namespace)
		return "", fmt.Errorf("failed to create or update ConfigMap: %w", err)
	}

	c.logger.Info("Successfully created or updated ConfigMap", "name", envConfigMapName, "namespace", namespace, "operation", operationResult)
	return operationResult, nil
}

func transformKeyToVariableName(valkeyKey string) string {
	return strings.ToUpper(valkeyKey)
}

func (c HubAdapter) listOtelCollectorsWithLabel(ctx context.Context, labelSelector string) ([]v1beta1.OpenTelemetryCollector, error) {
	var collectorList v1beta1.OpenTelemetryCollectorList

	selector, err := labels.Parse(labelSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selector: %w", err)
	}

	listOptions := &client.ListOptions{
		LabelSelector: selector,
	}

	if err := c.client.List(ctx, &collectorList, listOptions); err != nil {
		return nil, fmt.Errorf("failed to list OpenTelemetryCollectors: %w", err)
	}

	return collectorList.Items, nil
}

// ensureHubDeletionProcessed deletes Cluster in cases a deletion was triggered
func (c HubAdapter) ensureHubDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if !c.mdaiCR.DeletionTimestamp.IsZero() {
		c.logger.Info("Deleting Cluster:" + c.mdaiCR.Name)
		crState, err := c.FinalizeHub(ctx)
		if crState == ObjectUnchanged || err != nil {
			c.logger.Info("Has to requeue mdai")
			return RequeueAfter(5*time.Second, err)
		}
		return StopProcessing()
	}
	return ContinueProcessing()
}

func (c HubAdapter) ensureObserversSynchronized(ctx context.Context) (OperationResult, error) {
	observers := c.mdaiCR.Spec.Observers

	if observers == nil {
		c.logger.Info("No observers found in the CR, skipping observer synchronization")
		return ContinueProcessing()
	}

	// for now assuming one collector holds all observers
	if err := c.createOrUpdateWatcherCollectorConfigMap(ctx); err != nil {
		return OperationResult{}, err
	}

	// for now assuming one collector holds all observers
	if err := c.createOrUpdateWatcherCollectorService(ctx, c.mdaiCR.Namespace); err != nil {
		return OperationResult{}, err
	}
	if err := c.createOrUpdateWatcherCollectorDeployment(ctx, c.mdaiCR.Namespace); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c HubAdapter) createOrUpdateWatcherCollectorService(ctx context.Context, namespace string) error {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.mdaiCR.Name + "-watcher-collector-service",
			Namespace: namespace,
			Labels: map[string]string{
				"app": c.mdaiCR.Name + "-watcher-collector",
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", service.Name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		service.Spec = v1.ServiceSpec{
			Selector: map[string]string{
				"app": c.mdaiCR.Name + "-watcher-collector",
			},
			Ports: []v1.ServicePort{
				{
					Name:       "otlp-grpc",
					Protocol:   v1.ProtocolTCP,
					Port:       4317,
					TargetPort: intstr.FromString("otlp-grpc"),
				},
			},
			Type: v1.ServiceTypeClusterIP,
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create or update watcher-collector-service: %w", err)
	}

	c.logger.Info("Successfully created or updated watcher-collector-service", "namespace", namespace, "operation", operationResult)
	return nil
}

func (c HubAdapter) createOrUpdateWatcherCollectorConfigMap(ctx context.Context) error {
	namespace := c.mdaiCR.Namespace
	configMapName := c.mdaiCR.Name + watcherConfigMapPostfix

	collectorYAML, err := c.buildCollectorConfig()
	if err != nil {
		return fmt.Errorf("failed to build observer configuration: %w", err)
	}

	desiredConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app": c.mdaiCR.Name + "-watcher-collector",
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on ConfigMap", "configmap", configMapName)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "configmap", configMapName)
		return err
	}

	c.logger.Info("ConfigMap created or updated successfully", "configmap", configMapName, "operation", operationResult)
	return nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

func (c HubAdapter) createOrUpdateWatcherCollectorDeployment(ctx context.Context, namespace string) error {
	name := c.mdaiCR.Name + "-watcher-collector"
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                         name,
						"app.kubernetes.io/component": name,
					},
					Annotations: map[string]string{
						"prometheus.io/path":   "/metrics",
						"prometheus.io/port":   "8899",
						"prometheus.io/scrape": "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  name,
							Image: "public.ecr.aws/decisiveai/watcher-collector:0.1.0-dev",
							Ports: []v1.ContainerPort{
								{
									ContainerPort: 8888,
									Name:          "otelcol-metrics",
								},
								{
									ContainerPort: 8899,
									Name:          "watcher-metrics",
								},
								{
									ContainerPort: 4317,
									Name:          "otlp-grpc",
								},
								{
									ContainerPort: 4318,
									Name:          "otlp-http",
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/conf/collector.yaml",
									SubPath:   "collector.yaml",
								},
							},
							Command: []string{
								"/mdai-watcher-collector",
								"--config=/conf/collector.yaml",
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "config-volume",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: c.mdaiCR.Name + watcherConfigMapPostfix,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, deployment, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Deployment", "deployment", deployment.Name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update Deployment", "deployment", deployment.Name)
		return err
	}
	c.logger.Info("Deployment created or updated successfully", "deployment", deployment.Name, "operationResult", operationResult)

	return nil
}

func (c HubAdapter) buildCollectorConfig() (string, error) {
	observers := c.mdaiCR.Spec.Observers
	config := map[string]interface{}{
		"receivers": map[string]interface{}{
			"otlp": map[string]interface{}{
				"protocols": map[string]interface{}{
					"grpc": map[string]interface{}{
						"endpoint": "0.0.0.0:4317",
					},
				},
			},
		},
		"processors": map[string]interface{}{
			"batch":             map[string]interface{}{},
			"deltatocumulative": map[string]interface{}{},
		},
		"exporters": map[string]interface{}{
			"debug": map[string]interface{}{},
			"prometheus": map[string]interface{}{
				"endpoint":          "0.0.0.0:8899",
				"metric_expiration": "180m",
				"resource_to_telemetry_conversion": map[string]bool{
					"enabled": true,
				},
			},
		},
		"connectors": map[string]interface{}{},
		"service": map[string]interface{}{
			"telemetry": map[string]interface{}{
				"metrics": map[string]interface{}{
					"readers": []interface{}{
						map[string]interface{}{
							"pull": map[string]interface{}{
								"exporter": map[string]interface{}{
									"prometheus": map[string]interface{}{
										"host": "'0.0.0.0'",
										"port": 8888,
									},
								},
							},
						},
					},
				},
			},
			"pipelines": map[string]interface{}{},
		},
	}

	var dataVolumeReceivers = make([]string, 0)
	for _, obs := range *observers {
		if obs.Type != mdaiv1.ObserverTypeOtelWatcher {
			continue
		}

		watcherName := obs.Name

		groupByKey := "groupbyattrs/" + watcherName
		config["processors"].(map[string]interface{})[groupByKey] = map[string]interface{}{
			"keys": obs.OtelWatcherConfig.LabelResourceAttributes,
		}

		dvKey := fmt.Sprintf("datavolume/%s", watcherName)
		dvSpec := map[string]interface{}{
			"label_resource_attributes": obs.OtelWatcherConfig.LabelResourceAttributes,
		}
		if obs.OtelWatcherConfig.CountMetricName != nil {
			dvSpec["count_metric_name"] = *obs.OtelWatcherConfig.CountMetricName
		}
		if obs.OtelWatcherConfig.BytesMetricName != nil {
			dvSpec["bytes_metric_name"] = *obs.OtelWatcherConfig.BytesMetricName
		}
		config["connectors"].(map[string]interface{})[dvKey] = dvSpec

		filterName := ""
		if obs.OtelWatcherConfig.Filter != nil {
			filterName = fmt.Sprintf("filter/%s", watcherName)
			config["processors"].(map[string]interface{})[filterName] = buildFilterProcessorMap(obs.OtelWatcherConfig.Filter)
		}

		pipelineProcessors := []string{}
		if filterName != "" {
			pipelineProcessors = append(pipelineProcessors, filterName)
		}
		pipelineProcessors = append(pipelineProcessors, "batch", groupByKey)

		logsPipelineName := fmt.Sprintf("logs/%s", watcherName)
		config["service"].(map[string]interface{})["pipelines"].(map[string]interface{})[logsPipelineName] = map[string]interface{}{
			"receivers":  []string{"otlp"},
			"processors": pipelineProcessors,
			"exporters":  []string{dvKey},
		}

		dataVolumeReceivers = append(dataVolumeReceivers, dvKey)
	}

	config["service"].(map[string]interface{})["pipelines"].(map[string]interface{})["metrics/watcheroutput"] = map[string]interface{}{
		"receivers":  dataVolumeReceivers,
		"processors": []string{"deltatocumulative"},
		"exporters":  []string{"prometheus", "debug"},
	}

	raw, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func buildFilterProcessorMap(filter *mdaiv1.FilterProcessorConfig) map[string]interface{} {
	filterMap := map[string]interface{}{}

	if filter.ErrorMode != nil {
		filterMap["error_mode"] = filter.ErrorMode
	}

	if filter.Logs != nil && len(filter.Logs.LogConditions) > 0 {
		filterMap["logs"] = map[string]interface{}{
			"log_record": filter.Logs.LogConditions,
		}
	}

	if filter.Metrics != nil {
		metricsMap := map[string]interface{}{}
		if filter.Metrics.MetricConditions != nil && len(*filter.Metrics.MetricConditions) > 0 {
			metricsMap["metric"] = *filter.Metrics.MetricConditions
		}
		if filter.Metrics.DataPointConditions != nil && len(*filter.Metrics.DataPointConditions) > 0 {
			metricsMap["datapoint"] = *filter.Metrics.DataPointConditions
		}
		if len(metricsMap) > 0 {
			filterMap["metrics"] = metricsMap
		}
	}

	if filter.Traces != nil {
		tracesMap := map[string]interface{}{}
		if filter.Traces.SpanConditions != nil && len(*filter.Traces.SpanConditions) > 0 {
			tracesMap["span"] = *filter.Traces.SpanConditions
		}
		if filter.Traces.SpanEventConditions != nil && len(*filter.Traces.SpanEventConditions) > 0 {
			tracesMap["spanevent"] = *filter.Traces.SpanEventConditions
		}
		if len(tracesMap) > 0 {
			filterMap["traces"] = tracesMap
		}
	}

	return filterMap
}
