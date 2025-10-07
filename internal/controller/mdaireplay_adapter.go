package controller

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
	"slices"
)

const (
	mdaiReplayFinalizerName     = "mydecisive.ai/replay_finalizer"
	replayNameKey               = "replay_name"
	hubNameKey                  = "hub_name"
	replayerDefaultImage        = "otel/opentelemetry-collector-contrib:0.136.0"
	replayCollectorHubComponent = "mdai-replay"
)

//go:embed config/replay_collector_base_config.yaml
var baseReplayCollectorYaml string

var _ Adapter = (*MdaiReplayAdapter)(nil)

type MdaiReplayAdapter struct {
	replayCR *mdaiv1.MdaiReplay
	logger   logr.Logger
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
}

func NewMdaiReplayAdapter(
	cr *mdaiv1.MdaiReplay,
	log logr.Logger,
	k8sClient client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
) *MdaiReplayAdapter {
	return &MdaiReplayAdapter{
		replayCR: cr,
		logger:   log,
		client:   k8sClient,
		recorder: recorder,
		scheme:   scheme,
	}
}

func (c MdaiReplayAdapter) ensureDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if c.replayCR.DeletionTimestamp.IsZero() {
		return ContinueProcessing()
	}
	c.logger.Info("Deleting MdaiReplay:" + c.replayCR.Name)
	crState, err := c.finalize(ctx)
	if crState == ObjectUnchanged || err != nil {
		c.logger.Info("Has to requeue mdaireplay")
		return RequeueAfter(requeueTime, err)
	}
	return StopProcessing()
}

func (c MdaiReplayAdapter) ensureFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if controllerutil.ContainsFinalizer(c.replayCR, mdaiReplayFinalizerName) {
		return ContinueProcessing()
	}
	c.logger.Info("Adding Finalizer for MdaiReplay")
	if ok := controllerutil.AddFinalizer(c.replayCR, mdaiReplayFinalizerName); !ok {
		c.logger.Error(nil, "Failed to add finalizer into the custom resource")
		return RequeueWithError(errors.New("failed to add finalizer " + mdaiReplayFinalizerName))
	}

	if err := c.client.Update(ctx, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to update custom resource to add finalizer")
		return RequeueWithError(err)
	}
	return StopProcessing() // when finalizer is added it will trigger reconciliation
}

func (c MdaiReplayAdapter) ensureFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting MdaiReplay Finalizer")
	return c.deleteFinalizer(ctx, c.replayCR, mdaiReplayFinalizerName)
}

func (c MdaiReplayAdapter) ensureStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.replayCR.Status.Conditions) != 0 {
		return ContinueProcessing()
	}
	meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to update MdaiReplay status")
		return RequeueWithError(err)
	}
	c.logger.Info("Re-queued to reconcile with updated status")
	return StopProcessing()
}

func (c MdaiReplayAdapter) ensureStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.replayCR.Name, Namespace: c.replayCR.Namespace}, c.replayCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch MdaiReplay")
		return Requeue()
	}
	meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update mdai replay status")
		return Requeue()
	}
	c.logger.Info("Status set to done for mdai replay", "mdaiHub", c.replayCR.Name)
	return ContinueProcessing()
}

func (c MdaiReplayAdapter) ensureSynchronized(ctx context.Context) (OperationResult, error) {
	c.logger.Info("🐙 Synchronizing MdaiReplay 💬")
	hash, err := c.createOrUpdateReplayerConfigMap(ctx)
	if err != nil {
		return RequeueWithError(err)
	}

	if err := c.createOrUpdateReplayerDeployment(ctx, hash); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		return RequeueWithError(err)
	}

	if err := c.createOrUpdatReplayerService(ctx); err != nil {
		return RequeueWithError(err)
	}
	//replays := c.replayCR.Spec.replays
	//replayResource := c.replayCR.Spec.replayResource
	//
	//if len(replays) == 0 {
	//	c.logger.Info("No replays found in the CR, skipping replay synchronization")
	//	return ContinueProcessing()
	//}
	//
	//hash, err := c.createOrUpdatereplayResourceConfigMap(ctx, replayResource, replays)
	//if err != nil {
	//	return RequeueWithError(err)
	//}
	//
	//if err := c.createOrUpdatereplayResourceDeployment(ctx, c.replayCR.Namespace, hash, replayResource); err != nil {
	//	if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
	//		c.logger.Info("re-queuing due to resource conflict")
	//		return Requeue()
	//	}
	//	return RequeueWithError(err)
	//}
	//
	//if err := c.createOrUpdatereplayResourceService(ctx, c.replayCR.Namespace); err != nil {
	//	return RequeueWithError(err)
	//}

	c.logger.Info("🐙 MdaiReplay sync finished ✅")
	return ContinueProcessing()
}

