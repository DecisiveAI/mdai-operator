package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/valkey-io/valkey-go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"reflect"
	"strings"
	"time"

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

	mdaiVariablePrefix = "MDAI_"
)

type HubAdapter struct {
	mdaiCR       *mdaiv1.MdaiHub
	logger       logr.Logger
	client       client.Client
	recorder     record.EventRecorder
	scheme       *runtime.Scheme
	context      context.Context
	valKeyClient *valkey.Client
}

type ObjectState bool

func NewHubAdapter(
	ctx context.Context,
	cr *mdaiv1.MdaiHub,
	log logr.Logger,
	client client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
	valkeyClient *valkey.Client) *HubAdapter {
	return &HubAdapter{
		context:      ctx,
		mdaiCR:       cr,
		logger:       log,
		client:       client,
		recorder:     recorder,
		scheme:       scheme,
		valKeyClient: valkeyClient,
	}
}

func (c HubAdapter) ensureFinalizerInitialized() (OperationResult, error) {
	if !controllerutil.ContainsFinalizer(c.mdaiCR, hubFinalizer) {
		c.logger.Info("Adding Finalizer for Engine")
		if ok := controllerutil.AddFinalizer(c.mdaiCR, hubFinalizer); !ok {
			c.logger.Error(nil, "Failed to add finalizer into the custom resource")
			return RequeueWithError(errors.New("failed to add finalizer " + hubFinalizer))
		}

		if err := c.client.Update(c.context, c.mdaiCR); err != nil {
			c.logger.Error(err, "Failed to update custom resource to add finalizer")
			return RequeueWithError(err)
		}
		return StopProcessing() // when finalizer is added it will trigger reconciliation
	}
	return ContinueProcessing()
}

func (c HubAdapter) ensureStatusInitialized() (OperationResult, error) {
	if len(c.mdaiCR.Status.Conditions) == 0 {
		meta.SetStatusCondition(&c.mdaiCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err := c.client.Status().Update(c.context, c.mdaiCR); err != nil {
			c.logger.Error(err, "Failed to update Cluster status")
			return RequeueWithError(err)
		}
		c.logger.Info("Re-queued to reconcile with updated status")
		return StopProcessing()
	}
	return ContinueProcessing()
}

// FinalizeHub handles the deletion of a hub
func (c HubAdapter) FinalizeHub() (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.mdaiCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for Cluster before delete CR")

	// meta.SetStatusCondition(&c.mdaiCR.Status.Conditions, metav1.Condition{
	//	Type:    typeDegradedEngine,
	//	Status:  metav1.ConditionUnknown,
	//	Reason:  "Finalizing",
	//	Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", c.mdaiCR.Name)})
	// if err := c.client.Status().Update(c.context, c.mdaiCR); err != nil {
	//	c.logger.Info("Failed to update Cluster status, will re-try later")
	//	return ObjectUnchanged, err
	// }

	c.logger.Info("Here we are doing some real finalization")

	err := c.deletePrometheusRule()
	if err != nil {
		c.logger.Info("Failed to delete prometheus rules, will re-try later")
		return ObjectUnchanged, err
	}

	if err := c.client.Get(c.context, types.NamespacedName{Name: c.mdaiCR.Name, Namespace: c.mdaiCR.Namespace}, c.mdaiCR); err != nil {
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
		if err := c.client.Status().Update(c.context, c.mdaiCR); err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Info("Cluster has been deleted, no need to finalize")
				return ObjectModified, nil
			}
			c.logger.Error(err, "Failed to update Cluster status")

			return ObjectUnchanged, err
		}
	}

	c.logger.Info("Removing Finalizer for Cluster after successfully perform the operations")
	if err := c.EnsureHubFinalizerDeleted(); err != nil {
		return ObjectUnchanged, err
	}
	return ObjectModified, nil
}

// EnsureHubFinalizerDeleted removes finalizer of a Hub
func (c HubAdapter) EnsureHubFinalizerDeleted() error {
	c.logger.Info("Deleting Cluster Finalizer")
	return c.deleteFinalizer(c.mdaiCR, hubFinalizer)
}

// deleteFinalizer deletes finalizer of a generic CR
func (c HubAdapter) deleteFinalizer(object client.Object, finalizer string) error {
	metadata, err := meta.Accessor(object)
	if err != nil {
		c.logger.Error(err, "Failed to delete finalizer", "finalizer", finalizer)
		return err
	}
	finalizers := metadata.GetFinalizers()
	if Contains(finalizers, finalizer) {
		metadata.SetFinalizers(Filter(finalizers, finalizer))
		return c.client.Update(c.context, object)
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
func (c HubAdapter) ensureEvaluationsSynchronized() (OperationResult, error) {
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

			prometheusRuleCR, err := c.getOrCreatePrometheusRuleCR(defaultPrometheusRuleName)
			if err != nil {
				c.logger.Error(err, "Failed to get/create PrometheusRule")
				return RequeueAfter(time.Second*10, err)
			}

			if eval.AlertingRules == nil {
				c.logger.Info("No alerting rules found in the evaluation, skipping PrometheusRule synchronization")
				continue
			}

			prometheusRuleCR.Spec.Groups[0].Rules = composePrometheusRule(*eval.AlertingRules, c.mdaiCR.Name)

			if err = c.client.Update(c.context, prometheusRuleCR); err != nil {
				c.logger.Error(err, "Failed to update PrometheusRule")
			}
		}
	}

	return ContinueProcessing()
}

