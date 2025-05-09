package controller

import (
	"fmt"
	"testing"

	v1 "github.com/decisiveai/mdai-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestGetS3ExporterForLogstream(t *testing.T) {
	testCases := []struct {
		hubName                string
		logstream              MDAILogStream
		s3LogsConfig           v1.S3LogsConfig
		expectedExporterName   string
		expectedExporterConfig S3ExporterConfig
	}{
		{
			hubName:   "test-hub",
			logstream: CollectorLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: ptr.To("uesc-marathon-7"),
				S3Bucket: ptr.To("whoa-bucket"),
			},
			expectedExporterName: "awss3/collector",
			expectedExporterConfig: S3ExporterConfig{
				S3Uploader: S3UploaderConfig{
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
			logstream: HubLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: ptr.To("aeiou-meh-99"),
				S3Bucket: ptr.To("qwerty"),
			},
			expectedExporterName: "awss3/hub",
			expectedExporterConfig: S3ExporterConfig{
				S3Uploader: S3UploaderConfig{
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
			logstream: AuditLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: ptr.To("splat"),
				S3Bucket: ptr.To("hey"),
			},
			expectedExporterName: "awss3/audit",
			expectedExporterConfig: S3ExporterConfig{
				S3Uploader: S3UploaderConfig{
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
			logstream: OtherLogstream,
			s3LogsConfig: v1.S3LogsConfig{
				S3Region: ptr.To("okay"),
				S3Bucket: ptr.To("ytho"),
			},
			expectedExporterName: "awss3/other",
			expectedExporterConfig: S3ExporterConfig{
				S3Uploader: S3UploaderConfig{
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
	testCases := []struct {
		pipeline     map[string]any
		exporterName string
		expected     map[string]any
	}{
		{
			pipeline: map[string]any{
				"receivers":  []any{"otlp"},
				"processors": []any{"batch"},
				"exporters":  []any{"debug"},
			},
			exporterName: "awss3/collector",
			expected: map[string]any{
				"receivers":  []any{"otlp"},
				"processors": []any{"batch"},
				"exporters":  []any{"debug", "awss3/collector"},
			},
		},
		{
			pipeline: map[string]any{
				"receivers":  []any{"otlp", "qwer", "iouoip/ioeuwr"},
				"processors": []any{"batch", "lsikdjflks", "klsjdlfjslr", "ewroije"},
				"exporters":  []any{"debug", "slkdjflskdrn/selirjselkr", "zmcsdfkjls/slkdr/skjdlrjl"},
			},
			exporterName: "awss3/hub",
			expected: map[string]any{
				"receivers":  []any{"otlp", "qwer", "iouoip/ioeuwr"},
				"processors": []any{"batch", "lsikdjflks", "klsjdlfjslr", "ewroije"},
				"exporters":  []any{"debug", "slkdjflskdrn/selirjselkr", "zmcsdfkjls/slkdr/skjdlrjl", "awss3/hub"},
			},
		},
	}

	for idx, testCase := range testCases {
		t.Run(fmt.Sprintf("Case %d %s", idx, testCase.exporterName), func(t *testing.T) {
			assert.Equal(t, testCase.expected, getPipelineWithS3Exporter(testCase.pipeline, testCase.exporterName))
		})
	}
}
