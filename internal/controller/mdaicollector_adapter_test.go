package controller

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1core "k8s.io/api/core/v1"
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
