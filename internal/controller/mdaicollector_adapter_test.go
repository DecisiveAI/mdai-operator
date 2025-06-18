package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func newFakeClientForCollectorCR(cr *v1.MdaiCollector, scheme *runtime.Scheme) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()
}

func createTestSchemeForMdaiCollector() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = v1core.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = prometheusv1.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	return scheme
}

func TestGetS3ExporterForLogstream(t *testing.T) {
	testCases := []struct {
		hubName                string
		logstream              v1.MDAILogStream
		s3LogsConfig           v1.S3LogsConfig
		expectedExporterName   string
		expectedExporterConfig s3ExporterConfig
	}{
		{
			hubName:   "test-hub",
			logstream: v1.CollectorLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: "uesc-marathon-7",
				S3Bucket: "whoa-bucket",
			},
			expectedExporterName: "awss3/collector",
			expectedExporterConfig: s3ExporterConfig{
				S3Uploader: s3UploaderConfig{
					Region:            "uesc-marathon-7",
					S3Bucket:          "whoa-bucket",
					S3Prefix:          "test-hub-collector-logs",
					FilePrefix:        "collector-",
					S3PartitionFormat: S3PartitionFormat,
					DisableSSL:        true,
				},
			},
		}, {
			hubName:   "inf",
			logstream: v1.HubLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: "aeiou-meh-99",
				S3Bucket: "qwerty",
			},
			expectedExporterName: "awss3/hub",
			expectedExporterConfig: s3ExporterConfig{
				S3Uploader: s3UploaderConfig{
					Region:            "aeiou-meh-99",
					S3Bucket:          "qwerty",
					S3Prefix:          "inf-hub-logs",
					FilePrefix:        "hub-",
					S3PartitionFormat: S3PartitionFormat,
					DisableSSL:        true,
				},
			},
		}, {
			hubName:   "whoa",
			logstream: v1.AuditLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: "splat",
				S3Bucket: "hey",
			},
			expectedExporterName: "awss3/audit",
			expectedExporterConfig: s3ExporterConfig{
				S3Uploader: s3UploaderConfig{
					Region:            "splat",
					S3Bucket:          "hey",
					S3Prefix:          "whoa-audit-logs",
					FilePrefix:        "audit-",
					S3PartitionFormat: S3PartitionFormat,
					DisableSSL:        true,
				},
			},
		}, {
			hubName:   "heh",
			logstream: v1.OtherLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: "okay",
				S3Bucket: "ytho",
			},
			expectedExporterName: "awss3/other",
			expectedExporterConfig: s3ExporterConfig{
				S3Uploader: s3UploaderConfig{
					Region:            "okay",
					S3Bucket:          "ytho",
					S3Prefix:          "heh-other-logs",
					FilePrefix:        "other-",
					S3PartitionFormat: S3PartitionFormat,
					DisableSSL:        true,
				},
			},
		},
	}
	for idx, testCase := range testCases {
		t.Run(fmt.Sprintf("Case %d %s %s %s", idx, testCase.hubName, testCase.logstream, testCase.expectedExporterName), func(t *testing.T) {
			actualExporterName, actualExporterConfig := getS3ExporterForLogstream(testCase.hubName, testCase.logstream, testCase.s3LogsConfig)
			assert.Equal(t, testCase.expectedExporterName, actualExporterName)
			assert.Equal(t, testCase.expectedExporterConfig, actualExporterConfig)
		})
	}
}

