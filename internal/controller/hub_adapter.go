package controller

import (
	"context"
	"errors"
	"fmt"
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
)

type HubAdapter struct {
	mdaiCR   *mdaiv1.MdaiHub
	logger   logr.Logger
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	context  context.Context
}

type ObjectState bool

func NewHubAdapter(
	ctx context.Context,
	cr *mdaiv1.MdaiHub,
	log logr.Logger,
	client client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme) *HubAdapter {
	return &HubAdapter{
		context:  ctx,
		mdaiCR:   cr,
		logger:   log,
		client:   client,
		recorder: recorder,
		scheme:   scheme}
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

// Utility function to create a map from a slice of any type using a key selector function
func createMapFromSlice[T any, K comparable](slice *[]T, keyFn func(T) K) map[K]T {
	result := make(map[K]T)
	for _, item := range *slice {
		key := keyFn(item)
		result[key] = item
	}
	return result
}

type HydratedEvent struct {
	Trigger *mdaiv1.Trigger
	Actions *[]mdaiv1.Action
}

// AlertFirstEventMap Alert name is key
type AlertFirstEventMap map[string]*[]HydratedEvent

func (c HubAdapter) createHydratedEventMap() (AlertFirstEventMap, error) {
	// Retrieve event map from CR
	eventMap := c.mdaiCR.Spec.EventMap
	if eventMap == nil {
		c.logger.Info("No event map found in the CR, skipping hydration, PrometheusRule synchronization")
		return nil, errors.New("no event map configured")
	}

	// Create temporary maps from the configured slices
	tmpEvalsMap := createMapFromSlice(c.mdaiCR.Spec.Triggers, func(trigger mdaiv1.Trigger) string {
		return trigger.Name
	})
	tmpActionsMap := createMapFromSlice(c.mdaiCR.Spec.Actions, func(action mdaiv1.Action) string {
		return action.Name
	})
	tmpAlertsMap := createMapFromSlice(c.mdaiCR.Spec.Evaluations, func(evaluation mdaiv1.Evaluation) string {
		return evaluation.Name
	})

	// Initialize the hydrated event map
	hydratedEventMap := make(AlertFirstEventMap)

	// Process each Trigger in the event map
	for evalName, actionNames := range *eventMap {
		thisTrigger, evalExists := tmpEvalsMap[evalName]
		if !evalExists {
			// TODO: these `'%s'` not configured logs should probably do something else
			c.logger.Info("Trigger '%s' declared in event map but not configured", evalName)
			continue
		}

		// Check if the corresponding alerting rule is configured
		_, alertExists := tmpAlertsMap[thisTrigger.EvaluationName]
		if !alertExists {
			c.logger.Info("Alert '%s' declared as Trigger dependency but not configured", thisTrigger.EvaluationName)
			continue
		}

		// Collect actions associated with the Trigger
		tmpHydratedActions := make([]mdaiv1.Action, 0)
		for _, actionName := range actionNames {
			thisAction, actionExists := tmpActionsMap[actionName]
			if !actionExists {
				c.logger.Info("Action '%s' declared in event map but not configured", actionName)
				continue
			}
			tmpHydratedActions = append(tmpHydratedActions, thisAction)
		}

		// If there are no valid actions, skip this Trigger
		if len(tmpHydratedActions) == 0 {
			c.logger.Info("No valid actions found for Trigger '%s'", evalName)
			continue
		}

		// Create and append the hydrated event
		tmpHydratedEvent := HydratedEvent{
			Trigger: &thisTrigger,
			Actions: &tmpHydratedActions,
		}

		// Initialize the slice if it's nil for this alerting rule
		if hydratedEventMap[thisTrigger.EvaluationName] == nil {
			*hydratedEventMap[thisTrigger.EvaluationName] = make([]HydratedEvent, 0)
		}

		// Append the hydrated event to the map
		*hydratedEventMap[thisTrigger.EvaluationName] = append(
			*hydratedEventMap[thisTrigger.EvaluationName],
			tmpHydratedEvent,
		)
	}
	// TODO: The keys of this map indicate which alerts should be sent and configured as Prometheus rules
	return hydratedEventMap, nil
}

// EnsurePrometheusRuleSynchronized creates or updates PrometheusFilter CR
func (c HubAdapter) ensureEvaluationsSynchronized() (OperationResult, error) {
	// TODO: Update this such that `evals` is informed by the Evaluation.Name values that serve as keys to the `hydratedEventMap`
	evals := c.mdaiCR.Spec.Triggers
	if evals == nil {
		c.logger.Info("No triggers found in the CR, skipping PrometheusRule synchronization")
		return ContinueProcessing()
	}

	defaultPrometheusRuleName := "mdai-" + c.mdaiCR.Name + "-alert-rules"
	c.logger.Info("EnsurePrometheusRuleSynchronized")

	for _, eval := range *evals {

		prometheusRuleCR, err := c.getOrCreatePrometheusRuleCR(defaultPrometheusRuleName)
		if err != nil {
			c.logger.Error(err, "Failed to get/create PrometheusRule")
			return RequeueAfter(time.Second*10, err)
		}

		if eval.EvaluationName == "" {
			c.logger.Info("Trigger does not depend on alerting rule, skipping PrometheusRule synchronization")
			continue
		}

		var evaluation mdaiv1.Evaluation
		evaluationFound := false

		for _, rule := range *c.mdaiCR.Spec.Evaluations {
			if rule.Name == eval.EvaluationName {
				evaluation = rule
				evaluationFound = true
				break
			}
		}

		if evaluationFound {
			prometheusRuleCR.Spec.Groups[0].Rules = composePrometheusRule(
				evaluation, c.mdaiCR.Name)
		} else {
			c.logger.Info("Alert '%s' declared as Trigger dependency but not configured", eval.EvaluationName)
		}

		if err = c.client.Update(c.context, prometheusRuleCR); err != nil {
			c.logger.Error(err, "Failed to update PrometheusRule")
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

func composePrometheusRule(evaluation mdaiv1.Evaluation, engineName string) []prometheusv1.Rule {
	prometheusRule := prometheusv1.Rule{
		Expr:          evaluation.Expr,
		Alert:         evaluation.Name,
		For:           evaluation.For,
		KeepFiringFor: evaluation.KeepFiringFor,
		Annotations: map[string]string{
			"alert_name":  evaluation.Name, // FIXME we need a relationship between alert and variable
			"engine_name": engineName,
		},
		Labels: map[string]string{
			"severity": evaluation.Severity,
		},
	}

	return []prometheusv1.Rule{prometheusRule}
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
	// TODO: Implement this function
	// here we update variables for OTEL collectors that has labels with hub name
	// we sort variables by source type
	// next process each source type and update variables
	return ContinueProcessing()
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