func (c MdaiReplayAdapter) getReplayerResourceName(suffix string) string {
	return fmt.Sprintf("replay-%s-%s-%s", c.replayCR.Name, c.replayCR.Spec.HubName, suffix)
}

func (c MdaiReplayAdapter) createOrUpdateReplayerConfigMap(ctx context.Context) (string, error) {
	namespace := c.replayCR.Namespace
	configMapName := c.getReplayerResourceName("collector-config")
	collectorYAML, err := c.getReplayCollectorConfigYAML(c.replayCR.Name, c.replayCR.Spec.HubName)

	if err != nil {
		return "", fmt.Errorf("‼️failed to build replay configuration: %w", err)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                 c.getReplayerResourceName("collector"),
				LabelManagedByMdaiKey: LabelManagedByMdaiValue,
				HubComponentLabel:     mdaiObserverHubComponent,
				hubNameLabel:          c.replayCR.Spec.HubName,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}

	if err := controllerutil.SetControllerReference(c.replayCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "‼️Failed to set owner reference on ConfigMap", "configmap", configMapName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "‼️Failed to create or update ConfigMap", "configmap", configMapName)
		return "", err
	}

	c.logger.Info("‼️ConfigMap created or updated successfully", "configmap", configMapName, "operation", operationResult)
	return getConfigMapSHA(*desiredConfigMap)
}

func (c MdaiReplayAdapter) getReplayCollectorConfigYAML(replayId string, hubName string) (string, error) {
	replayCRSpec := c.replayCR.Spec
	var config builder.ConfigBlock

	if err := yaml.Unmarshal([]byte(baseReplayCollectorYaml), &config); err != nil {
		c.logger.Error(err, "Failed to unmarshal base collector config")
		return "", fmt.Errorf(`unmarshal base collector config: %w`, err)
	}

	// Add replayId insert processor.
	// Assumed to already be in service.pipelines.logs/replay.processors slice as it is not optional!
	attrProcessor := config.MustMap("processors").MustMap("attributes")
	attrProcessorActions := attrProcessor.MustSlice("actions")
	upsertReplayIdAction := map[string]string{
		"key":    replayNameKey,
		"action": "upsert",
		"value":  replayId,
	}
	newActions := append(attrProcessorActions, upsertReplayIdAction)
	attrProcessor.Set("actions", newActions)

	// Add otlphttp exporter if otlphttp destination is set
	otlpEndpoint := replayCRSpec.Destination.OtlpHttp.Endpoint
	if otlpEndpoint != "" {
		exporters := config.MustMap("exporters")
		exporters.Set("otlphttp", map[string]any{
			"endpoint": otlpEndpoint,
		})
		config.Set("exporters", exporters)
		logsReplayPipeline := config.MustMap("service").MustMap("pipelines").MustMap("logs/replay")
		logsReplayExporters := append(logsReplayPipeline.MustSlice("exporters"), "otlphttp")
		logsReplayPipeline.Set("exporters", logsReplayExporters)
	}

	// Add S3 receiver thangz
	s3ReceiverConfig := config.MustMap("receivers").MustMap("awss3")
	s3DownloaderConfig := s3ReceiverConfig.MustMap("s3downloader")
	s3ReceiverConfig.Set("starttime", replayCRSpec.StartTime)
	s3ReceiverConfig.Set("endtime", replayCRSpec.EndTime)
	s3DownloaderConfig.Set("file_prefix", replayCRSpec.Source.S3.FilePrefix)
	s3DownloaderConfig.Set("region", replayCRSpec.Source.S3.S3Region)
	s3DownloaderConfig.Set("s3_bucket", replayCRSpec.Source.S3.S3Bucket)
	s3DownloaderConfig.Set("s3_prefix", replayCRSpec.Source.S3.S3Path)
	s3DownloaderConfig.Set("s3_partition", replayCRSpec.Source.S3.S3Partition)

	// Set up OpAMP Agent config
	opampExtension := config.MustMap("extensions").MustMap("opamp")
	agentDescription := opampExtension.MustMap("agent_description")
	replayerAttributes := agentDescription.MustMap("non_identifying_attributes")
	replayerAttributes.Set("replay_id", replayId)
	replayerAttributes.Set("hub_name", hubName)
	agentDescription.Set("non_identifying_attributes", replayerAttributes)
	opampServer := opampExtension.MustMap("server")
	opampServer.MustMap("http").Set("endpoint", replayCRSpec.OpAMPEndpoint)
	opampExtension.Set("server", opampServer)

	return config.YAML()
}

