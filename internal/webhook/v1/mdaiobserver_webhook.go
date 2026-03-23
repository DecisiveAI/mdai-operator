package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/decisiveai/mdai-operator/internal/greptimedb"
	"github.com/go-viper/mapstructure/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"gorm.io/gorm"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// nolint:unused
// log is for logging in this package.
var mdaiobserverlog = logf.Log.WithName("mdaiobserver-resource")

var sinkTableTTLPattern = regexp.MustCompile(`^(?:\d+\s*(?:nsec|ns|usec|us|msec|ms|seconds|second|sec|s|minutes|minute|min|m|hours|hour|hr|h|days|day|d|weeks|week|w|months|month|M|years|year|y)\s*)+$`)
var flowAggregateIntervalPattern = regexp.MustCompile(`^\d+\s+(?:nsec|ns|usec|us|msec|ms|seconds|second|sec|s|minutes|minute|min|m|hours|hour|hr|h|days|day|d|weeks|week|w|months|month|M|years|year|y)$`)
var greptimeObserverSinkTables = []string{
	"golden_signals_traffic",
	"golden_signals_duration_sketch_5s",
	"golden_signals_errors",
}

// SetupMdaiObserverWebhookWithManager registers the webhook for MdaiObserver in the manager.
func SetupMdaiObserverWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&mdaiv1.MdaiObserver{}).
		WithValidator(NewMdaiObserverCustomValidator()).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-hub-mydecisive-ai-v1-mdaiobserver,mutating=false,failurePolicy=fail,sideEffects=None,groups=hub.mydecisive.ai,resources=mdaiobservers,verbs=create;update,versions=v1,name=vmdaiobserver-v1.kb.io,admissionReviewVersions=v1

// MdaiObserverCustomValidator struct is responsible for validating the MdaiObserver resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MdaiObserverCustomValidator struct {
	greptimeInspector        greptimeInspector
	greptimeInspectorFactory func() (greptimeInspector, error)
}

var _ webhook.CustomValidator = &MdaiObserverCustomValidator{}

func NewMdaiObserverCustomValidator() *MdaiObserverCustomValidator {
	return &MdaiObserverCustomValidator{
		greptimeInspectorFactory: func() (greptimeInspector, error) {
			db, err := greptimedb.OpenFromEnv(mdaiobserverlog)
			if err != nil {
				return nil, err
			}
			return &gormGreptimeInspector{db: db}, nil
		},
	}
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type MdaiObserver.
func (v *MdaiObserverCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaiobserver, ok := obj.(*mdaiv1.MdaiObserver)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiObserver object but got %T", obj)
	}
	mdaiobserverlog.Info("Validation for MdaiObserver upon creation", "name", mdaiobserver.GetName())

	return v.validateObserversAndObserverResources(mdaiobserver)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type MdaiObserver.
func (v *MdaiObserverCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	mdaiobserver, ok := newObj.(*mdaiv1.MdaiObserver)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiObserver object for the newObj but got %T", newObj)
	}
	oldMdaiobserver, ok := oldObj.(*mdaiv1.MdaiObserver)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiObserver object for the oldObj but got %T", oldObj)
	}
	mdaiobserverlog.Info("Validation for MdaiObserver upon update", "name", mdaiobserver.GetName())

	warnings, err := v.validateObserversAndObserverResources(mdaiobserver)
	if err != nil {
		return warnings, err
	}
	return warnings, v.validateGreptimeRecreatePolicy(oldMdaiobserver, mdaiobserver)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type MdaiObserver.
