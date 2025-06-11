package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/decisiveai/mdai-data-core/audit"

	datacore "github.com/decisiveai/mdai-data-core/variables"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/opentelemetry-operator/apis/v1beta1"
	"github.com/go-logr/logr"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/valkey-io/valkey-go"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	envConfigMapNamePostfix        = "-variables"
	manualEnvConfigMapNamePostfix  = "-manual-variables"
	automationConfigMapNamePostfix = "-automation"

	requeueTime = time.Second * 10

	hubNameLabel      = "mdai-hub-name"
	HubComponentLabel = "mdai-hub-component"
)

type HubAdapter struct {
	mdaiCR                  *mdaiv1.MdaiHub
	logger                  logr.Logger
	client                  client.Client
	recorder                record.EventRecorder
	scheme                  *runtime.Scheme
	valKeyClient            valkey.Client
	valkeyAuditStreamExpiry time.Duration
	releaseName             string
	zapLogger               *zap.Logger
}

func NewHubAdapter(
	cr *mdaiv1.MdaiHub,
	log logr.Logger,
	zapLogger *zap.Logger,
	client client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
	valkeyClient valkey.Client,
	valkeyAuditStreamExpiry time.Duration,
) *HubAdapter {
	return &HubAdapter{
		mdaiCR:                  cr,
		logger:                  log,
		client:                  client,
		recorder:                recorder,
		scheme:                  scheme,
		valKeyClient:            valkeyClient,
		valkeyAuditStreamExpiry: valkeyAuditStreamExpiry,
		releaseName:             os.Getenv("RELEASE_NAME"),
		zapLogger:               zapLogger,
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

// finalizeHub handles the deletion of a hub
func (c HubAdapter) finalizeHub(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.mdaiCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for Cluster before delete CR")

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
	if err := c.ensureHubFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	prefix := VariableKeyPrefix + c.mdaiCR.Name + "/"
	c.logger.Info("Cleaning up old variables from Valkey with prefix", "prefix", prefix)
	if err := datacore.NewValkeyAdapter(c.valKeyClient, c.zapLogger).DeleteKeysWithPrefixUsingScan(ctx, map[string]struct{}{}, c.mdaiCR.Name); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

// ensureHubFinalizerDeleted removes finalizer of a Hub
func (c HubAdapter) ensureHubFinalizerDeleted(ctx context.Context) error {
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
	if slices.Contains(finalizers, finalizer) {
		metadata.SetFinalizers(slices.DeleteFunc(finalizers, func(f string) bool { return f == finalizer }))
		return c.client.Update(ctx, object)
	}
	return nil
}

// ensurePrometheusRuleSynchronized creates or updates PrometheusFilter CR
func (c HubAdapter) ensurePrometheusAlertsSynchronized(ctx context.Context) (OperationResult, error) {
	defaultPrometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	c.logger.Info("EnsurePrometheusRuleSynchronized")

	evals := c.mdaiCR.Spec.PrometheusAlert

	prometheusRule := &prometheusv1.PrometheusRule{}
	err := c.client.Get(
		ctx,
		client.ObjectKey{Namespace: c.mdaiCR.Namespace, Name: defaultPrometheusRuleName},
		prometheusRule,
	)

	// rules exist, but no evaluations
	if evals == nil {
		c.logger.Info("No evaluations found, skipping PrometheusRule creation")
		if err == nil {
			c.logger.Info("Removing existing rules")
			if err := c.deletePrometheusRule(ctx); err != nil {
				c.logger.Error(err, "Failed to remove existing rules")
			}
		}
		return ContinueProcessing()
	}

	// create new prometheus rule
	if apierrors.IsNotFound(err) {
		prometheusRule = &prometheusv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultPrometheusRuleName,
				Namespace: c.mdaiCR.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "mdai-operator",
					"app.kubernetes.io/part-of":    "kube-prometheus-stack",
					"app.kubernetes.io/instance":   c.releaseName,
				},
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
			return RequeueAfter(requeueTime, err)
		}
		c.logger.Info("Created new PrometheusRule:"+defaultPrometheusRuleName, "prometheus_rule_name", defaultPrometheusRuleName)
	} else if err != nil {
		c.logger.Error(err, "Failed to get PrometheusRule:"+defaultPrometheusRuleName, "prometheus_rule_name", defaultPrometheusRuleName)
		return RequeueAfter(requeueTime, err)
	}

	if c.mdaiCR.Spec.Config != nil && c.mdaiCR.Spec.Config.EvaluationInterval != nil {
		prometheusRule.Spec.Groups[0].Interval = c.mdaiCR.Spec.Config.EvaluationInterval
	}

	rules := make([]prometheusv1.Rule, 0, len(evals))
	for _, eval := range evals {
		rule := c.composePrometheusRule(eval)
		rules = append(rules, rule)
	}

	prometheusRule.Spec.Groups[0].Rules = rules
	if err = c.client.Update(ctx, prometheusRule); err != nil {
		c.logger.Error(err, "Failed to update PrometheusRule")
	}

	return ContinueProcessing()
}

func (c HubAdapter) composePrometheusRule(alertingRule mdaiv1.PrometheusAlert) prometheusv1.Rule {
	alertName := alertingRule.Name

	prometheusRule := prometheusv1.Rule{
		Expr:  alertingRule.Expr,
		Alert: alertName,
		For:   alertingRule.For,
		Annotations: map[string]string{
			"alert_name":    alertName,
			"hub_name":      c.mdaiCR.Name,
			"current_value": "{{ $value | printf \"%.2f\" }}",
			"expression":    alertingRule.Expr.StrVal,
		},
		Labels: map[string]string{
			"severity": alertingRule.Severity,
		},
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

//nolint:gocyclo // TODO refactor later
func (c HubAdapter) ensureVariableSynced(ctx context.Context) (OperationResult, error) {
	// current assumption is we have only built-in Valkey storage
	variables := c.mdaiCR.Spec.Variables
	if variables == nil {
		c.logger.Info("No variables found in the CR, skipping variable synchronization")
		return ContinueProcessing()
	}

	envMap := make(map[string]string)
	manualEnvMap := make(map[string]string)
	dataAdapter := datacore.NewValkeyAdapter(c.valKeyClient, c.zapLogger)
	valkeyKeysToKeep := map[string]struct{}{}
	for _, variable := range variables {
		c.logger.Info(fmt.Sprintf("Processing variable: %s", variable.Key))
		switch variable.StorageType {
		case mdaiv1.VariableSourceTypeBuiltInValkey:
			key := variable.Key
			valkeyKeysToKeep[key] = struct{}{}
			switch variable.Type {
			// from the operator's perspective, computed and manual are the same, they are differently processed by the handler
			case mdaiv1.VariableTypeManual:
				manualEnvMap[key] = string(variable.DataType)
				fallthrough
			case mdaiv1.VariableTypeComputed:
				switch variable.DataType {
				case mdaiv1.VariableDataTypeSet:
					valueAsSlice, err := dataAdapter.GetSetAsStringSlice(ctx, key, c.mdaiCR.Name)
					if err != nil {
						return RequeueAfter(requeueTime, err)
					}
					c.applySetTransformation(variable, envMap, valueAsSlice)
				case mdaiv1.VariableDataTypeString, mdaiv1.VariableDataTypeInt, mdaiv1.VariableDataTypeBoolean:
					// int is represented as string in Valkey, we assume writer guarantee the variable type is correct
					// boolean is represented as string in Valkey: false or true, we assume writer guarantee the variable type is correct
					value, found, err := dataAdapter.GetString(ctx, key, c.mdaiCR.Name)
					if err != nil {
						return RequeueAfter(requeueTime, err)
					}
					if !found {
						continue
					}
					c.applySerializerToString(variable, envMap, value)
				case mdaiv1.VariableDataTypeMap:
					value, err := dataAdapter.GetMapAsString(ctx, key, c.mdaiCR.Name)
					if err != nil {
						return RequeueAfter(requeueTime, err)
					}
					c.applySerializerToString(variable, envMap, value)
				default:
					c.logger.Error(fmt.Errorf("unsupported variable data type: %s", variable.DataType), "key", variable.Key)
					continue
				}
			case mdaiv1.VariableTypeMeta:
				switch variable.DataType {
				case mdaiv1.MetaVariableDataTypePriorityList:
					valueAsSlice, found, err := dataAdapter.GetOrCreateMetaPriorityList(ctx, key, c.mdaiCR.Name, variable.VariableRefs)
					if err != nil {
						return RequeueAfter(requeueTime, err)
					}
					if !found {
						continue
					}
					c.applySetTransformation(variable, envMap, valueAsSlice)
				case mdaiv1.MetaVariableDataTypeHashSet:
					value, found, err := dataAdapter.GetOrCreateMetaHashSet(ctx, key, c.mdaiCR.Name, variable.VariableRefs[0], variable.VariableRefs[1])
					if err != nil {
						return RequeueAfter(requeueTime, err)
					}
					if !found {
						continue
					}
					c.applySerializerToString(variable, envMap, value)
				default:
					c.logger.Error(fmt.Errorf("unsupported variable data type: %s", variable.DataType), "key", variable.Key)
				}
			default:
				c.logger.Error(fmt.Errorf("unsupported variable type: %s", variable.Type), "key", variable.Key)
				continue
			}
		default:
			c.logger.Error(fmt.Errorf("unsupported variable source type: %s", variable.StorageType), "key", variable.Key)
			continue
		}
	}

	c.logger.Info("Deleting old valkey keys", "valkeyKeysToKeep", valkeyKeysToKeep)
	if err := dataAdapter.DeleteKeysWithPrefixUsingScan(ctx, valkeyKeysToKeep, c.mdaiCR.Name); err != nil {
		return OperationResult{}, err
	}

	// manual variables: we need  one ConfigMap for hub in hub's namespace
	switch len(manualEnvMap) {
	case 0:
		{
			c.logger.Info("No manual variables defined in the MDAI CR", "name", c.mdaiCR.Name)
			if err := c.deleteEnvConfigMap(ctx, manualEnvConfigMapNamePostfix, c.mdaiCR.Namespace); err != nil {
				c.logger.Error(err, "Failed to delete manual variables ConfigMap", "name", c.mdaiCR.Name)
				return OperationResult{}, err
			}
		}
	default:
		{
			_, err := c.createOrUpdateEnvConfigMap(ctx, manualEnvMap, manualEnvConfigMapNamePostfix, c.mdaiCR.Namespace, true)
			if err != nil {
				return OperationResult{}, err
			}
		}
	}

	collectors, err := c.listOtelCollectorsWithLabel(ctx, fmt.Sprintf("%s=%s", LabelMdaiHubName, c.mdaiCR.Name))
	if err != nil {
		return OperationResult{}, err
	}

	// assuming collectors could be running in different namespaces, so we need to update envConfigMap in each namespace
	namespaces := make(map[string]struct{})
	for _, collector := range collectors {
		namespaces[collector.Namespace] = struct{}{}
	}

	namespaceToRestart := make(map[string]struct{})
	for namespace := range namespaces {
		// computed variables
		operationResultComputed, err := c.createOrUpdateEnvConfigMap(ctx, envMap, envConfigMapNamePostfix, namespace, false)
		if err != nil {
			return OperationResult{}, err
		}
		if operationResultComputed == controllerutil.OperationResultCreated || operationResultComputed == controllerutil.OperationResultUpdated {
			namespaceToRestart[namespace] = struct{}{}
		}
	}
	auditAdapter := audit.NewAuditAdapter(c.zapLogger, c.valKeyClient, c.valkeyAuditStreamExpiry)

	for _, collector := range collectors {
		if _, shouldRestart := namespaceToRestart[collector.Namespace]; shouldRestart {
			c.logger.Info("Triggering restart of OpenTelemetry Collector", "name", collector.Name)
			collectorCopy := collector.DeepCopy()
			if collectorCopy.Annotations == nil {
				collectorCopy.Annotations = make(map[string]string)
			}
			collectorCopy.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

			if err := c.client.Update(ctx, collectorCopy); err != nil {
				if apierrors.IsConflict(err) {
					c.logger.Info("Conflict while updating OpenTelemetry Collector, will retry", "name", collectorCopy.Name)
					return RequeueAfter(requeueTime, nil)
				}
				c.logger.Error(err, "Failed to update OpenTelemetry Collector", "name", collectorCopy.Name)
				return OperationResult{}, err
			}

			restartEvent := auditAdapter.CreateRestartEvent(c.mdaiCR.Name, envMap)
			restartEventKeyValsForLog := []any{"mdai-logstream", "audit"}
			for key, val := range restartEvent {
				restartEventKeyValsForLog = append(restartEventKeyValsForLog, key, val)
			}
			c.logger.Info("AUDIT: Triggering restart of OpenTelemetry Collector", restartEventKeyValsForLog...)
			if err := auditAdapter.InsertAuditLogEventFromMap(ctx, restartEvent); err != nil {
				return OperationResult{}, err
			}
		}
	}

	return ContinueProcessing()
}

func (c HubAdapter) applySerializerToString(variable mdaiv1.Variable, envMap map[string]string, value string) {
	for _, serializer := range variable.SerializeAs {
		exportedVariableName := serializer.Name
		envMap[exportedVariableName] = value
	}
}

func (c HubAdapter) applySetTransformation(variable mdaiv1.Variable, envMap map[string]string, valueAsSlice []string) {
	for _, serializer := range variable.SerializeAs {
		exportedVariableName := serializer.Name
		if _, exists := envMap[exportedVariableName]; exists {
			c.logger.Info("Serializer configuration overrides existing configuration", "exportedVariableName", exportedVariableName)
			continue
		}

		transformers := serializer.Transformers
		if len(transformers) == 0 {
			c.logger.Info("No Transformers configured", "exportedVariableName", exportedVariableName)
			continue
		}
		for _, transformer := range transformers {
			switch transformer.Type {
			case mdaiv1.TransformerTypeJoin:
				delimiter := transformer.Join.Delimiter
				variableWithDelimiter := strings.Join(valueAsSlice, delimiter)
				envMap[exportedVariableName] = variableWithDelimiter
			default:
				c.logger.Error(fmt.Errorf("unsupported Transformer type: %s", transformer.Type), "exportedVariableName", exportedVariableName)
			}
		}
	}
}

func (c HubAdapter) ensureAutomationsSynchronized(ctx context.Context) (OperationResult, error) {
	if c.mdaiCR.Spec.Automations == nil {
		c.logger.Info("No automations defined in the MDAI CR", "name", c.mdaiCR.Name)
		if err := c.deleteEnvConfigMap(ctx, automationConfigMapNamePostfix, c.mdaiCR.Namespace); err != nil {
			c.logger.Error(err, "Failed to delete automations ConfigMap", "name", c.mdaiCR.Name)
			return OperationResult{}, err
		}
		return ContinueProcessing()
	}
	c.logger.Info("Creating or updating ConfigMap for automations", "name", c.mdaiCR.Name)
	automationMap := make(map[string]string)
	for _, automation := range c.mdaiCR.Spec.Automations {
		key := automation.EventRef
		workflowJSON, err := json.Marshal(automation.Workflow)
		if err != nil {
			return OperationResult{}, fmt.Errorf("failed to marshal automation workflow: %w", err)
		}
		automationMap[key] = string(workflowJSON)
	}
	operationResult, err := c.createOrUpdateEnvConfigMap(ctx, automationMap, automationConfigMapNamePostfix, c.mdaiCR.Namespace, true) // TODO trigger reload of configmap at event hub through some API
	c.logger.Info(fmt.Sprintf("Successfully %s ConfigMap for automations", operationResult), "name", c.mdaiCR.Name)
	if err != nil {
		return OperationResult{}, err
	}
	return ContinueProcessing()
}

func (c HubAdapter) deleteEnvConfigMap(ctx context.Context, postfix string, namespace string) error {
	configMapName := c.mdaiCR.Name + postfix
	c.logger.Info("Deleting ConfigMap", "name", configMapName, "namespace", namespace)
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	if err := c.client.Delete(ctx, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("ConfigMap not found, skipping deletion", "name", configMapName, "namespace", namespace)
			return nil
		}
		c.logger.Error(err, "Failed to delete ConfigMap", "name", configMapName, "namespace", namespace)
		return fmt.Errorf("failed to delete ConfigMap: %w", err)
	}
	return nil
}

func (c HubAdapter) createOrUpdateEnvConfigMap(ctx context.Context, envMap map[string]string, configMapPostfix string, namespace string, setControllerRef bool) (controllerutil.OperationResult, error) {
	envConfigMapName := c.mdaiCR.Name + configMapPostfix
	desiredConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				LabelManagedByMdaiKey: LabelManagedByMdaiValue,
			},
		},
	}
	// we are setting an owner reference here if we need one configmap for a specific Hub in the Hub namespace
	if setControllerRef {
		if err := controllerutil.SetControllerReference(c.mdaiCR, desiredConfigMap, c.scheme); err != nil {
			c.logger.Error(err, "Failed to set owner reference on "+envConfigMapName+" ConfigMap", "configmap", envConfigMapName)
			return "", err
		}
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
		crState, err := c.finalizeHub(ctx)
		if crState == ObjectUnchanged || err != nil {
			c.logger.Info("Has to requeue mdai")
			return RequeueAfter(5*time.Second, err)
		}
		return StopProcessing()
	}
	return ContinueProcessing()
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
	return hex.EncodeToString(sum[:]), nil
}

func (c HubAdapter) ensureStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.mdaiCR.Name, Namespace: c.mdaiCR.Namespace}, c.mdaiCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch Engine")
		return Requeue()
	}
	meta.SetStatusCondition(&c.mdaiCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.mdaiCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update mdai hub status")
		return Requeue()
	}
	c.logger.Info("Status set to done for mdai hub", "mdaiHub", c.mdaiCR.Name)
	return ContinueProcessing()
}