func (c MdaiReplayAdapter) createOrUpdateReplayerDeployment(ctx context.Context, configHash string) error {
	name := c.getReplayerResourceName("collector")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.replayCR.Namespace,
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if err := controllerutil.SetControllerReference(c.replayCR, deployment, c.scheme); err != nil {
			c.logger.Error(err, "‼️Failed to set owner reference on Deployment", "deployment", deployment.Name)
			return err
		}

		image := replayerDefaultImage
		if c.replayCR.Spec.Resource.Image != "" {
			image = c.replayCR.Spec.Resource.Image
		}
		container := builder.Container(name, image).
			WithCommand("/otelcol-contrib", "--config=/conf/collector.yaml").
			WithPorts(
				corev1.ContainerPort{ContainerPort: promHTTPPort, Name: "prom-http"},
			).
			WithVolumeMounts(corev1.VolumeMount{
				Name:      "config-volume",
				MountPath: "/conf/collector.yaml",
				SubPath:   "collector.yaml",
			}).
			WithEnvFrom(corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: c.getReplayerResourceName("collector-config"),
					},
				},
			}).
			WithAWSSecret(c.replayCR.Spec.Source.AWSConfig.AWSAccessKeySecret).
			Build()

		builder.Deployment(deployment).
			WithLabel("app", name).
			WithLabel(hubNameLabel, c.replayCR.Name).
			WithLabel(LabelManagedByMdaiKey, LabelManagedByMdaiValue).
			WithLabel(HubComponentLabel, replayCollectorHubComponent).
			WithSelectorLabel("app", name).
			WithTemplateLabel("app", name).
			WithTemplateLabel("app.kubernetes.io/component", name).
			WithTemplateAnnotation("replay-collector-config/sha256", configHash).
			WithReplicas(1).
			WithContainers(container).
			WithVolumes(corev1.Volume{
				Name: "config-volume",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: c.getReplayerResourceName("collector-config"),
						},
					},
				},
			})

		return nil
	})
	if err != nil {
		return err
	}
	c.logger.Info("✅ Deployment created or updated successfully", "deployment", deployment.Name, "operationResult", operationResult)

	return nil
}

func (c MdaiReplayAdapter) createOrUpdatReplayerService(ctx context.Context) error {
	name := c.getReplayerResourceName("service")
	appLabel := c.getReplayerResourceName("collector")

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.replayCR.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(c.replayCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on Service", "service", name)
		return err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = map[string]string{
				"app":                 appLabel,
				hubNameLabel:          c.replayCR.Name,
				HubComponentLabel:     mdaiObserverHubComponent,
				LabelManagedByMdaiKey: LabelManagedByMdaiValue,
			}
		}

		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabel,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "otelcol-metrics",
					Protocol:   corev1.ProtocolTCP,
					Port:       otelMetricsPort,
					TargetPort: intstr.FromString("otelcol-metrics"),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("‼️failed to create or update %s: %w", name, err)
	}

	c.logger.Info("✅ Successfully created or updated "+name, "service", name, "namespace", c.replayCR.Namespace, "operation", operationResult)
	return nil
}

func (c MdaiReplayAdapter) checkPrometheusForEmptySendingQueue(ctx context.Context) error {
	return nil
}

func (c MdaiReplayAdapter) finalize(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.replayCR, mdaiReplayFinalizerName) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("💬Performing Finalizer Operations for MdaiReplay before delete CR")

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.replayCR.Name, Namespace: c.replayCR.Namespace}, c.replayCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("✅MdaiReplay has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "‼️Failed to re-fetch MdaiReplay")
		return ObjectUnchanged, err
	}

	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF
	// TODO FINALIZER STUFFFFFFF

	//if meta.SetStatusCondition(&c.replayCR.Status.Conditions, metav1.Condition{
	//	Type:    typeDegradedHub,
	//	Status:  metav1.ConditionTrue,
	//	Reason:  "Finalizing",
	//	Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.replayCR.Name),
	//}) {
	//	if err := c.client.Status().Update(ctx, c.replayCR); err != nil {
	//		if apierrors.IsNotFound(err) {
	//			c.logger.Info("MdaiReplay has been deleted, no need to finalize")
	//			return ObjectModified, nil
	//		}
	//		c.logger.Error(err, "Failed to update MdaiReplay status")
	//
	//		return ObjectUnchanged, err
	//	}
	//}

	c.logger.Info("Removing Finalizer for MdaiReplay after successfully perform the operations")
	if err := c.ensureFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

func (c MdaiReplayAdapter) deleteFinalizer(ctx context.Context, object client.Object, finalizer string) error {
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
