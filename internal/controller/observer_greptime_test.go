package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePipeline(t *testing.T) {
	t.Parallel()

	p := Pipeline{
		Name:         "traffic",
		Schema:       "public",
		SourceTable:  "opentelemetry_traces",
		SinkTable:    "golden_signals_traffic",
		FlowName:     "golden_signals_traffic_flow",
		SinkTemplate: "sink_traffic",
		FlowTemplate: "flow_traffic",
		ValueColumn:  "total_count",
		ValueType:    "INT64",
		ValueExpr:    "COUNT(service_name)",
		PrimaryKeys:  []string{"service_name"},
		TimeInterval: "5 seconds",
		WhereClause:  "span_status_code = 'STATUS_CODE_UNSET'",
	}
	d := TemplateData{
		Schema:         "public",
		SourceTable:    "opentelemetry_traces",
		SinkTable:      "golden_signals_traffic",
		FlowName:       "golden_signals_traffic_flow",
		Dimensions:     []string{"service_name", "resource_attributes.host.name"},
		PrimaryKeys:    []string{"service_name"},
		ValueColumn:    "total_count",
		ValueType:      "INT64",
		ValueExpr:      "COUNT(service_name)",
		TimeSelectExpr: buildTimeSelectExpr("5 seconds"),
		TimeGroupExpr:  buildTimeGroupExpr("5 seconds"),
		WhereClause:    "span_status_code = 'STATUS_CODE_UNSET'",
	}

	require.NoError(t, validatePipeline(p, d))

	p.TimeInterval = "5 seconds!"
	err := validatePipeline(p, d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid time interval")
}

func TestExistingDimensionColumns(t *testing.T) {
	t.Parallel()

	d := TemplateData{
		Dimensions:  []string{"service_name", "region"},
		PrimaryKeys: []string{"service_name"},
		ValueColumn: "total_count",
	}

	existingCols := []string{
		"service_name",
		"region",
		"total_count",
		"time_window",
		"update_at",
	}

	dims := existingDimensionColumns(existingCols, d)
	assert.ElementsMatch(t, []string{"service_name", "region"}, dims)
}

func TestSinkTableNeedsRecreate(t *testing.T) {
	t.Parallel()

	d := TemplateData{
		Dimensions:  []string{"service_name", "region"},
		PrimaryKeys: []string{"service_name"},
		ValueColumn: "total_count",
	}

	assert.False(t, sinkTableNeedsRecreate([]string{
		"service_name",
		"region",
		"total_count",
		"time_window",
		"update_at",
	}, d))

	assert.True(t, sinkTableNeedsRecreate([]string{
		"service_name",
		"total_count",
		"time_window",
		"update_at",
	}, d))
}

func TestBuildTimeExprs(t *testing.T) {
	t.Parallel()

	group := buildTimeGroupExpr("5 seconds")
	selectExpr := buildTimeSelectExpr("5 seconds")

	assert.Equal(t, "date_bin('5 seconds'::INTERVAL, timestamp)", group)
	assert.Equal(t, "date_bin('5 seconds'::INTERVAL, timestamp) AS time_window", selectExpr)
}

func TestRenderSQLTemplate(t *testing.T) {
	t.Parallel()

	templates := loadTemplates()
	d := TemplateData{
		Schema:         "public",
		SourceTable:    "opentelemetry_traces",
		SinkTable:      "golden_signals_traffic",
		FlowName:       "golden_signals_traffic_flow",
		Dimensions:     []string{"service_name", "region"},
		PrimaryKeys:    []string{"service_name"},
		ValueColumn:    "total_count",
		ValueType:      "INT64",
		ValueExpr:      "COUNT(service_name)",
		TimeSelectExpr: buildTimeSelectExpr("5 seconds"),
		TimeGroupExpr:  buildTimeGroupExpr("5 seconds"),
		WhereClause:    "span_status_code = 'STATUS_CODE_UNSET'",
	}

	sinkDDL, err := renderSQLTemplate(templates, "sink_traffic", d)
	require.NoError(t, err)
	assert.True(t, strings.Contains(sinkDDL, "CREATE TABLE IF NOT EXISTS"))
	assert.True(t, strings.Contains(sinkDDL, `"service_name"`))
	assert.True(t, strings.Contains(sinkDDL, `"region"`))

	flowDDL, err := renderSQLTemplate(templates, "flow_traffic", d)
	require.NoError(t, err)
	assert.True(t, strings.Contains(flowDDL, "CREATE OR REPLACE FLOW"))
	assert.True(t, strings.Contains(flowDDL, "GROUP BY"))
	assert.True(t, strings.Contains(flowDDL, `"service_name"`))
}
