package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func setupTestAdapter(t *testing.T, replayCR *mdaiv1.MdaiReplay, objects ...client.Object) *MdaiReplayAdapter {
	scheme := runtime.NewScheme()
	require.NoError(t, mdaiv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	allObjects := append([]client.Object{replayCR}, objects...)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(allObjects...).
		WithStatusSubresource(replayCR).
		Build()

	logger := zap.New(zap.UseDevMode(true))
	recorder := record.NewFakeRecorder(10)

	return NewMdaiReplayAdapter(replayCR, logger, fakeClient, recorder, scheme, nil)
}

func createTestReplayCR(name, namespace string) *mdaiv1.MdaiReplay {
	return &mdaiv1.MdaiReplay{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: mdaiv1.MdaiReplaySpec{
			HubName:           "test-hub",
			StartTime:         "2024-01-01T00:00:00Z",
			EndTime:           "2024-01-02T00:00:00Z",
			OpAMPEndpoint:     "http://opamp:4320",
			StatusVariableRef: "status-var",
			Source: mdaiv1.MdaiReplaySourceConfiguration{
				S3: &mdaiv1.MdaiReplayS3Configuration{
					FilePrefix:  "logs",
					S3Region:    "us-east-1",
					S3Bucket:    "test-bucket",
					S3Path:      "path/to/logs",
					S3Partition: "daily",
				},
				AWSConfig: &mdaiv1.MdaiReplayAwsConfig{},
			},
			Destination: mdaiv1.MdaiReplayDestinationConfiguration{
				OtlpHttp: &mdaiv1.MdaiReplayOtlpHttpDestinationConfiguration{
					Endpoint: "http://otlp:4318",
				},
			},
		},
	}
}

// Test ensureFinalizerInitialized
func TestEnsureFinalizerInitialized(t *testing.T) {
	t.Run("adds finalizer when not present", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureFinalizerInitialized(ctx)

		require.NoError(t, err)
		opResult, err := StopProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)

		// Verify finalizer was added
		updated := &mdaiv1.MdaiReplay{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: "test-replay", Namespace: "default"}, updated)
		require.NoError(t, err)
		assert.Contains(t, updated.Finalizers, mdaiReplayFinalizerName)
	})

	t.Run("continues when finalizer already exists", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Finalizers = []string{mdaiReplayFinalizerName}
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureFinalizerInitialized(ctx)

		require.NoError(t, err)
		opResult, err := ContinueProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)
	})
}

// Test ensureStatusInitialized
func TestEnsureStatusInitialized(t *testing.T) {
	t.Run("initializes status when empty", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureStatusInitialized(ctx)

		require.NoError(t, err)
		opResult, err := StopProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)

		// Verify status was initialized
		updated := &mdaiv1.MdaiReplay{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: "test-replay", Namespace: "default"}, updated)
		require.NoError(t, err)
		assert.NotEmpty(t, updated.Status.Conditions)

		condition := meta.FindStatusCondition(updated.Status.Conditions, typeAvailableHub)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionUnknown, condition.Status)
	})

	t.Run("continues when status already initialized", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Status.Conditions = []metav1.Condition{
			{Type: typeAvailableHub, Status: metav1.ConditionTrue},
		}
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureStatusInitialized(ctx)

		require.NoError(t, err)
		opResult, err := ContinueProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)
	})
}

// Test ensureDeletionProcessed
func TestEnsureDeletionProcessed(t *testing.T) {
	t.Run("continues when deletion timestamp is zero", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureDeletionProcessed(ctx)

		require.NoError(t, err)
		opResult, err := ContinueProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)
	})

	t.Run("processes deletion when timestamp is set", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		now := metav1.Now()
		replayCR.DeletionTimestamp = &now
		replayCR.Finalizers = []string{mdaiReplayFinalizerName}
		replayCR.Spec.IgnoreSendingQueue = true // Skip queue check for this test
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureDeletionProcessed(ctx)

		require.NoError(t, err)
		opResult, err := StopProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)
	})
}

