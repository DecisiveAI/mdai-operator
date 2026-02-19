package controller

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type Greptime struct {
	greptimeDb        gorm.DB
	greptimeTemplates map[string]string
}

type Pipeline struct {
	Name         string
	Schema       string
	SourceTable  string
	SinkTable    string
	FlowName     string
	SinkTemplate string
	FlowTemplate string
	ValueColumn  string
	ValueType    string
	ValueExpr    string
	PrimaryKeys  []string
	TimeInterval string
	WhereClause  string
}

type TemplateData struct {
	Schema         string
	SourceTable    string
	SinkTable      string
	FlowName       string
	Dimensions     []string
	PrimaryKeys    []string
	ValueColumn    string
	ValueType      string
	ValueExpr      string
	TimeSelectExpr string
	TimeGroupExpr  string
	WhereClause    string
}

var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var columnPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var intervalPattern = regexp.MustCompile(`^[0-9]+\s+(millisecond|milliseconds|second|seconds|minute|minutes|hour|hours)$`)

func doGreptime(greptime Greptime, dimensions []string, primaryKey string) error {
	db := greptime.greptimeDb
	templates := greptime.greptimeTemplates

	pipelines := []Pipeline{
		{
			Name:         "traffic",
			Schema:       "public",
			SourceTable:  "opentelemetry_traces",
			SinkTable:    "golden_signals_traffic",
			FlowName:     "golden_signals_traffic_flow",
			SinkTemplate: "sink_traffic",
			FlowTemplate: "flow_traffic",
			ValueColumn:  "total_count",
			ValueType:    "INT64",
			ValueExpr:    fmt.Sprintf("COUNT(%s)", primaryKey),
			PrimaryKeys:  []string{primaryKey},
			// TODO: parametrize
			TimeInterval: "5 seconds",
			WhereClause:  "span_status_code = 'STATUS_CODE_UNSET'",
		},
		{
			Name:         "latency",
			Schema:       "public",
			SourceTable:  "opentelemetry_traces",
			SinkTable:    "golden_signals_duration_sketch_5s",
			FlowName:     "golden_signals_duration_sketch_5s_flow",
			SinkTemplate: "sink_latency",
			FlowTemplate: "flow_latency",
			ValueColumn:  "latency_sketch",
			ValueType:    "BINARY",
			ValueExpr:    "uddsketch_state(128, 0.01, duration_nano)",
			PrimaryKeys:  []string{primaryKey},
			// TODO: parametrize
			TimeInterval: "5 seconds",
			WhereClause:  "span_status_code = 'STATUS_CODE_UNSET'",
		},
		{
			Name:         "errors",
			Schema:       "public",
			SourceTable:  "opentelemetry_traces",
			SinkTable:    "golden_signals_errors",
			FlowName:     "golden_signals_errors_flow",
			SinkTemplate: "sink_errors",
			FlowTemplate: "flow_errors",
			ValueColumn:  "total_count",
			ValueType:    "INT64",
			ValueExpr:    fmt.Sprintf("COUNT(%s)", primaryKey),
			PrimaryKeys:  []string{primaryKey},
			// TODO: parametrize
			TimeInterval: "5 seconds",
			WhereClause:  "span_status_code = 'STATUS_CODE_ERROR'",
		},
	}

	for _, p := range pipelines {
		data := TemplateData{
			Schema:         p.Schema,
			SourceTable:    p.SourceTable,
			SinkTable:      p.SinkTable,
			FlowName:       p.FlowName,
			Dimensions:     dimensions,
			PrimaryKeys:    p.PrimaryKeys,
			ValueColumn:    p.ValueColumn,
			ValueType:      p.ValueType,
			ValueExpr:      p.ValueExpr,
			TimeSelectExpr: buildTimeSelectExpr(p.TimeInterval),
			TimeGroupExpr:  buildTimeGroupExpr(p.TimeInterval),
			WhereClause:    p.WhereClause,
		}

		if err := validatePipeline(p, data); err != nil {
			return fmt.Errorf("pipeline %s: %w", p.Name, err)
		}

		sinkDDL, err := renderSQLTemplate(templates, p.SinkTemplate, data)
		if err != nil {
			return fmt.Errorf("pipeline %s sink template: %w", p.Name, err)
		}
		if err := db.Exec(sinkDDL).Error; err != nil {
			return fmt.Errorf("pipeline %s create sink: %w", p.Name, err)
		}

		if err := reconcileDimensionColumns(&db, data); err != nil {
			return fmt.Errorf("pipeline %s reconcile dimensions: %w", p.Name, err)
		}

		flowDDL, err := renderSQLTemplate(templates, p.FlowTemplate, data)
		if err != nil {
			return fmt.Errorf("pipeline %s flow template: %w", p.Name, err)
		}
		if err := db.Exec(flowDDL).Error; err != nil {
			return fmt.Errorf("pipeline %s create/replace flow: %w", p.Name, err)
		}
	}
	return nil
}

