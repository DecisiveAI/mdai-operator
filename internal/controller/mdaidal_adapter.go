package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/decisiveai/mdai-operator/internal/builder"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	MdaiDalHubComponent  = "mdai-dal"
	MdaiDalConfigMapName = "mdai-dal-config"
)

type MdaiDalAdapter struct {
	dalCR  *mdaiv1.MdaiDal
	logger logr.Logger
	client client.Client
	scheme *runtime.Scheme
}

func NewMdaiDalAdapter(
	dalCR *mdaiv1.MdaiDal,
	log logr.Logger,
	k8sClient client.Client,
	scheme *runtime.Scheme,
) *MdaiDalAdapter {
	return &MdaiDalAdapter{
		dalCR:  dalCR,
		logger: log,
		client: k8sClient,
		scheme: scheme,
	}
}

func (c MdaiDalAdapter) configMapName() string {
	return fmt.Sprintf("%s-%s", c.dalCR.Name, MdaiDalConfigMapName)
}

func (c MdaiDalAdapter) ensureDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if c.dalCR.DeletionTimestamp.IsZero() {
		return ContinueProcessing()
	}
	c.logger.Info("Deleting DAL:" + c.dalCR.Name)
	crState, err := c.finalize(ctx)
	if crState == ObjectUnchanged || err != nil {
		c.logger.Info("Has to requeue mdai")
		return RequeueAfter(requeueTime, err)
	}
	return StopProcessing()
}

func (c MdaiDalAdapter) ensureStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.dalCR.Name, Namespace: c.dalCR.Namespace}, c.dalCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch MDAI DAL")
		return Requeue()
	}

	deployment := &appsv1.Deployment{}
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.dalCR.Name, Namespace: c.dalCR.Namespace}, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("Deployment not found yet, requeueing for status update")
			return Requeue()
		}
		c.logger.Error(err, "Failed to get MDAI DAL deployment for status")
		return Requeue()
	}

	service := &corev1.Service{}
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.dalCR.Name, Namespace: c.dalCR.Namespace}, service); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("Service not found yet, requeueing for status update")
			return Requeue()
		}
		c.logger.Error(err, "Failed to get MDAI DAL service for status")
		return Requeue()
	}

	c.dalCR.Status.ObservedGeneration = c.dalCR.Generation
	c.dalCR.Status.Replicas = deployment.Status.Replicas
	c.dalCR.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	c.dalCR.Status.Endpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, otlpHTTPPort)
	c.dalCR.Status.LastSyncedTime = metav1.Now()

	meta.SetStatusCondition(&c.dalCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.dalCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update MDAI DAL status")
		return Requeue()
	}
	c.logger.Info("Status set to done for MDAI DAL", "mdaiHub", c.dalCR.Name)
	return ContinueProcessing()
}

func (c MdaiDalAdapter) ensureFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if controllerutil.ContainsFinalizer(c.dalCR, hubFinalizer) {
		return ContinueProcessing()
	}
	c.logger.Info("Adding Finalizer for MdaiDalAdapter")
	if ok := controllerutil.AddFinalizer(c.dalCR, hubFinalizer); !ok {
		c.logger.Error(nil, "Failed to add finalizer into the custom resource")
		return RequeueWithError(errors.New("failed to add finalizer " + hubFinalizer))
	}

	if err := c.client.Update(ctx, c.dalCR); err != nil {
		c.logger.Error(err, "Failed to update custom resource to add finalizer")
		return RequeueWithError(err)
	}
	return StopProcessing() // when finalizer is added it will trigger reconciliation
}

func (c MdaiDalAdapter) ensureStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.dalCR.Status.Conditions) != 0 {
		return ContinueProcessing()
	}
	meta.SetStatusCondition(&c.dalCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	if err := c.client.Status().Update(ctx, c.dalCR); err != nil {
		c.logger.Error(err, "Failed to update MDAI DAL status")
		return RequeueWithError(err)
	}
	c.logger.Info("Re-queued to reconcile with updated status")
	return StopProcessing()
}