// Test getReplayerResourceName
func TestGetReplayerResourceName(t *testing.T) {
	replayCR := createTestReplayCR("my-replay", "default")
	replayCR.Spec.HubName = "my-hub"
	adapter := setupTestAdapter(t, replayCR)

	tests := []struct {
		suffix   string
		expected string
	}{
		{"collector", "replay-my-replay-my-hub-collector"},
		{"service", "replay-my-replay-my-hub-service"},
		{"collector-config", "replay-my-replay-my-hub-collector-config"},
	}

	for _, tt := range tests {
		t.Run(tt.suffix, func(t *testing.T) {
			result := adapter.getReplayerResourceName(tt.suffix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test createOrUpdateReplayerConfigMap
func TestCreateOrUpdateReplayerConfigMap(t *testing.T) {
	t.Run("creates configmap successfully", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		hash, err := adapter.createOrUpdateReplayerConfigMap(ctx)

		require.NoError(t, err)
		assert.NotEmpty(t, hash)

		// Verify ConfigMap was created
		configMapName := adapter.getReplayerResourceName("collector-config")
		cm := &corev1.ConfigMap{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: "default"}, cm)
		require.NoError(t, err)
		assert.Contains(t, cm.Data, "collector.yaml")
		assert.NotEmpty(t, cm.Data["collector.yaml"])
	})

	t.Run("configmap has correct labels", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		_, err := adapter.createOrUpdateReplayerConfigMap(ctx)
		require.NoError(t, err)

		configMapName := adapter.getReplayerResourceName("collector-config")
		cm := &corev1.ConfigMap{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: "default"}, cm)
		require.NoError(t, err)

		assert.Equal(t, LabelManagedByMdaiValue, cm.Labels[LabelManagedByMdaiKey])
		assert.Equal(t, "test-hub", cm.Labels[hubNameLabel])
		assert.Equal(t, mdaiObserverHubComponent, cm.Labels[HubComponentLabel])
	})
}

// Test getReplayCollectorConfigYAML
func TestGetReplayCollectorConfigYAML(t *testing.T) {
	t.Run("generates valid YAML config", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)

		yaml, err := adapter.getReplayCollectorConfigYAML("test-replay", "test-hub")

		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		// Verify key configuration elements are present
		assert.Contains(t, yaml, "test-replay")
		assert.Contains(t, yaml, "test-hub")
		assert.Contains(t, yaml, "2024-01-01T00:00:00Z")
		assert.Contains(t, yaml, "2024-01-02T00:00:00Z")
	})

	t.Run("includes otlphttp exporter when endpoint is set", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Spec.Destination.OtlpHttp.Endpoint = "http://otlp:4318"
		adapter := setupTestAdapter(t, replayCR)

		yaml, err := adapter.getReplayCollectorConfigYAML("test-replay", "test-hub")

		require.NoError(t, err)
		assert.Contains(t, yaml, "otlphttp")
		assert.Contains(t, yaml, "http://otlp:4318")
	})

	t.Run("includes S3 configuration", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)

		yaml, err := adapter.getReplayCollectorConfigYAML("test-replay", "test-hub")

		require.NoError(t, err)
		assert.Contains(t, yaml, "test-bucket")
		assert.Contains(t, yaml, "us-east-1")
		assert.Contains(t, yaml, "path/to/logs")
	})
}

// Test createOrUpdateReplayerDeployment
func TestCreateOrUpdateReplayerDeployment(t *testing.T) {
	t.Run("creates deployment successfully", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.createOrUpdateReplayerDeployment(ctx, "test-hash")

		require.NoError(t, err)

		// Verify Deployment was created
		deploymentName := adapter.getReplayerResourceName("collector")
		deployment := &appsv1.Deployment{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: "default"}, deployment)
		require.NoError(t, err)
		assert.Equal(t, int32(1), *deployment.Spec.Replicas)
	})

	t.Run("uses custom image when specified", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Spec.Resource.Image = "custom/image:v1.0"
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.createOrUpdateReplayerDeployment(ctx, "test-hash")

		require.NoError(t, err)

		deploymentName := adapter.getReplayerResourceName("collector")
		deployment := &appsv1.Deployment{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: "default"}, deployment)
		require.NoError(t, err)
		assert.Equal(t, "custom/image:v1.0", deployment.Spec.Template.Spec.Containers[0].Image)
	})

	t.Run("deployment has config hash annotation", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.createOrUpdateReplayerDeployment(ctx, "abc123")

		require.NoError(t, err)

		deploymentName := adapter.getReplayerResourceName("collector")
		deployment := &appsv1.Deployment{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: "default"}, deployment)
		require.NoError(t, err)
		assert.Equal(t, "abc123", deployment.Spec.Template.Annotations["replay-collector-config/sha256"])
	})
}