func reconcileDimensionColumns(db *gorm.DB, d TemplateData) error {
	existingCols, err := listTableColumns(db, d.Schema, d.SinkTable)
	if err != nil {
		return fmt.Errorf("list table columns: %w", err)
	}

	desired := make(map[string]struct{}, len(d.Dimensions))
	for _, dim := range d.Dimensions {
		desired[dim] = struct{}{}
	}

	primaryKeys := make(map[string]struct{}, len(d.PrimaryKeys))
	for _, c := range d.PrimaryKeys {
		primaryKeys[c] = struct{}{}
	}

	managedNonDimension := map[string]struct{}{
		d.ValueColumn: {},
		"time_window": {},
		"update_at":   {},
	}

	existingDimensions := make(map[string]struct{})
	for _, c := range existingCols {
		if _, ok := managedNonDimension[c]; ok {
			continue
		}
		//if _, ok := primaryKeys[c]; ok {
		//	continue
		//}
		existingDimensions[c] = struct{}{}
	}

	var toAdd []string
	for c := range desired {
		if _, ok := existingDimensions[c]; !ok {
			toAdd = append(toAdd, c)
		}
	}

	var toDrop []string
	for c := range existingDimensions {
		if _, ok := desired[c]; !ok {
			toDrop = append(toDrop, c)
		}
	}

	sort.Strings(toAdd)
	sort.Strings(toDrop)

	for _, c := range toAdd {
		addStmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s STRING", quoteIdentifier(d.SinkTable), quoteIdentifier(c))
		if err := db.Exec(addStmt).Error; err != nil {
			return fmt.Errorf("add column %s: %w", c, err)
		}
		indexStmt := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s SET INVERTED INDEX", quoteIdentifier(d.SinkTable), quoteIdentifier(c))
		if err := db.Exec(indexStmt).Error; err != nil {
			return fmt.Errorf("set inverted index for column %s: %w", c, err)
		}
	}

	for _, c := range toDrop {
		unsetStmt := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s UNSET INVERTED INDEX", quoteIdentifier(d.SinkTable), quoteIdentifier(c))
		if err := db.Exec(unsetStmt).Error; err != nil {
			return fmt.Errorf("unset inverted index for column %s: %w", c, err)
		}
		dropStmt := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdentifier(d.SinkTable), quoteIdentifier(c))
		if err := db.Exec(dropStmt).Error; err != nil {
			return fmt.Errorf("drop column %s: %w", c, err)
		}
	}

	return nil
}

func listTableColumns(db *gorm.DB, schema, table string) ([]string, error) {
	var columns []string
	err := db.Raw(
		`SELECT column_name
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?`,
		schema, table,
	).Scan(&columns).Error
	if err != nil {
		return nil, err
	}
	return columns, nil
}

func validatePipeline(p Pipeline, d TemplateData) error {
	for _, id := range []struct {
		name  string
		label string
	}{
		{name: p.Name, label: "pipeline name"},
		{name: d.Schema, label: "schema"},
		{name: d.SourceTable, label: "source table"},
		{name: d.SinkTable, label: "sink table"},
		{name: d.FlowName, label: "flow name"},
		{name: d.ValueColumn, label: "value column"},
	} {
		if !identPattern.MatchString(id.name) {
			return fmt.Errorf("invalid %s identifier: %s", id.label, id.name)
		}
	}

	//if len(d.Dimensions) == 0 {
	//	return fmt.Errorf("at least one dimension is required")
	//}
	if len(d.PrimaryKeys) == 0 {
		return fmt.Errorf("at least one primary key column is required")
	}

	for _, dim := range d.Dimensions {
		if !columnPattern.MatchString(dim) {
			return fmt.Errorf("invalid dimension name: %s", dim)
		}
	}
	for _, pk := range d.PrimaryKeys {
		if !columnPattern.MatchString(pk) {
			return fmt.Errorf("invalid primary key name: %s", pk)
		}
	}

	for _, field := range []struct {
		value string
		label string
	}{
		{value: p.SinkTemplate, label: "sink template"},
		{value: p.FlowTemplate, label: "flow template"},
		{value: p.TimeInterval, label: "time interval"},
		{value: d.ValueType, label: "value type"},
		{value: d.ValueExpr, label: "value expression"},
		{value: d.TimeSelectExpr, label: "time select expression"},
		{value: d.TimeGroupExpr, label: "time group expression"},
		{value: d.WhereClause, label: "where clause"},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("empty %s", field.label)
		}
	}
	if !intervalPattern.MatchString(p.TimeInterval) {
		return fmt.Errorf("invalid time interval: %s", p.TimeInterval)
	}

	return nil
}

