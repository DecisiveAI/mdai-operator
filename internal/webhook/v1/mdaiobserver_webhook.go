package v1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-viper/mapstructure/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// nolint:unused
// log is for logging in this package.
var mdaiobserverlog = logf.Log.WithName("mdaiobserver-resource")

// SetupMdaiObserverWebhookWithManager registers the webhook for MdaiObserver in the manager.
func SetupMdaiObserverWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&mdaiv1.MdaiObserver{}).
		WithValidator(&MdaiObserverCustomValidator{}).
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
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &MdaiObserverCustomValidator{}

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
func (v *MdaiObserverCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	mdaiobserver, ok := newObj.(*mdaiv1.MdaiObserver)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiObserver object for the newObj but got %T", newObj)
	}
	mdaiobserverlog.Info("Validation for MdaiObserver upon update", "name", mdaiobserver.GetName())

	return v.validateObserversAndObserverResources(mdaiobserver)
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
				// TODO: refactor
				if observer.Provider == mdaiv1.OTEL_COLLECTOR {
					return newWarnings, ParseSpanMetricsConfig(&observer)
				} else {
					return newWarnings, nil
				}
			}
		case mdaiv1.DATA_VOLUME:
			{
				if observer.DataVolumeObserver.BytesMetricName == nil && observer.DataVolumeObserver.CountMetricName == nil {
					return newWarnings, fmt.Errorf("observer %s must have either a bytesMetricName or countMetricName", observer.Name)
				}
				if len(observer.DataVolumeObserver.LabelResourceAttributes) == 0 {
					newWarnings = append(newWarnings, "observer "+observer.Name+" does not define any labels to apply to counts")
				}
				if observer.SpanMetricsObserver != nil {
					return newWarnings, fmt.Errorf("observer %s of type %s can not have spanMetricsConnectorConfig section", observer.Name, observer.Type)
				}
			}
		}
	}
	return newWarnings, nil
}

func ParseSpanMetricsConfig(observer *mdaiv1.Observer) error {
	var configJsonData map[string]any
	if err := json.Unmarshal(observer.SpanMetricsObserver.ConnectorConfig.Raw, &configJsonData); err != nil {
		return fmt.Errorf("can not marshall observer %s SpanMetricsConnectorConfig to json", observer.Name)
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
		return fmt.Errorf("failed to build mapstructure decoder for %s", observer.Name)
	}

	if err := dec.Decode(configJsonData); err != nil {
		if len(md.Unused) > 0 {
			return fmt.Errorf("unknown spanMetricsConnectorConfig fields: %v: %w", md.Unused, err)
		}
		return fmt.Errorf("invalid spanMetricsConnectorConfig: %w", err)
	}

	if err := spanmetricsConfig.Validate(); err != nil {
		return fmt.Errorf("validate SpanMetricsConnectorConfig for observer %s failed, Error: %s", observer.Name, err.Error())
	}

	return nil
}