func (c HubAdapter) getOrCreatePrometheusRuleCR(defaultPrometheusRuleName string) (*prometheusv1.PrometheusRule, error) {
	prometheusRule := &prometheusv1.PrometheusRule{}
	err := c.client.Get(
		c.context,
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
		if err := c.client.Create(c.context, prometheusRule); err != nil {
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

func (c HubAdapter) deletePrometheusRule() error {
	prometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	prometheusRule := &prometheusv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prometheusRuleName,
			Namespace: c.mdaiCR.Namespace,
		},
	}

	err := c.client.Delete(c.context, prometheusRule)
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

func (c HubAdapter) ensureVariableSynced() (OperationResult, error) {
	// current assumption is we have only built-in Valkey storage
	variables := c.mdaiCR.Spec.Variables
	if variables == nil {
		c.logger.Info("No variables found in the CR, skipping variable synchronization")
		return ContinueProcessing()
	}

	if c.valKeyClient == nil {
		c.logger.Error(nil, "Valkey valkeyClient not initialized, cannot sync variables")
		return ContinueProcessing()
	}

	envVariables := make([]v1.EnvVar, 0)
	valkeyClient := *c.valKeyClient
	for _, variable := range *variables {
		// we should test filter processor when the variable is empty and if breaks it we may recommend to use some placeholder as default value
		if variable.StorageType == mdaiv1.VariableSourceTypeBultInValkey {
			valkeyKey := variable.Name
			if variable.Type == mdaiv1.VariableTypeArray {
				valueAsSlice, err := valkeyClient.Do(
					c.context,
					valkeyClient.B().Smembers().Key(valkeyKey).Build(),
				).AsStrSlice()

				if err != nil {
					c.logger.Error(err, "Failed to get value from Valkey", "key", valkeyKey)
					return RequeueAfter(time.Second*10, err)
				}

				c.logger.Info("Valkey data received", "key", valkeyKey, "valueAsSlice", valueAsSlice)

				if variable.DefaultValue != nil && len(valueAsSlice) == 0 {
					c.logger.Info("Applying default value to variable", "key", valkeyKey, "defaultValue", *variable.DefaultValue)
					valueAsSlice = append(valueAsSlice, *variable.DefaultValue)
				} else {
					c.logger.Info("No value found in Valkey, skipping", "key", valkeyKey)
					continue
				}
				variableWithDelimiter := strings.Join(valueAsSlice, variable.Delimiter)
				envVariables = append(
					envVariables,
					v1.EnvVar{
						Name:  transformKeyToVariableName(valkeyKey),
						Value: variableWithDelimiter,
					},
				)
			} else if variable.Type == mdaiv1.VariableTypeScalar {
				value, err := valkeyClient.Do(
					c.context,
					valkeyClient.B().Get().Key(valkeyKey).Build(),
				).ToString()

				if err != nil && err.Error() == "valkey nil message" {
					if variable.DefaultValue != nil {
						c.logger.Info("Applying default value to variable", "key", valkeyKey, "defaultValue", *variable.DefaultValue)
						value = *variable.DefaultValue
					} else {
						c.logger.Info("No value found in Valkey, skipping", "key", valkeyKey)
						continue
					}
				} else {
					c.logger.Error(err, "Failed to get value from Valkey", "key", valkeyKey)
					return RequeueAfter(time.Second*10, err)
				}

				c.logger.Info("Valkey data received", "key", valkeyKey, "value", value)
				envVariables = append(envVariables, v1.EnvVar{
					Name:  transformKeyToVariableName(valkeyKey),
					Value: value,
				})
			}
		}
	}

	if len(envVariables) == 0 {
		c.logger.Info("No variables need to be updated")
		return ContinueProcessing()
	}

	collectors, err := c.listOtelCollectorsWithLabel(c.context, fmt.Sprintf("%s=%s", LabelMdaiHubName, c.mdaiCR.Name))
	if err != nil {
		return OperationResult{}, err
	}
	for _, collector := range collectors {
		c.logger.Info("Updating environment variables in OpenTelemetry Collector", "name", collector.Name)
		// insert env variable
		collectorCopy := collector.DeepCopy()
		//TODO clean up old env variables with mdai prefix
		collectorCopy.Spec.Env = upsertEnvVars(collectorCopy.Spec.Env, envVariables)

		if collector.Spec.Env != nil && reflect.DeepEqual(collector.Spec.Env, collectorCopy.Spec.Env) {
			c.logger.Info("Environment variables are already up-to-date")
			continue
		}

		// trigger update
		if err := c.client.Update(c.context, collectorCopy); err != nil {
			c.logger.Error(err, "Failed to update OpenTelemetry Collector", "name", collectorCopy.Name)
			return OperationResult{}, err
		}
	}

	return ContinueProcessing()
}

func transformKeyToVariableName(valkeyKey string) string {
	return mdaiVariablePrefix + strings.ToUpper(valkeyKey)
}

func upsertEnvVars(existingEnv []v1.EnvVar, newEnv []v1.EnvVar) []v1.EnvVar {
	envMap := make(map[string]int)
	for i, env := range existingEnv {
		envMap[env.Name] = i
	}

	for _, env := range newEnv {
		if idx, exists := envMap[env.Name]; exists {
			existingEnv[idx].Value = env.Value
		} else {
			existingEnv = append(existingEnv, env)
		}
	}

	return existingEnv
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
func (c HubAdapter) ensureHubDeletionProcessed() (OperationResult, error) {
	if !c.mdaiCR.DeletionTimestamp.IsZero() {
		c.logger.Info("Deleting Cluster:" + c.mdaiCR.Name)
		crState, err := c.FinalizeHub()
		if crState == ObjectUnchanged || err != nil {
			c.logger.Info("Has to requeue mdai")
			return RequeueAfter(5*time.Second, err)
		}
		return StopProcessing()
	}
	return ContinueProcessing()
}