func buildTimeGroupExpr(interval string) string {
	return fmt.Sprintf("date_bin('%s'::INTERVAL, timestamp)", interval)
}

func buildTimeSelectExpr(interval string) string {
	return fmt.Sprintf("%s AS time_window", buildTimeGroupExpr(interval))
}

func loadTemplates() map[string]string {
	return map[string]string{
		"sink_traffic": sinkTrafficTemplate,
		"flow_traffic": flowTrafficTemplate,
		"sink_latency": sinkLatencyTemplate,
		"flow_latency": flowLatencyTemplate,
		"sink_errors":  sinkErrorsTemplate,
		"flow_errors":  flowErrorsTemplate,
	}
}

func renderSQLTemplate(templates map[string]string, templateName string, data TemplateData) (string, error) {
	templateText, ok := templates[templateName]
	if !ok {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	tmpl, err := template.New(templateName).Funcs(template.FuncMap{
		"quoteIdentifier": quoteIdentifier,
		"dimensionProjection": func(col string) string {
			q := quoteIdentifier(col)
			return fmt.Sprintf("%s AS %s", q, q)
		},
		"joinDimensionNames": func(dimensions []string) string {
			names := make([]string, 0, len(dimensions))
			for _, d := range dimensions {
				names = append(names, quoteIdentifier(d))
			}
			return strings.Join(names, ", ")
		},
		"joinPrimaryKeys": func(keys []string) string {
			names := make([]string, 0, len(keys))
			for _, k := range keys {
				names = append(names, quoteIdentifier(k))
			}
			return strings.Join(names, ", ")
		},
	}).Parse(templateText)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return html.UnescapeString(out.String()), nil
}

const sinkTrafficTemplate = `
CREATE TABLE IF NOT EXISTS {{ .SinkTable }} (
{{- range .Dimensions }}
  {{ quoteIdentifier . }} STRING INVERTED INDEX,
{{- end }}
  {{ .ValueColumn }} {{ .ValueType }},
  time_window TIMESTAMP TIME INDEX,
  update_at TIMESTAMP,
  PRIMARY KEY ({{ joinPrimaryKeys .PrimaryKeys }})
);`

const flowTrafficTemplate = `
CREATE OR REPLACE FLOW {{ .FlowName }}
SINK TO {{ .SinkTable }}
AS
SELECT
{{- range .Dimensions }}
  {{ dimensionProjection . }},
{{- end }}
  {{ .ValueExpr }} AS {{ .ValueColumn }},
  {{ .TimeSelectExpr }}
FROM {{ .SourceTable }}
WHERE {{ .WhereClause }}
GROUP BY
{{- range .Dimensions }}
  {{ quoteIdentifier . }},
{{- end }}
  time_window;`

const sinkLatencyTemplate = `
CREATE TABLE IF NOT EXISTS {{ .SinkTable }} (
{{- range .Dimensions }}
  {{ quoteIdentifier . }} STRING INVERTED INDEX,
{{- end }}
  {{ .ValueColumn }} {{ .ValueType }},
  time_window TIMESTAMP TIME INDEX,
  update_at TIMESTAMP,
  PRIMARY KEY ({{ joinPrimaryKeys .PrimaryKeys }})
);`

const flowLatencyTemplate = `
CREATE OR REPLACE FLOW {{ .FlowName }}
SINK TO {{ .SinkTable }}
AS
SELECT
{{- range .Dimensions }}
  {{ dimensionProjection . }},
{{- end }}
  {{ .ValueExpr }} AS {{ .ValueColumn }},
  {{ .TimeSelectExpr }}
FROM {{ .SourceTable }}
WHERE {{ .WhereClause }}
GROUP BY
{{- range .Dimensions }}
  {{ quoteIdentifier . }},
{{- end }}
  time_window;`

const sinkErrorsTemplate = `
CREATE TABLE IF NOT EXISTS {{ .SinkTable }} (
{{- range .Dimensions }}
  {{ quoteIdentifier . }} STRING INVERTED INDEX,
{{- end }}
  {{ .ValueColumn }} {{ .ValueType }},
  time_window TIMESTAMP TIME INDEX,
  update_at TIMESTAMP,
  PRIMARY KEY ({{ joinPrimaryKeys .PrimaryKeys }})
);`

const flowErrorsTemplate = `
CREATE OR REPLACE FLOW {{ .FlowName }}
SINK TO {{ .SinkTable }}
AS
SELECT
{{- range .Dimensions }}
  {{ dimensionProjection . }},
{{- end }}
  {{ .ValueExpr }} AS {{ .ValueColumn }},
  {{ .TimeSelectExpr }}
FROM {{ .SourceTable }}
WHERE {{ .WhereClause }}
GROUP BY
{{- range .Dimensions }}
  {{ quoteIdentifier . }},
{{- end }}
  time_window;`

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
