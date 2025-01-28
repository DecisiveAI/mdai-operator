package controller

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
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
	defaultPrometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	c.logger.Info("EnsurePrometheusRuleSynchronized")

	prometheusRuleCR, err := c.getOrCreatePrometheusRuleCR(ctx, defaultPrometheusRuleName)
	if err != nil {
		c.logger.Error(err, "Failed to get/create PrometheusRule")
		return RequeueAfter(time.Second*10, err)
	}

	evals := c.mdaiCR.Spec.Evaluations
	if evals == nil {
		if len(prometheusRuleCR.Spec.Groups[0].Rules) != 0 {
			c.logger.Info("Rules removed from CR but still exist in prometheus, removing existing rules")
			if err := c.deletePrometheusRule(ctx); err != nil {
				c.logger.Error(err, "Failed to remove existing rules")
			}
		} else {
			c.logger.Info("No evaluation found in the CR, skipping PrometheusRule synchronization")
		}
		return ContinueProcessing()
	}

	rules := make([]prometheusv1.Rule, 0, len(*evals))
	for _, eval := range *evals {
		rule := c.composePrometheusRule(eval)
		rules = append(rules, rule)
	}

	prometheusRuleCR.Spec.Groups[0].Rules = rules
	if err = c.client.Update(ctx, prometheusRuleCR); err != nil {
		c.logger.Error(err, "Failed to update PrometheusRule")
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

func (c HubAdapter) composePrometheusRule(alertingRule mdaiv1.Evaluation) prometheusv1.Rule {
	alertName := string(alertingRule.Name)

	prometheusRule := prometheusv1.Rule{
		Expr:  alertingRule.Expr,
		Alert: alertName,
		For:   alertingRule.For,
		Annotations: map[string]string{
			"alert_name":    alertName,
			"engine_name":   c.mdaiCR.Name,
			"current_value": "{{ $value | printf \"%.2f\" }}",
		},
		Labels: map[string]string{
			"severity": alertingRule.Severity,
		},
	}

	if alertingRule.OnStatus != nil {
		actionContextJson, err := json.Marshal(alertingRule.OnStatus)
		if err != nil {
			c.logger.Error(err, "Failed to compose action context for eval", "name", alertName, "status", *alertingRule.OnStatus)
		}
		prometheusRule.Annotations["action_context"] = string(actionContextJson)
	}

	if alertingRule.RelevantLabels != nil {
		relevantLabelsJson, err := json.Marshal(*alertingRule.RelevantLabels)
		if err != nil {
			c.logger.Error(err, "Failed to compose relevant labels for eval", "name", alertName, "relevantLabels", *alertingRule.RelevantLabels)
		}
		prometheusRule.Annotations["relevant_labels"] = string(relevantLabelsJson)
	}

	return prometheusRule
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
		if *variable.StorageType == mdaiv1.VariableSourceTypeBultInValkey {
			valkeyKey := variable.StorageKey
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

				for _, with := range *variable.With {
					exportedVariableName := with.ExportedVariableName
					transformer := with.Transformer

					join := transformer.Join
					if join != nil {
						delimiter := join.Delimiter
						variableWithDelimiter := strings.Join(valueAsSlice, delimiter)
						envMap[transformKeyToVariableName(exportedVariableName)] = variableWithDelimiter
					}
				}
			} else if variable.Type == mdaiv1.VariableTypeString {
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
	}
	// we are not setting an owner reference here as we want to allow config maps being deployed across namespaces
	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data = envMap
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update ConfigMap", "name", envConfigMapName, "namespace", namespace)
		return controllerutil.OperationResultNone, fmt.Errorf("failed to create or update ConfigMap: %w", err)
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
	hash, err := c.createOrUpdateWatcherCollectorConfigMap(ctx)
	if err != nil {
		return OperationResult{}, err
	}

	// for now assuming one collector holds all observers
	if err := c.createOrUpdateWatcherCollectorService(ctx, c.mdaiCR.Namespace); err != nil {
		return OperationResult{}, err
	}
	if err := c.createOrUpdateWatcherCollectorDeployment(ctx, c.mdaiCR.Namespace, hash); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c HubAdapter) createOrUpdateWatcherCollectorService(ctx context.Context, namespace string) error {
	name := c.mdaiCR.Name + "-watcher-collector-service"
	appLabel := c.mdaiCR.Name + "-watcher-collector"

	service := &v1.Service{
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

		service.Spec = v1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabel,
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

	c.logger.Info("Successfully created or updated watcher-collector-service", "service", name, "namespace", namespace, "operation", operationResult)
	return nil
}

func (c HubAdapter) createOrUpdateWatcherCollectorConfigMap(ctx context.Context) (string, error) {
	namespace := c.mdaiCR.Namespace
	configMapName := c.mdaiCR.Name + watcherConfigMapPostfix

	collectorYAML, err := c.buildCollectorConfig()
	if err != nil {
		return "", fmt.Errorf("failed to build observer configuration: %w", err)
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

func int32Ptr(i int32) *int32 {
	return &i
}

func getConfigMapSHA(config v1.ConfigMap) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func (c HubAdapter) createOrUpdateWatcherCollectorDeployment(ctx context.Context, namespace string, hash string) error {
	name := c.mdaiCR.Name + "-watcher-collector"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := controllerutil.SetControllerReference(c.mdaiCR, deployment, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Deployment", "deployment", deployment.Name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels["app"] = name
		deployment.Spec.Replicas = int32Ptr(1)
		if deployment.Spec.Selector == nil {
			deployment.Spec.Selector = &metav1.LabelSelector{}
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
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash
		deployment.Spec.Template.Spec = v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  name,
					Image: "public.ecr.aws/decisiveai/watcher-collector:0.1.0-dev",
					Ports: []v1.ContainerPort{
						{ContainerPort: 8888, Name: "otelcol-metrics"},
						{ContainerPort: 8899, Name: "watcher-metrics"},
						{ContainerPort: 4317, Name: "otlp-grpc"},
						{ContainerPort: 4318, Name: "otlp-http"},
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
		}

		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update Deployment", "deployment", deployment.Name)
		return err
	}
	c.logger.Info("Deployment created or updated successfully", "deployment", deployment.Name, "operationResult", operationResult)

	return nil
}

//go:embed config/base_collector.yaml
var baseCollectorYAML string

func (c HubAdapter) buildCollectorConfig() (string, error) {
	observers := c.mdaiCR.Spec.Observers

	var config map[string]any
	if err := yaml.Unmarshal([]byte(baseCollectorYAML), &config); err != nil {
		c.logger.Error(err, "Failed to unmarshal base collector config")
		return "", err
	}

	var dataVolumeReceivers = make([]string, 0)
	for _, obs := range *observers {
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
			config["processors"].(map[string]any)[filterName] = buildFilterProcessorMap(obs.Filter)
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
		"exporters":  []string{"prometheus", "debug"},
	}

	raw, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func buildFilterProcessorMap(filter *mdaiv1.ObserverFilter) map[string]any {
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