func (c MdaiDalAdapter) finalize(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.dalCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for MDAI DAL before delete CR")

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.dalCR.Name, Namespace: c.dalCR.Namespace}, c.dalCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("Cluster has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "Failed to re-fetch MdaiHub")
		return ObjectUnchanged, err
	}

	if meta.SetStatusCondition(&c.dalCR.Status.Conditions, metav1.Condition{
		Type:    typeDegradedHub,
		Status:  metav1.ConditionTrue,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.dalCR.Name),
	}) {
		if err := c.client.Status().Update(ctx, c.dalCR); err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Info("Cluster has been deleted, no need to finalize")
				return ObjectModified, nil
			}
			c.logger.Error(err, "Failed to update MDAI DAL status")

			return ObjectUnchanged, err
		}
	}

	c.logger.Info("Removing Finalizer for MDAI DAL after successfully perform the operations")
	if err := c.ensureFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

func (c MdaiDalAdapter) ensureFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting MDAI DAL Finalizer")
	return c.deleteFinalizer(ctx, c.dalCR, hubFinalizer)
}

func (c MdaiDalAdapter) deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error {
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

func (c MdaiDalAdapter) ensureSynchronized(ctx context.Context) (OperationResult, error) {
	log := logger.FromContext(ctx)
	log.Info("Ensuring Synchronized MDAI DAL")

	if err := c.createOrUpdateConfigMap(ctx); err != nil {
		log.Error(err, "Failed to create or update MDAI DAL ConfigMap")
		return RequeueOnErrorOrContinue(err)
	}

	if err := c.createOrUpdateService(ctx); err != nil {
		log.Error(err, "Failed to create or update MDAI DAL service")
		return RequeueOnErrorOrContinue(err)
	}

	if err := c.createOrUpdateDeployment(ctx); err != nil {
		log.Error(err, "Failed to create or update MDAI DAL deployment")
		return RequeueOnErrorOrContinue(err)
	}

	return ContinueProcessing()
}

func (c MdaiDalAdapter) createOrUpdateConfigMap(ctx context.Context) error {
	const envPrefix = "MDAI_DAL_"
	configMapName := c.configMapName()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: c.dalCR.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, configMap, func() error {
		if err := controllerutil.SetControllerReference(c.dalCR, configMap, c.scheme); err != nil {
			return err
		}
		if configMap.Labels == nil {
			configMap.Labels = map[string]string{}
		}
		configMap.Labels[LabelAppNameKey] = MdaiDalHubComponent
		configMap.Labels[LabelAppInstanceKey] = c.dalCR.Name
		configMap.Labels[LabelManagedByMdaiKey] = LabelManagedByMdaiValue
		configMap.Labels[HubComponentLabel] = MdaiDalHubComponent
		configMap.Data = map[string]string{
			envPrefix + "S3_BUCKET":      c.dalCR.Spec.S3.Bucket,
			envPrefix + "AWS_REGION":     c.dalCR.Spec.AWS.Region,
			envPrefix + "S3_PREFIX":      c.dalCR.Spec.S3.Prefix,
			envPrefix + "GRANULARITY":    c.dalCR.Spec.Granularity,
			envPrefix + "USE_PAYLOAD_TS": strconv.FormatBool(c.dalCR.Spec.UsePayloadTS),
			envPrefix + "MARSHALER":      c.dalCR.Spec.Marshaler,
		}

		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update mdai-dal-config ConfigMap", "configmap", configMapName)
		return err
	}

	c.logger.Info("mdai-dal-config ConfigMap created or updated successfully", "configmap", configMapName, "operation", operationResult)
	return nil
}

func (c MdaiDalAdapter) createOrUpdateService(ctx context.Context) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.dalCR.Name,
			Namespace: c.dalCR.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if err := controllerutil.SetControllerReference(c.dalCR, service, c.scheme); err != nil {
			return err
		}

		builder.Service(service).
			WithLabel(LabelAppNameKey, MdaiDalHubComponent).
			WithLabel(LabelAppInstanceKey, c.dalCR.Name).
			WithLabel(LabelManagedByMdaiKey, LabelManagedByMdaiValue).
			WithLabel(HubComponentLabel, MdaiDalHubComponent).
			WithSelectorLabel(LabelAppNameKey, MdaiDalHubComponent).
			WithSelectorLabel(LabelAppInstanceKey, c.dalCR.Name).
			WithPorts(
				corev1.ServicePort{
					Port:       otlpGRPCPort,
					Name:       otlpGRPCName,
					TargetPort: intstr.FromInt32(otlpGRPCPort),
					Protocol:   corev1.ProtocolTCP,
				},
				corev1.ServicePort{
					Port:       otlpHTTPPort,
					Name:       otlpHTTPName,
					TargetPort: intstr.FromInt32(otlpHTTPPort),
					Protocol:   corev1.ProtocolTCP,
				},
			)

		if service.CreationTimestamp.IsZero() {
			builder.Service(service).
				WithType(corev1.ServiceTypeClusterIP)
		}

		return nil
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			c.logger.Info("Service already exists, skipping",
				"service", service.Name,
				"namespace", service.Namespace,
			)
			return nil
		}

		return fmt.Errorf("failed to create or update mdai dal service: %w", err)
	}

	c.logger.Info("Successfully created or updated "+c.dalCR.Name+" Service", "service", c.dalCR.Name, "namespace", c.dalCR.Namespace, "operation", operationResult)
	return nil
}

