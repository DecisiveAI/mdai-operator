package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/valkey-io/valkey-go"
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
		prometheusRuleCR, err := c.getOrCreatePrometheusRuleCR(ctx, defaultPrometheusRuleName)
		if err != nil {
			c.logger.Error(err, "Failed to get/create PrometheusRule")
			return RequeueAfter(time.Second*10, err)
		}

		prometheusRuleCR.Spec.Groups[0].Rules = composePrometheusRule(eval, c.mdaiCR.Name)

		if err = c.client.Update(ctx, prometheusRuleCR); err != nil {
			c.logger.Error(err, "Failed to update PrometheusRule")
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

func composePrometheusRule(alertingRule mdaiv1.Evaluation, engineName string) []prometheusv1.Rule {
	alertName := (string)(alertingRule.Name)
	prometheusRule := prometheusv1.Rule{
		Expr:  alertingRule.Expr,
		Alert: alertName,
		For:   alertingRule.For,
		Annotations: map[string]string{
			"alert_name":  alertName, // FIXME we need a relationship between alert and variable
			"engine_name": engineName,
		},
		Labels: map[string]string{
			"severity": alertingRule.Severity,
		},
	}
	return []prometheusv1.Rule{prometheusRule}
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
			valkeyKey := (string)(variable.Name)
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

				delimiter := "|"

				for _, strategy := range *c.mdaiCR.Spec.Platform.Use {
					if *strategy.VariableName == variable.Name {
						for key, value := range strategy.Arguments {
							if key == "delimiter" {
								delimiter = value
							}
						}
					}
				}

				variableWithDelimiter := strings.Join(valueAsSlice, delimiter)
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

	// Set owner reference if necessary
	// For example, if the ConfigMap should be owned by a specific Custom Resource
	// controllerutil.SetControllerReference(owner, desiredConfigMap, scheme)

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
