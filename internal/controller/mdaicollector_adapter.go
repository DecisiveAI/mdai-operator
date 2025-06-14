package controller

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"time"

	"errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

type otlpTlsConfig struct {
	Insecure bool `yaml:"insecure" json:"insecure"`
}

type otlpExporterConfig struct {
	Endpoint  string        `yaml:"endpoint" json:"endpoint"`
	TlsConfig otlpTlsConfig `yaml:"tls" json:"tls"`
}

type s3ExporterConfig struct {
	S3Uploader s3UploaderConfig `yaml:"s3uploader" json:"s3uploader"`
}

type s3UploaderConfig struct {
	Region            string `yaml:"region" json:"region"`
	S3Bucket          string `yaml:"s3_bucket" json:"s3_bucket"`
	S3Prefix          string `yaml:"s3_prefix" json:"s3_prefix"`
	S3PartitionFormat string `yaml:"s3_partition_format" json:"s3_partition_format"`
	FilePrefix        string `yaml:"file_prefix" json:"file_prefix"`
	DisableSSL        bool   `yaml:"disable_ssl" json:"disable_ssl"`
}

type routingConnectorTableEntry struct {
	Context   string   `yaml:"context" json:"context"`
	Condition string   `yaml:"condition" json:"condition"`
	Pipelines []string `yaml:"pipelines" json:"pipelines"`
}

const (
	MdaiCollectorHubComponent     = "mdai-collector"
	mdaiCollectorResourceNameBase = "mdai-collector"

	S3PartitionFormat = "%Y/%m/%d/%H"
)

var (
	//go:embed config/mdai_collector_base_config.yaml
	baseMdaiCollectorYAML string
	severityFilterMap     = map[mdaiv1.SeverityLevel]string{
		mdaiv1.DebugSeverityLevel: "filter/severity_min_debug",
		mdaiv1.InfoSeverityLevel:  "filter/severity_min_info",
		mdaiv1.WarnSeverityLevel:  "filter/severity_min_warn",
		mdaiv1.ErrorSeverityLevel: "filter/severity_min_error",
	}
	routingTableContextMap = map[mdaiv1.MDAILogStream]string{
		mdaiv1.CollectorLogstream: "resource",
		mdaiv1.HubLogstream:       "resource",
		mdaiv1.AuditLogstream:     "log",
	}
	routingTableConditionMap = map[mdaiv1.MDAILogStream]string{
		mdaiv1.CollectorLogstream: "attributes[\"mdai-logstream\"] == \"collector\"",
		mdaiv1.HubLogstream:       "attributes[\"mdai-logstream\"] == \"hub\"",
		mdaiv1.AuditLogstream:     "attributes[\"mdai-logstream\"] == \"audit\"",
	}
)

type MdaiCollectorAdapter struct {
	collectorCR *mdaiv1.MdaiCollector
	logger      logr.Logger
	client      client.Client
	recorder    record.EventRecorder
	scheme      *runtime.Scheme
}

func NewMdaiCollectorAdapter(
	cr *mdaiv1.MdaiCollector,
	log logr.Logger,
	client client.Client,
	recorder record.EventRecorder,
	scheme *runtime.Scheme,
) *MdaiCollectorAdapter {
	return &MdaiCollectorAdapter{
		collectorCR: cr,
		logger:      log,
		client:      client,
		recorder:    recorder,
		scheme:      scheme,
	}
}

func (c MdaiCollectorAdapter) ensureMdaiCollectorFinalizerInitialized(ctx context.Context) (OperationResult, error) {
	if !controllerutil.ContainsFinalizer(c.collectorCR, hubFinalizer) {
		c.logger.Info("Adding Finalizer for MdaiHub")
		if ok := controllerutil.AddFinalizer(c.collectorCR, hubFinalizer); !ok {
			c.logger.Error(nil, "Failed to add finalizer into the custom resource")
			return RequeueWithError(errors.New("failed to add finalizer " + hubFinalizer))
		}

		if err := c.client.Update(ctx, c.collectorCR); err != nil {
			c.logger.Error(err, "Failed to update custom resource to add finalizer")
			return RequeueWithError(err)
		}
		return StopProcessing() // when finalizer is added it will trigger reconciliation
	}
	return ContinueProcessing()
}