func (c MdaiDalAdapter) createOrUpdateDeployment(ctx context.Context) error {
	awsCredentials := c.dalCR.Spec.AWS.Credentials
	configMapName := c.configMapName()
	livenessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.FromInt32(otlpHTTPPort),
			},
		},
		InitialDelaySeconds: 10, //nolint:mnd
		PeriodSeconds:       10, //nolint:mnd
	}
	readinessProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.FromInt32(otlpHTTPPort),
			},
		},
		InitialDelaySeconds: 3, //nolint:mnd
		PeriodSeconds:       5, //nolint:mnd
	}

	container := builder.Container(c.dalCR.Name, imageRef(*c.dalCR.Spec.ImageSpec)).
		WithImagePullPolicy(c.dalCR.Spec.ImageSpec.PullPolicy.ToK8s()).
		WithPorts(
			corev1.ContainerPort{ContainerPort: otlpGRPCPort, Name: otlpGRPCName, Protocol: corev1.ProtocolTCP},
			corev1.ContainerPort{ContainerPort: otlpHTTPPort, Name: otlpHTTPName, Protocol: corev1.ProtocolTCP},
		).
		WithEnvFromSecretKey("AWS_ACCESS_KEY_ID", awsCredentials.SecretName, awsCredentials.AccessKeyField).
		WithEnvFromSecretKey("AWS_SECRET_ACCESS_KEY", awsCredentials.SecretName, awsCredentials.SecretKeyField).
		WithEnvFromConfigMap(configMapName).
		WithSecurityContext(DefaultSecurityContext).
		WithLivenessProbe(livenessProbe).
		WithReadinessProbe(readinessProbe)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.dalCR.Name,
			Namespace: c.dalCR.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if err := controllerutil.SetControllerReference(c.dalCR, deployment, c.scheme); err != nil {
			return err
		}

		if deployment.CreationTimestamp.IsZero() {
			builder.Deployment(deployment).
				WithSelectorLabel(LabelAppNameKey, MdaiDalHubComponent).
				WithSelectorLabel(LabelManagedByMdaiKey, LabelManagedByMdaiValue).
				WithSelectorLabel(LabelAppInstanceKey, c.dalCR.Name)
		}

		builder.Deployment(deployment).
			WithReplicas(1).
			WithLabel(LabelAppNameKey, MdaiDalHubComponent).
			WithLabel(LabelAppInstanceKey, c.dalCR.Name).
			WithLabel(LabelManagedByMdaiKey, LabelManagedByMdaiValue).
			WithLabel(HubComponentLabel, MdaiDalHubComponent).
			WithTemplateLabel(LabelAppNameKey, MdaiDalHubComponent).
			WithTemplateLabel(LabelAppInstanceKey, c.dalCR.Name).
			WithTemplateLabel(LabelManagedByMdaiKey, LabelManagedByMdaiValue).
			WithContainers(container.Build())

		return nil
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			c.logger.Info("Deployment already exists, skipping",
				"deployment", deployment.Name,
				"namespace", deployment.Namespace,
			)
			return nil
		}
		return fmt.Errorf("failed to create or update mdai dal deployment: %w", err)
	}
	c.logger.Info("Successfully created or updated "+c.dalCR.Name+" Deployment", "service", c.dalCR.Name, "namespace", c.dalCR.Namespace, "operation", operationResult)
	return nil
}

func imageRef(i mdaiv1.MdaiDalImageSpec) string {
	switch {
	case i.Repository == "":
		return ""
	case i.Tag == "":
		return i.Repository
	default:
		return i.Repository + ":" + i.Tag
	}
}