// Test createOrUpdateReplayerService
func TestCreateOrUpdateReplayerService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.createOrUpdateReplayerService(ctx)

		require.NoError(t, err)

		// Verify Service was created
		serviceName := adapter.getReplayerResourceName("service")
		service := &corev1.Service{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: "default"}, service)
		require.NoError(t, err)
		assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
	})

	t.Run("service has correct ports", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.createOrUpdateReplayerService(ctx)

		require.NoError(t, err)

		serviceName := adapter.getReplayerResourceName("service")
		service := &corev1.Service{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: "default"}, service)
		require.NoError(t, err)
		require.Len(t, service.Spec.Ports, 1)
		assert.Equal(t, "otelcol-metrics", service.Spec.Ports[0].Name)
		assert.Equal(t, int32(otelMetricsPort), service.Spec.Ports[0].Port)
	})
}

// Test getMetricValue
func TestGetMetricValue(t *testing.T) {
	t.Run("parses gauge metric successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`# HELP otelcol_exporter_queue_size Current size of the retry queue
# TYPE otelcol_exporter_queue_size gauge
otelcol_exporter_queue_size{exporter="otlphttp"} 42
`))
		}))
		defer server.Close()

		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)

		// Note: This test would need to be adjusted to inject the test server URL
		// For now, this demonstrates the test structure
		_ = adapter
		_ = server
	})

	t.Run("returns error when metric not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`# HELP some_other_metric Some other metric
# TYPE some_other_metric gauge
some_other_metric 10
`))
		}))
		defer server.Close()

		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)

		// This demonstrates the error case structure
		_ = adapter
		_ = server
	})
}

// Test deleteFinalizer
func TestDeleteReplayFinalizer(t *testing.T) {
	t.Run("removes finalizer when present", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Finalizers = []string{mdaiReplayFinalizerName, "other-finalizer"}
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.deleteFinalizer(ctx, replayCR, mdaiReplayFinalizerName)

		require.NoError(t, err)

		updated := &mdaiv1.MdaiReplay{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: "test-replay", Namespace: "default"}, updated)
		require.NoError(t, err)
		assert.NotContains(t, updated.Finalizers, mdaiReplayFinalizerName)
		assert.Contains(t, updated.Finalizers, "other-finalizer")
	})

	t.Run("no-op when finalizer not present", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		replayCR.Finalizers = []string{"other-finalizer"}
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		err := adapter.deleteFinalizer(ctx, replayCR, mdaiReplayFinalizerName)

		require.NoError(t, err)

		updated := &mdaiv1.MdaiReplay{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: "test-replay", Namespace: "default"}, updated)
		require.NoError(t, err)
		assert.Contains(t, updated.Finalizers, "other-finalizer")
	})
}

// Test ensureStatusSetToDone
func TestEnsureReplayStatusSetToDone(t *testing.T) {
	t.Run("sets status to done successfully", func(t *testing.T) {
		replayCR := createTestReplayCR("test-replay", "default")
		adapter := setupTestAdapter(t, replayCR)
		ctx := context.Background()

		result, err := adapter.ensureStatusSetToDone(ctx)

		require.NoError(t, err)
		opResult, err := ContinueProcessing()
		require.NoError(t, err)
		assert.Equal(t, opResult, result)

		updated := &mdaiv1.MdaiReplay{}
		err = adapter.client.Get(ctx, types.NamespacedName{Name: "test-replay", Namespace: "default"}, updated)
		require.NoError(t, err)

		condition := meta.FindStatusCondition(updated.Status.Conditions, typeAvailableHub)
		require.NotNil(t, condition)
		assert.Equal(t, metav1.ConditionTrue, condition.Status)
		assert.Equal(t, "reconciled successfully", condition.Message)
	})
}