func TestGetPipelineWithS3Exporter(t *testing.T) {
	cr := &v1.MdaiCollector{}
	recorder := record.NewFakeRecorder(10)
	scheme := createTestSchemeForMdaiCollector()
	adapter := NewMdaiCollectorAdapter(cr, logr.Discard(), newFakeClientForCollectorCR(cr, scheme), recorder, scheme)

	testCases := []struct {
		receiverName  string
		severityLevel v1.SeverityLevel
		exporterName  string
		expected      map[string]any
	}{
		{
			receiverName:  "routing/logstream",
			severityLevel: v1.WarnSeverityLevel,
			exporterName:  "awss3/collector",
			expected: map[string]any{
				"receivers":  []any{"routing/logstream"},
				"processors": []any{severityFilterMap[v1.WarnSeverityLevel]},
				"exporters":  []any{"awss3/collector"},
			},
		},
		{
			receiverName:  "foobaz",
			severityLevel: v1.InfoSeverityLevel,
			exporterName:  "awss3/hub",
			expected: map[string]any{
				"receivers":  []any{"foobaz"},
				"processors": []any{severityFilterMap[v1.InfoSeverityLevel]},
				"exporters":  []any{"awss3/hub"},
			},
		},
	}

	for idx, testCase := range testCases {
		t.Run(fmt.Sprintf("Case %d %s", idx, testCase.exporterName), func(t *testing.T) {
			assert.Equal(t, testCase.expected, adapter.getPipelineWithExporterAndSeverityFilter(testCase.receiverName, testCase.exporterName, ptr.To(testCase.severityLevel)))
		})
	}
}

func TestCreateOrUpdateMdaiCollectorRole(t *testing.T) {
	cr := &v1.MdaiCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hub",
			Namespace: "default",
		},
	}
	scheme := createTestSchemeForMdaiCollector()
	assert.NoError(t, rbacv1.AddToScheme(scheme))

	cl := newFakeClientForCollectorCR(cr, scheme)
	adapter := NewMdaiCollectorAdapter(cr, logr.Discard(), cl, record.NewFakeRecorder(10), scheme)

	expectedName := adapter.getScopedMdaiCollectorResourceName("role")

	t.Run(fmt.Sprintf("creates or updates ClusterRole %q", expectedName), func(t *testing.T) {
		name, err := adapter.createOrUpdateMdaiCollectorRole(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, expectedName, name)

		var role rbacv1.ClusterRole
		assert.NoError(t,
			cl.Get(context.Background(), client.ObjectKey{Name: name}, &role),
		)

		assert.Equal(t, expectedName, role.Name)
		assert.Equal(t, "mdai-operator", role.Labels["app.kubernetes.io/managed-by"])
		assert.Equal(t, MdaiCollectorHubComponent, role.Labels[HubComponentLabel])
		assert.Len(t, role.Rules, 5)

		for _, r := range role.Rules {
			assert.Contains(t, r.Verbs, "get")
			assert.Contains(t, r.Verbs, "list")
			assert.Contains(t, r.Verbs, "watch")
			assert.NotEmpty(t, r.Resources)
		}
	})
}

func TestCreateOrUpdateMdaiCollectorRoleBinding(t *testing.T) {
	cr := &v1.MdaiCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hub",
			Namespace: "default",
		},
	}
	scheme := createTestSchemeForMdaiCollector()
	assert.NoError(t, rbacv1.AddToScheme(scheme))

	cl := newFakeClientForCollectorCR(cr, scheme)
	adapter := NewMdaiCollectorAdapter(cr, logr.Discard(), cl, record.NewFakeRecorder(10), scheme)

	expectedName := adapter.getScopedMdaiCollectorResourceName("rb")

	namespace := "mdai"
	roleName := "mdai-role"
	saName := "mdai-sa"

	t.Run(fmt.Sprintf("creates/updates RoleBinding %q", expectedName), func(t *testing.T) {
		err := adapter.createOrUpdateMdaiCollectorRoleBinding(context.Background(), namespace, roleName, saName)
		assert.NoError(t, err)

		var rb rbacv1.ClusterRoleBinding
		assert.NoError(t, cl.Get(context.Background(), client.ObjectKey{Name: expectedName}, &rb))

		assert.Equal(t, expectedName, rb.Name)
		assert.Equal(t, map[string]string{
			"app.kubernetes.io/managed-by": "mdai-operator",
			HubComponentLabel:              MdaiCollectorHubComponent,
		}, rb.Labels)

		assert.Equal(t, "rbac.authorization.k8s.io", rb.RoleRef.APIGroup)
		assert.Equal(t, "ClusterRole", rb.RoleRef.Kind)
		assert.Equal(t, roleName, rb.RoleRef.Name)

		if assert.Len(t, rb.Subjects, 1) {
			subj := rb.Subjects[0]
			assert.Equal(t, "ServiceAccount", subj.Kind)
			assert.Equal(t, saName, subj.Name)
			assert.Equal(t, namespace, subj.Namespace)
		}
	})
}
