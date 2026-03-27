package controller

import (
	"strings"
	"testing"

	"github.com/decisiveai/mdai-operator/internal/greptimedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePipeline(t *testing.T) {
	t.Parallel()

	p := Pipeline{
		Name:         "traffic",
		Schema:       "public",
		SourceTable:  "opentelemetry_traces",
		SinkTable:    prefixedGreptimeObjectName("observer1", "golden_signals_traffic"),
		FlowName:     prefixedGreptimeObjectName("observer1", "golden_signals_traffic_flow"),
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
		SinkTable:      prefixedGreptimeObjectName("observer1", "golden_signals_traffic"),
		FlowName:       prefixedGreptimeObjectName("observer1", "golden_signals_traffic_flow"),
		Dimensions:     []string{"service_name", "resource_attributes.host.name"},
		PrimaryKeys:    []string{"service_name"},
		SinkTableTTL:   greptimeSinkTableTtl,
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
		Dimensions:   []string{"service_name", "region"},
		PrimaryKeys:  []string{"service_name"},
		SinkTableTTL: greptimeSinkTableTtl,
		ValueColumn:  "total_count",
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
		Dimensions:   []string{"service_name", "region"},
		PrimaryKeys:  []string{"service_name"},
		SinkTableTTL: "7d",
		ValueColumn:  "total_count",
	}

	assert.False(t, sinkTableNeedsRecreate([]string{
		"service_name",
		"region",
		"total_count",
		"time_window",
		"update_at",
	}, []string{"service_name"}, d))

	assert.True(t, sinkTableNeedsRecreate([]string{
		"service_name",
		"total_count",
		"time_window",
		"update_at",
	}, []string{"service_name"}, d))

	assert.True(t, sinkTableNeedsRecreate([]string{
		"service_name",
		"region",
		"total_count",
		"time_window",
		"update_at",
	}, []string{"region"}, d))
}

func TestSinkTableTTLNeedsUpdate(t *testing.T) {
	t.Parallel()

	assert.False(t, sinkTableTTLNeedsUpdate("7d", "7d"))
	assert.True(t, sinkTableTTLNeedsUpdate("3months", "7d"))
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
		SinkTable:      prefixedGreptimeObjectName("observer1", "golden_signals_traffic"),
		FlowName:       prefixedGreptimeObjectName("observer1", "golden_signals_traffic_flow"),
		Dimensions:     []string{"service_name", "region"},
		PrimaryKeys:    []string{"service_name"},
		SinkTableTTL:   "7d",
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
	assert.True(t, strings.Contains(sinkDDL, `"observer1_golden_signals_traffic"`))
	assert.True(t, strings.Contains(sinkDDL, `"service_name"`))
	assert.True(t, strings.Contains(sinkDDL, `"region"`))
	assert.True(t, strings.Contains(sinkDDL, "WITH (ttl='7d')"))

	flowDDL, err := renderSQLTemplate(templates, "flow_traffic", d)
	require.NoError(t, err)
	assert.True(t, strings.Contains(flowDDL, "CREATE OR REPLACE FLOW"))
	assert.True(t, strings.Contains(flowDDL, `"observer1_golden_signals_traffic_flow"`))
	assert.True(t, strings.Contains(flowDDL, `SINK TO "observer1_golden_signals_traffic"`))
	assert.True(t, strings.Contains(flowDDL, "GROUP BY"))
	assert.True(t, strings.Contains(flowDDL, `"service_name"`))
}

func TestDoGreptimeUsesConfiguredDatabaseName(t *testing.T) {
	t.Setenv("GREPTIME_DATABASE", "mdai")

	schema := greptimedb.DatabaseNameFromEnv()
	assert.Equal(t, "mdai", schema)

	p := Pipeline{
		Name:         "traffic",
		Schema:       schema,
		SourceTable:  "opentelemetry_traces",
		SinkTable:    prefixedGreptimeObjectName("observer1", "golden_signals_traffic"),
		FlowName:     prefixedGreptimeObjectName("observer1", "golden_signals_traffic_flow"),
		SinkTemplate: "sink_traffic",
		FlowTemplate: "flow_traffic",
		ValueColumn:  "total_count",
		ValueType:    "INT64",
		ValueExpr:    "COUNT(service_name)",
		PrimaryKeys:  []string{"service_name"},
		TimeInterval: "5 seconds",
		SinkTableTTL: "7d",
		WhereClause:  "span_status_code = 'STATUS_CODE_UNSET'",
	}

	assert.Equal(t, "mdai", p.Schema)
}

func TestPrefixedGreptimeObjectName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "sample_golden_signals_traffic", prefixedGreptimeObjectName("sample", "golden_signals_traffic"))
	assert.Equal(t, "sample_golden_signals_duration_flow", prefixedGreptimeObjectName("sample", "golden_signals_duration_flow"))
}

func TestExistingSinkTableTTL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "7d", existingSinkTableTTL(`CREATE TABLE foo (...) WITH (ttl='7d');`))
	assert.Equal(t, "3months", existingSinkTableTTL(`create table foo (...) with(ttl = '3months');`))
	assert.Equal(t, "", existingSinkTableTTL(`CREATE TABLE foo (...);`))
}

func TestExistingPrimaryKeys(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"service_name"}, existingPrimaryKeys(`CREATE TABLE foo (...) PRIMARY KEY ("service_name");`))
	assert.Equal(t, []string{"service_name", "region"}, existingPrimaryKeys(`CREATE TABLE foo (...) PRIMARY KEY ("service_name", "region");`))
	assert.Nil(t, existingPrimaryKeys(`CREATE TABLE foo (...);`))
}

func TestExistingFlowAggregateInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "5 second", existingFlowAggregateInterval(`
CREATE FLOW test_flow
AS
SELECT date_bin('5 second'::INTERVAL, timestamp) AS time_window
FROM test;
`))
	assert.Equal(t, "1 hour", existingFlowAggregateInterval(`
create flow test_flow as
select date_bin('1 hour'::INTERVAL, timestamp) as time_window
from test;
`))
	assert.Equal(t, "", existingFlowAggregateInterval(`CREATE FLOW test_flow AS SELECT * FROM test;`))
}

func TestFlowNeedsUpdate(t *testing.T) {
	t.Parallel()

	assert.False(t, flowNeedsUpdate("5 second", buildTimeGroupExpr("5 second")))
	assert.True(t, flowNeedsUpdate("5 second", buildTimeGroupExpr("1 minute")))
	assert.True(t, flowNeedsUpdate("", buildTimeGroupExpr("5 second")))
}