func (*MdaiObserverCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaiobserver, ok := obj.(*mdaiv1.MdaiObserver)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiObserver object but got %T", obj)
	}
	mdaiobserverlog.Info("Validation for MdaiObserver upon deletion", "name", mdaiobserver.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func (*MdaiObserverCustomValidator) validateObserversAndObserverResources(mdaiobserver *mdaiv1.MdaiObserver) (admission.Warnings, error) {
	newWarnings := admission.Warnings{}
	observers := mdaiobserver.Spec.Observers
	observerSpec := mdaiobserver.Spec

	if observerSpec.ObserverResource.Resources == nil {
		newWarnings = append(newWarnings, "ObserverResource "+mdaiobserver.Name+" does not define resource requests/limits")
	}

	if len(observers) == 0 {
		return append(newWarnings, "Observers are not specified"), nil
	}
	for _, observer := range observers {

		switch observer.Type {
		case mdaiv1.SPAN_METRICS:
			{
				if observer.SpanMetricsObserver == nil {
					return newWarnings, fmt.Errorf("observer %s of type %s must define spanMetricsObserver", observer.Name, observer.Type)
				}
				switch observer.Provider {
				case mdaiv1.OTEL_COLLECTOR:
					if observer.SpanMetricsObserver.Otel == nil {
						return newWarnings, fmt.Errorf("observer %s of provider %s must define spanMetricsObserver.otel", observer.Name, observer.Provider)
					}
					if observer.SpanMetricsObserver.Greptime != nil {
						return newWarnings, fmt.Errorf("observer %s of provider %s must not define spanMetricsObserver.greptime", observer.Name, observer.Provider)
					}
					config, err := ParseSpanMetricsConfig(&observer)
					if err != nil {
						return newWarnings, err
					}
					if config != nil && len(observer.SpanMetricsObserver.Otel.GroupByAttrs) > 0 {
						used := make(map[string]struct{}, len(config.Dimensions))
						for _, dim := range config.Dimensions {
							used[dim.Name] = struct{}{}
						}
						for _, attr := range observer.SpanMetricsObserver.Otel.GroupByAttrs {
							if _, ok := used[attr]; ok {
								return newWarnings, fmt.Errorf("observer %s has duplicate attribute %q in groupByAttrs and connectorConfig.dimensions", observer.Name, attr)
							}
						}
					}
				case mdaiv1.GREPTIME_FLOW:
					if observer.SpanMetricsObserver.Greptime == nil {
						return newWarnings, fmt.Errorf("observer %s of provider %s must define spanMetricsObserver.greptime", observer.Name, observer.Provider)
					}
					if observer.SpanMetricsObserver.Otel != nil {
						return newWarnings, fmt.Errorf("observer %s of provider %s must not define spanMetricsObserver.otel", observer.Name, observer.Provider)
					}
					if len(observer.SpanMetricsObserver.Greptime.Dimensions) == 0 || observer.SpanMetricsObserver.Greptime.PrimaryKey == "" {
						return newWarnings, fmt.Errorf("observer %s of provider %s must define greptime dimensions and primaryKey", observer.Name, observer.Provider)
					}
					if err := validateSinkTableTTL(observer.SpanMetricsObserver.Greptime.SinkTableTtl); err != nil {
						return newWarnings, fmt.Errorf("observer %s has invalid sinkTableTtl: %w", observer.Name, err)
					}
					if err := validateFlowAggregateInterval(observer.SpanMetricsObserver.Greptime.FlowAggregateInterval); err != nil {
						return newWarnings, fmt.Errorf("observer %s has invalid flowAggregateInterval: %w", observer.Name, err)
					}
				default:
					return newWarnings, fmt.Errorf("observer %s has unsupported provider %s for type %s", observer.Name, observer.Provider, observer.Type)
				}
			}
		case mdaiv1.DATA_VOLUME:
			{
				if observer.DataVolumeObserver == nil {
					return newWarnings, fmt.Errorf("observer %s of type %s must define dataVolumeObserver", observer.Name, observer.Type)
				}
				if observer.DataVolumeObserver.BytesMetricName == nil && observer.DataVolumeObserver.CountMetricName == nil {
					return newWarnings, fmt.Errorf("observer %s must have either a bytesMetricName or countMetricName", observer.Name)
				}
				if len(observer.DataVolumeObserver.LabelResourceAttributes) == 0 {
					newWarnings = append(newWarnings, "observer "+observer.Name+" does not define any labels to apply to counts")
				}
				if observer.SpanMetricsObserver != nil {
					return newWarnings, fmt.Errorf("observer %s of type %s can not have spanMetricsObserver section", observer.Name, observer.Type)
				}
			}
		}
	}
	return newWarnings, nil
}

func (v *MdaiObserverCustomValidator) validateGreptimeRecreatePolicy(oldObj, newObj *mdaiv1.MdaiObserver) error {
	oldObservers := greptimeObserversByName(oldObj.Spec.Observers)
	newObservers := greptimeObserversByName(newObj.Spec.Observers)

	for name, newObserver := range newObservers {
		if newObserver.SpanMetricsObserver == nil || newObserver.SpanMetricsObserver.Greptime == nil {
			continue
		}
		if newObserver.SpanMetricsObserver.Greptime.AllowSinkTableRecreate {
			continue
		}

		oldObserver, ok := oldObservers[name]
		if !ok {
			continue
		}
		if !greptimeObserverRequiresSinkRecreate(oldObserver, newObserver) {
			continue
		}

		inspector, err := v.getGreptimeInspector()
		if err != nil {
			return fmt.Errorf("initialize GreptimeDB inspector: %w", err)
		}

		exists, err := greptimeSinkTablesExist(inspector)
		if err != nil {
			return fmt.Errorf("inspect GreptimeDB sink tables for observer %s: %w", name, err)
		}
		if !exists {
			continue
		}

		mdaiobserverlog.Info("WARN: rejecting MdaiObserver update because sink table recreation is disabled", "observer", name)
		return fmt.Errorf("observer %s update requires Greptime sink table recreation, but allowSinkTableRecreate is false", name)
	}

	return nil
}

func (v *MdaiObserverCustomValidator) getGreptimeInspector() (greptimeInspector, error) {
	if v.greptimeInspector != nil {
		return v.greptimeInspector, nil
	}
	if v.greptimeInspectorFactory == nil {
		return nil, fmt.Errorf("greptime inspector is not configured")
	}

	inspector, err := v.greptimeInspectorFactory()
	if err != nil {
		return nil, err
	}
	v.greptimeInspector = inspector
	return inspector, nil
}

func greptimeObserversByName(observers []mdaiv1.Observer) map[string]mdaiv1.Observer {
	result := make(map[string]mdaiv1.Observer)
	for _, observer := range observers {
		if observer.Provider != mdaiv1.GREPTIME_FLOW || observer.Type != mdaiv1.SPAN_METRICS {
			continue
		}
		result[observer.Name] = observer
	}
	return result
}

func greptimeObserverRequiresSinkRecreate(oldObserver, newObserver mdaiv1.Observer) bool {
	oldGreptime := oldObserver.SpanMetricsObserver.Greptime
	newGreptime := newObserver.SpanMetricsObserver.Greptime
	if oldGreptime == nil || newGreptime == nil {
		return false
	}
	if !sameStringSet(oldGreptime.Dimensions, newGreptime.Dimensions) {
		return true
	}
	return oldGreptime.PrimaryKey != newGreptime.PrimaryKey
}

func greptimeSinkTablesExist(inspector greptimeInspector) (bool, error) {
	for _, table := range greptimeObserverSinkTables {
		exists, err := inspector.TableExists("public", table)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

type greptimeInspector interface {
	TableExists(schema, table string) (bool, error)
}

type gormGreptimeInspector struct {
	db *gorm.DB
}

func (g *gormGreptimeInspector) TableExists(schema, table string) (bool, error) {
	var count int64
	err := g.db.Raw(
		`SELECT COUNT(*)
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?`,
		schema, table,
	).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func validateSinkTableTTL(ttl string) error {
	ttl = strings.TrimSpace(ttl)
	if ttl == "" {
		return nil
	}
	if sinkTableTTLPattern.MatchString(ttl) {
		return nil
	}
	return fmt.Errorf("must be a sequence of <number><unit> values using GreptimeDB TTL units")
}

func validateFlowAggregateInterval(interval string) error {
	interval = strings.TrimSpace(interval)
	if interval == "" {
		return nil
	}
	if flowAggregateIntervalPattern.MatchString(interval) {
		return nil
	}
	return fmt.Errorf("must be a single <number> <unit> value using GreptimeDB interval units")
}

func ParseSpanMetricsConfig(observer *mdaiv1.Observer) (*spanmetricsconnector.Config, error) {
	var configJsonData map[string]any
	if observer.SpanMetricsObserver == nil || observer.SpanMetricsObserver.Otel == nil || observer.SpanMetricsObserver.Otel.ConnectorConfig == nil {
		return nil, nil
	}
	if err := json.Unmarshal(observer.SpanMetricsObserver.Otel.ConnectorConfig.Raw, &configJsonData); err != nil {
		return nil, fmt.Errorf("can not marshall observer %s SpanMetricsConnectorConfig to json", observer.Name)
	}

	var spanmetricsConfig spanmetricsconnector.Config

	var md mapstructure.Metadata

	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           &spanmetricsConfig,
		Metadata:         &md,
		ErrorUnused:      true,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build mapstructure decoder for %s", observer.Name)
	}

	if err := dec.Decode(configJsonData); err != nil {
		if len(md.Unused) > 0 {
			return nil, fmt.Errorf("unknown spanMetricsConnectorConfig fields: %v: %w", md.Unused, err)
		}
		return nil, fmt.Errorf("invalid spanMetricsConnectorConfig: %w", err)
	}

	if err := spanmetricsConfig.Validate(); err != nil {
		return nil, fmt.Errorf("validate SpanMetricsConnectorConfig for observer %s failed, Error: %s", observer.Name, err.Error())
	}

	return &spanmetricsConfig, nil
}