func (c MdaiCollectorAdapter) ensureMdaiCollectorStatusInitialized(ctx context.Context) (OperationResult, error) {
	if len(c.collectorCR.Status.Conditions) == 0 {
		meta.SetStatusCondition(&c.collectorCR.Status.Conditions, metav1.Condition{Type: typeAvailableHub, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err := c.client.Status().Update(ctx, c.collectorCR); err != nil {
			c.logger.Error(err, "Failed to update MDAI Collector status")
			return RequeueWithError(err)
		}
		c.logger.Info("Re-queued to reconcile with updated status")
		return StopProcessing()
	}
	return ContinueProcessing()
}

// finalizeHub handles the deletion of a hub
func (c MdaiCollectorAdapter) finalizeMdaiCollector(ctx context.Context) (ObjectState, error) {
	if !controllerutil.ContainsFinalizer(c.collectorCR, hubFinalizer) {
		c.logger.Info("No finalizer found")
		return ObjectModified, nil
	}

	c.logger.Info("Performing Finalizer Operations for MDAI Collector before delete CR")

	if err := c.client.Get(ctx, types.NamespacedName{Name: c.collectorCR.Name, Namespace: c.collectorCR.Namespace}, c.collectorCR); err != nil {
		if apierrors.IsNotFound(err) {
			c.logger.Info("Cluster has been deleted, no need to finalize")
			return ObjectModified, nil
		}
		c.logger.Error(err, "Failed to re-fetch MdaiHub")
		return ObjectUnchanged, err
	}

	if meta.SetStatusCondition(&c.collectorCR.Status.Conditions, metav1.Condition{
		Type:    typeDegradedHub,
		Status:  metav1.ConditionTrue,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", c.collectorCR.Name),
	}) {
		if err := c.client.Status().Update(ctx, c.collectorCR); err != nil {
			if apierrors.IsNotFound(err) {
				c.logger.Info("Cluster has been deleted, no need to finalize")
				return ObjectModified, nil
			}
			c.logger.Error(err, "Failed to update MDAI Collector status")

			return ObjectUnchanged, err
		}
	}

	c.logger.Info("Removing Finalizer for MDAI Collector after successfully perform the operations")
	if err := c.ensureMdaiCollectorFinalizerDeleted(ctx); err != nil {
		return ObjectUnchanged, err
	}

	return ObjectModified, nil
}

// ensureHubFinalizerDeleted removes finalizer of a Hub
func (c MdaiCollectorAdapter) ensureMdaiCollectorFinalizerDeleted(ctx context.Context) error {
	c.logger.Info("Deleting MDAI Collector Finalizer")
	return c.deleteMdaiCollectorFinalizer(ctx, c.collectorCR, hubFinalizer)
}

// deleteFinalizer deletes finalizer of a generic CR
func (c MdaiCollectorAdapter) deleteMdaiCollectorFinalizer(ctx context.Context, object client.Object, finalizer string) error {
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

func (c MdaiCollectorAdapter) ensureMdaiCollectorSynchronized(ctx context.Context) (OperationResult, error) {
	namespace := c.collectorCR.Namespace
	var (
		awsConfig          *mdaiv1.AWSConfig
		logsConfig         *mdaiv1.LogsConfig
		awsAccessKeySecret *string
	)
	awsConfig = c.collectorCR.Spec.AWSConfig
	if awsConfig != nil {
		awsAccessKeySecret = awsConfig.AWSAccessKeySecret
	}
	logsConfig = c.collectorCR.Spec.Logs

	collectorConfigMapName, hash, err := c.createOrUpdateMdaiCollectorConfigMap(ctx, namespace, logsConfig, awsAccessKeySecret)
	if err != nil {
		return OperationResult{}, err
	}
	collectorEnvConfigMapName, err := c.createOrUpdateMdaiCollectorEnvVarConfigMap(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	serviceAccountName, err := c.createOrUpdateMdaiCollectorServiceAccount(ctx, namespace)
	if err != nil {
		return OperationResult{}, err
	}
	roleName, err := c.createOrUpdateMdaiCollectorRole(ctx)
	if err != nil {
		return OperationResult{}, err
	}
	err = c.createOrUpdateMdaiCollectorRoleBinding(ctx, namespace, roleName, serviceAccountName)
	if err != nil {
		return OperationResult{}, err
	}

	deploymentName, err := c.createOrUpdateMdaiCollectorDeployment(ctx, namespace, collectorConfigMapName, collectorEnvConfigMapName, serviceAccountName, awsAccessKeySecret, hash)
	if err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		return OperationResult{}, err
	}

	if _, err := c.createOrUpdateMdaiCollectorService(ctx, namespace, deploymentName); err != nil {
		return OperationResult{}, err
	}

	return ContinueProcessing()
}

func (c MdaiCollectorAdapter) getScopedMdaiCollectorResourceName(postfix string) string {
	if postfix != "" {
		return fmt.Sprintf("%s-%s-%s", c.collectorCR.Name, mdaiCollectorResourceNameBase, postfix)
	}
	return fmt.Sprintf("%s-%s", c.collectorCR.Name, mdaiCollectorResourceNameBase)
}

func (c MdaiCollectorAdapter) getMdaiCollectorConfig(logsConfig *mdaiv1.LogsConfig, awsAccessKeySecret *string) (string, error) {
	var mdaiCollectorConfig map[string]any
	if err := yaml.Unmarshal([]byte(baseMdaiCollectorYAML), &mdaiCollectorConfig); err != nil {
		c.logger.Error(err, "Failed to unmarshal base mdai collector config")
		return "", err
	}

	if logsConfig != nil {
		exporters := mdaiCollectorConfig["exporters"].(map[string]any)
		serviceBlock := mdaiCollectorConfig["service"].(map[string]any)
		pipelines := serviceBlock["pipelines"].(map[string]any)
		connectors := mdaiCollectorConfig["connectors"].(map[string]any)
		routingTableMap := make(map[mdaiv1.MDAILogStream]routingConnectorTableEntry)
		otherLogstreamPipelines := make([]string, 0)

		otlpOtherLogstreamPipelines := c.augmentConfigForOtlpConfigAndGetOtherLogstreamPipelines(logsConfig, exporters, pipelines, routingTableMap)
		otherLogstreamPipelines = append(otherLogstreamPipelines, otlpOtherLogstreamPipelines...)

		s3OtherLogstreamPipelines := c.augmentPipelinesForS3ConfigAndGetOtherLogstreamPipelines(logsConfig, awsAccessKeySecret, exporters, pipelines, routingTableMap)
		otherLogstreamPipelines = append(otherLogstreamPipelines, s3OtherLogstreamPipelines...)

		logstreamRouter := connectors["routing/logstream"].(map[string]any)
		newRoutingTable := make([]routingConnectorTableEntry, 0, len(routingTableMap))
		for _, entry := range routingTableMap {
			newRoutingTable = append(newRoutingTable, entry)
		}
		logstreamRouter["table"] = newRoutingTable
		if len(otherLogstreamPipelines) > 0 {
			logstreamRouter["default_pipelines"] = otherLogstreamPipelines
		}
	}

	collectorConfigBytes, err := yaml.Marshal(mdaiCollectorConfig)
	if err != nil {
		c.logger.Error(err, "Failed to marshal mdai-collector config", "mdaiCollectorConfig", mdaiCollectorConfig)
		return "", err
	}
	collectorConfig := string(collectorConfigBytes)

	return collectorConfig, nil
}

func (c MdaiCollectorAdapter) augmentPipelinesForS3ConfigAndGetOtherLogstreamPipelines(
	logsConfig *mdaiv1.LogsConfig,
	awsAccessKeySecret *string,
	exporters map[string]any,
	pipelines map[string]any,
	routingTableMap map[mdaiv1.MDAILogStream]routingConnectorTableEntry,
) []string {
	defaultRoutingPipelines := make([]string, 0)
	if awsAccessKeySecret != nil {
		s3Config := logsConfig.S3
		if s3Config != nil {
			c.logger.Info(fmt.Sprintf("Adding S3 components to mdai-collector config for %s", c.collectorCR.Name))
			auditLogstreamConfig := s3Config.AuditLogs
			if auditLogstreamConfig == nil || !auditLogstreamConfig.Disabled {
				logstream := mdaiv1.AuditLogstream
				c.addS3ComponentsAndPipeline(logstream, s3Config, exporters, nil, pipelines, routingTableMap)
			}

			collectorLogstreamConfig := s3Config.CollectorLogs
			if collectorLogstreamConfig == nil || !collectorLogstreamConfig.Disabled {
				logstream := mdaiv1.CollectorLogstream
				s3ExporterName, s3Exporter := getS3ExporterForLogstream(c.collectorCR.Name, logstream, *s3Config)
				exporters[s3ExporterName] = s3Exporter
				var minSeverity *mdaiv1.SeverityLevel
				if collectorLogstreamConfig != nil && collectorLogstreamConfig.MinSeverity != nil {
					minSeverity = collectorLogstreamConfig.MinSeverity
				}
				c.addS3ComponentsAndPipeline(logstream, s3Config, exporters, minSeverity, pipelines, routingTableMap)
			}

			hubLogstreamConfig := s3Config.HubLogs
			if hubLogstreamConfig == nil || !hubLogstreamConfig.Disabled {
				logstream := mdaiv1.HubLogstream
				var minSeverity *mdaiv1.SeverityLevel
				if hubLogstreamConfig != nil && hubLogstreamConfig.MinSeverity != nil {
					minSeverity = hubLogstreamConfig.MinSeverity
				}
				c.addS3ComponentsAndPipeline(logstream, s3Config, exporters, minSeverity, pipelines, routingTableMap)
			}

			otherLogstreamConfig := s3Config.OtherLogs
			if otherLogstreamConfig == nil || !otherLogstreamConfig.Disabled {
				logstream := mdaiv1.OtherLogstream
				var minSeverity *mdaiv1.SeverityLevel
				if otherLogstreamConfig != nil && otherLogstreamConfig.MinSeverity != nil {
					minSeverity = otherLogstreamConfig.MinSeverity
				}
				pipelineName := c.addS3ComponentsAndPipeline(logstream, s3Config, exporters, minSeverity, pipelines, routingTableMap)
				defaultRoutingPipelines = append(defaultRoutingPipelines, pipelineName)
			}
		}
	} else {
		c.logger.Info("Skipped adding s3 components to mdai-collector due to missing s3 configuration", "logsConfig", logsConfig, "awsAccessKeySecret", awsAccessKeySecret)
	}
	return defaultRoutingPipelines
}

func (c MdaiCollectorAdapter) augmentConfigForOtlpConfigAndGetOtherLogstreamPipelines(
	logsConfig *mdaiv1.LogsConfig,
	exporters map[string]any,
	pipelines map[string]any,
	routingTableMap map[mdaiv1.MDAILogStream]routingConnectorTableEntry,
) []string {
	defaultRoutingPipelines := make([]string, 0)
	otlpConfig := logsConfig.Otlp
	if otlpConfig != nil && otlpConfig.Endpoint != "" {
		c.logger.Info(fmt.Sprintf("Adding OTLP components to mdai-collector config for %s", c.collectorCR.Name))
		otlpExporterName, otlpExporterConfig := getOtlpExporterForLogstream(*otlpConfig)
		exporters[otlpExporterName] = otlpExporterConfig

		auditLogstreamConfig := otlpConfig.AuditLogs
		if auditLogstreamConfig == nil || !auditLogstreamConfig.Disabled {
			logstream := mdaiv1.AuditLogstream
			pipelineName := fmt.Sprintf("logs/otlp_%s", logstream)
			newPipeline := c.getPipelineWithExporterAndSeverityFilter("routing/logstream", otlpExporterName, nil)
			pipelines[pipelineName] = newPipeline
			c.addOrCreateRoutingTableEntryWithPipeline(logstream, &routingTableMap, pipelineName)
		}

		collectorLogstreamConfig := otlpConfig.CollectorLogs
		if collectorLogstreamConfig == nil || !collectorLogstreamConfig.Disabled {
			logstream := mdaiv1.CollectorLogstream
			c.addPipelineForLogstream(collectorLogstreamConfig, logstream, otlpExporterName, pipelines, &routingTableMap)
		}

		hubLogstreamConfig := otlpConfig.HubLogs
		if hubLogstreamConfig == nil || !hubLogstreamConfig.Disabled {
			logstream := mdaiv1.HubLogstream
			c.addPipelineForLogstream(hubLogstreamConfig, logstream, otlpExporterName, pipelines, &routingTableMap)
		}

		otherLogstreamConfig := otlpConfig.OtherLogs
		if otherLogstreamConfig == nil || !otherLogstreamConfig.Disabled {
			logstream := mdaiv1.OtherLogstream
			pipelineName := c.addPipelineForLogstream(otherLogstreamConfig, logstream, otlpExporterName, pipelines, &routingTableMap)
			defaultRoutingPipelines = append(defaultRoutingPipelines, pipelineName)
		}
	}
	return defaultRoutingPipelines
}

func (c MdaiCollectorAdapter) addS3ComponentsAndPipeline(
	logstream mdaiv1.MDAILogStream,
	s3Config *mdaiv1.S3LogsConfig,
	exporters map[string]any,
	minSeverity *mdaiv1.SeverityLevel,
	pipelines map[string]any,
	routingTableMap map[mdaiv1.MDAILogStream]routingConnectorTableEntry,
) string {
	s3ExporterName, s3Exporter := getS3ExporterForLogstream(c.collectorCR.Name, logstream, *s3Config)
	exporters[s3ExporterName] = s3Exporter
	pipelineName := fmt.Sprintf("logs/s3_%s", logstream)
	newPipeline := c.getPipelineWithExporterAndSeverityFilter("routing/logstream", s3ExporterName, minSeverity)
	pipelines[pipelineName] = newPipeline
	c.addOrCreateRoutingTableEntryWithPipeline(logstream, &routingTableMap, pipelineName)
	return pipelineName
}

func (c MdaiCollectorAdapter) addPipelineForLogstream(
	logstreamConfig *mdaiv1.LogstreamConfig,
	logstream mdaiv1.MDAILogStream,
	otlpExporterName string,
	pipelines map[string]any,
	routingTableMap *map[mdaiv1.MDAILogStream]routingConnectorTableEntry,
) string {
	var minSeverity *mdaiv1.SeverityLevel
	if logstreamConfig != nil && logstreamConfig.MinSeverity != nil {
		minSeverity = logstreamConfig.MinSeverity
	}
	pipelineName := fmt.Sprintf("logs/otlp_%s", logstream)
	newPipeline := c.getPipelineWithExporterAndSeverityFilter("routing/logstream", otlpExporterName, minSeverity)
	pipelines[pipelineName] = newPipeline
	c.addOrCreateRoutingTableEntryWithPipeline(logstream, routingTableMap, pipelineName)
	return pipelineName
}

func (c MdaiCollectorAdapter) addOrCreateRoutingTableEntryWithPipeline(
	logstream mdaiv1.MDAILogStream,
	routingTableMap *map[mdaiv1.MDAILogStream]routingConnectorTableEntry,
	pipelineName string,
) {
	if logstream != mdaiv1.OtherLogstream {
		if _, ok := (*routingTableMap)[logstream]; !ok {
			(*routingTableMap)[logstream] = routingConnectorTableEntry{
				Context:   routingTableContextMap[logstream],
				Condition: routingTableConditionMap[logstream],
				Pipelines: []string{
					pipelineName,
				},
			}
		} else {
			routingEntry := (*routingTableMap)[logstream]
			routingEntry.Pipelines = append((*routingTableMap)[logstream].Pipelines, pipelineName)
			(*routingTableMap)[logstream] = routingEntry
		}
	}
}

func getOtlpExporterForLogstream(otlpLogsConfig mdaiv1.OtlpLogsConfig) (string, otlpExporterConfig) {
	exporter := otlpExporterConfig{
		Endpoint: otlpLogsConfig.Endpoint,
		TlsConfig: otlpTlsConfig{
			Insecure: true,
		},
	}
	return "otlp", exporter
}

func getS3ExporterForLogstream(hubName string, logstream mdaiv1.MDAILogStream, s3LogsConfig mdaiv1.S3LogsConfig) (string, s3ExporterConfig) {
	s3Prefix := fmt.Sprintf("%s-%s-logs", hubName, logstream)
	exporterKey := fmt.Sprintf("awss3/%s", logstream)
	filePrefix := fmt.Sprintf("%s-", logstream)
	exporter := s3ExporterConfig{
		S3Uploader: s3UploaderConfig{
			Region:            s3LogsConfig.S3Region,
			S3Bucket:          s3LogsConfig.S3Bucket,
			S3Prefix:          s3Prefix,
			FilePrefix:        filePrefix,
			S3PartitionFormat: S3PartitionFormat,
			DisableSSL:        true,
		},
	}
	return exporterKey, exporter
}

func (c MdaiCollectorAdapter) getPipelineWithExporterAndSeverityFilter(receiverName string, exporterName string, minSeverity *mdaiv1.SeverityLevel) map[string]any {
	receivers := []any{receiverName}
	exporters := make([]any, 0)
	processors := make([]any, 0)
	if minSeverity != nil {
		severityFilter := severityFilterMap[*minSeverity]
		if severityFilter != "" {
			processors = append(processors, severityFilter)
		}
	}
	newPipeline := map[string]any{
		"receivers":  receivers,
		"processors": processors,
		"exporters":  append(exporters, exporterName),
	}
	return newPipeline
}

// ensureHubDeletionProcessed deletes MDAI Collector in cases a deletion was triggered
func (c MdaiCollectorAdapter) ensureMdaiCollectorDeletionProcessed(ctx context.Context) (OperationResult, error) {
	if !c.collectorCR.DeletionTimestamp.IsZero() {
		c.logger.Info("Deleting Cluster:" + c.collectorCR.Name)
		crState, err := c.finalizeMdaiCollector(ctx)
		if crState == ObjectUnchanged || err != nil {
			c.logger.Info("Has to requeue mdai")
			return RequeueAfter(5*time.Second, err)
		}
		return StopProcessing()
	}
	return ContinueProcessing()
}

func (c MdaiCollectorAdapter) ensureMdaiCollectorStatusSetToDone(ctx context.Context) (OperationResult, error) {
	// Re-fetch the Custom Resource after update or create
	if err := c.client.Get(ctx, types.NamespacedName{Name: c.collectorCR.Name, Namespace: c.collectorCR.Namespace}, c.collectorCR); err != nil {
		c.logger.Error(err, "Failed to re-fetch MDAI Collector")
		return Requeue()
	}
	meta.SetStatusCondition(&c.collectorCR.Status.Conditions, metav1.Condition{
		Type:   typeAvailableHub,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "reconciled successfully",
	})
	if err := c.client.Status().Update(ctx, c.collectorCR); err != nil {
		if apierrors.ReasonForError(err) == metav1.StatusReasonConflict {
			c.logger.Info("re-queuing due to resource conflict")
			return Requeue()
		}
		c.logger.Error(err, "Failed to update MDAI Collector status")
		return Requeue()
	}
	c.logger.Info("Status set to done for MDAI Collector", "mdaiHub", c.collectorCR.Name)
	return ContinueProcessing()
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorConfigMap(ctx context.Context, namespace string, logsConfig *mdaiv1.LogsConfig, awsAccessKeySecret *string) (string, string, error) {
	mdaiCollectorConfigConfigMapName := c.getScopedMdaiCollectorResourceName("config")
	collectorYAML, err := c.getMdaiCollectorConfig(logsConfig, awsAccessKeySecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to build mdai-collector configuration: %w", err)
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorConfigConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.collectorCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
		Data: map[string]string{
			"collector.yaml": collectorYAML,
		},
	}
	if err := controllerutil.SetControllerReference(c.collectorCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on "+mdaiCollectorConfigConfigMapName+" ConfigMap", "configmap", mdaiCollectorConfigConfigMapName)
		return "", "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data["collector.yaml"] = collectorYAML
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update "+mdaiCollectorConfigConfigMapName+" ConfigMap", "configmap", mdaiCollectorConfigConfigMapName)
		return "", "", err
	}

	c.logger.Info(mdaiCollectorConfigConfigMapName+" ConfigMap created or updated successfully", "configmap", mdaiCollectorConfigConfigMapName, "operation", operationResult)
	sha, err := getConfigMapSHA(*desiredConfigMap)
	return mdaiCollectorConfigConfigMapName, sha, err
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorEnvVarConfigMap(ctx context.Context, namespace string) (string, error) {
	mdaiCollectorEnvVarConfigMapName := c.getScopedMdaiCollectorResourceName("env")
	data := map[string]string{
		"LOG_SEVERITY":  "SEVERITY_NUMBER_WARN",
		"K8S_NAMESPACE": namespace,
	}

	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorEnvVarConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          "mdai-collector",
				hubNameLabel:                   c.collectorCR.Name,
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
		Data: data,
	}
	if err := controllerutil.SetControllerReference(c.collectorCR, desiredConfigMap, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on "+mdaiCollectorEnvVarConfigMapName+" ConfigMap", "configmap", mdaiCollectorEnvVarConfigMapName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, desiredConfigMap, func() error {
		desiredConfigMap.Data = data
		return nil
	})
	if err != nil {
		c.logger.Error(err, "Failed to create or update "+mdaiCollectorEnvVarConfigMapName+" ConfigMap", "configmap", mdaiCollectorEnvVarConfigMapName)
		return "", err
	}

	c.logger.Info(mdaiCollectorEnvVarConfigMapName+" ConfigMap created or updated successfully", "configmap", mdaiCollectorEnvVarConfigMapName, "operation", operationResult)
	return mdaiCollectorEnvVarConfigMapName, nil
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorDeployment(ctx context.Context, namespace string, collectorConfigMapName string, collectorEnvConfigMapName string, serviceAccountName string, awsAccessKeySecret *string, hash string) (string, error) {
	mdaiCollectorDeploymentName := c.getScopedMdaiCollectorResourceName("")
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, deployment, func() error {
		if err := controllerutil.SetControllerReference(c.collectorCR, deployment, c.scheme); err != nil {
			c.logger.Error(err, "Failed to set owner reference on "+mdaiCollectorDeploymentName+" Deployment", "deployment", deployment.Name)
			return err
		}

		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels["app"] = mdaiCollectorDeploymentName
		deployment.Labels[hubNameLabel] = c.collectorCR.Name

		deployment.Spec.Replicas = int32Ptr(1)
		if deployment.Spec.Selector == nil {
			deployment.Spec.Selector = &metav1.LabelSelector{}
		}
		if deployment.Spec.Selector.MatchLabels == nil {
			deployment.Spec.Selector.MatchLabels = make(map[string]string)
		}
		deployment.Spec.Selector.MatchLabels["app"] = mdaiCollectorDeploymentName

		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		deployment.Spec.Template.Labels["app"] = mdaiCollectorDeploymentName
		deployment.Spec.Template.Labels["app.kubernetes.io/component"] = mdaiCollectorDeploymentName

		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations["mdai-collector-config/sha256"] = hash

		deployment.Spec.Template.Spec.ServiceAccountName = serviceAccountName

		containerSpec := corev1.Container{
			Name:  mdaiCollectorDeploymentName,
			Image: "public.ecr.aws/decisiveai/mdai-collector:0.1",
			Command: []string{
				"/mdai-collector",
				"--config=/conf/collector.yaml",
			},
			Ports: []corev1.ContainerPort{
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
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: collectorEnvConfigMapName,
						},
					},
				},
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
		deployment.Spec.Template.Spec.Containers = []corev1.Container{containerSpec}

		if awsAccessKeySecret != nil {
			secretEnvSource := corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: *awsAccessKeySecret,
					},
				},
			}
			containerSpec.EnvFrom = append(containerSpec.EnvFrom, secretEnvSource)
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
							Name: collectorConfigMapName,
						},
					},
				},
			},
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	c.logger.Info(mdaiCollectorDeploymentName+" Deployment created or updated successfully", "deployment", deployment.Name, "operationResult", operationResult)

	return mdaiCollectorDeploymentName, nil
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorService(ctx context.Context, namespace string, appName string) (string, error) {
	mdaiCollectorServiceName := c.getScopedMdaiCollectorResourceName("service")
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorServiceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.collectorCR, service, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on "+mdaiCollectorServiceName+" Service", "service", mdaiCollectorServiceName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, service, func() error {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		service.Labels["app"] = appName
		service.Labels[hubNameLabel] = c.collectorCR.Name

		service.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appName,
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
		return "", fmt.Errorf("failed to create or update mdai-collector-service: %w", err)
	}

	c.logger.Info("Successfully created or updated"+mdaiCollectorServiceName+" Service", "service", mdaiCollectorServiceName, "namespace", namespace, "operation", operationResult)
	return mdaiCollectorServiceName, nil
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorServiceAccount(ctx context.Context, namespace string) (string, error) {
	mdaiCollectorServiceAccountName := c.getScopedMdaiCollectorResourceName("sa")
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdaiCollectorServiceAccountName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	if err := controllerutil.SetControllerReference(c.collectorCR, serviceAccount, c.scheme); err != nil {
		c.logger.Error(err, "Failed to set owner reference on "+mdaiCollectorServiceAccountName+" ServiceAccount", "serviceAccount", mdaiCollectorServiceAccountName)
		return "", err
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, serviceAccount, func() error {
		return nil
	})

	if err != nil {
		return "", err
	}
	c.logger.Info(mdaiCollectorServiceAccountName+" ServiceAccount created or updated successfully", "serviceAccount", serviceAccount.Name, "operationResult", operationResult)

	return mdaiCollectorServiceAccountName, nil
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorRole(ctx context.Context) (string, error) {
	mdaiCollectorClusterRoleName := c.getScopedMdaiCollectorResourceName("role")
	// BEWARE: If you're thinking about making a yaml and unmarshaling it, to extract all this out... there's a weird problem where apiGroups won't unmarshal from yaml.
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: mdaiCollectorClusterRoleName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{
					"events",
					"namespaces",
					"namespaces/status",
					"nodes",
					"nodes/spec",
					"pods",
					"pods/status",
					"replicationcontrollers",
					"replicationcontrollers/status",
					"resourcequotas",
					"services",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{
					"daemonsets",
					"deployments",
					"replicasets",
					"statefulsets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"extensions"},
				Resources: []string{
					"daemonsets",
					"deployments",
					"replicasets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{
					"jobs",
					"cronjobs",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
			{
				APIGroups: []string{"autoscaling"},
				Resources: []string{
					"horizontalpodautoscalers",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
			},
		}

		return nil
	})

	if err != nil {
		return "", err
	}
	c.logger.Info(mdaiCollectorClusterRoleName+" ClusterRole created or updated successfully", "role", role.Name, "operationResult", operationResult)

	return mdaiCollectorClusterRoleName, nil
}

func (c MdaiCollectorAdapter) createOrUpdateMdaiCollectorRoleBinding(ctx context.Context, namespace string, roleName string, serviceAccountName string) error {
	mdaiCollectorClusterRoleBindingName := c.getScopedMdaiCollectorResourceName("rb")
	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: mdaiCollectorClusterRoleBindingName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "mdai-operator",
				HubComponentLabel:              MdaiCollectorHubComponent,
			},
		},
	}

	operationResult, err := controllerutil.CreateOrUpdate(ctx, c.client, roleBinding, func() error {
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		}
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		}
		return nil
	})

	if err != nil {
		return err
	}
	c.logger.Info(mdaiCollectorClusterRoleBindingName+"RoleBinding created or updated successfully", "roleBinding", roleBinding.Name, "operationResult", operationResult)

	return nil
}
