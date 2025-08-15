package v1

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/prometheus/prometheus/promql/parser"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// nolint:unused
// log is for logging in this package.
var mdaihublog = logf.Log.WithName("mdaihub-resource")

// SetupMdaiHubWebhookWithManager registers the webhook for MdaiHub in the manager.
func SetupMdaiHubWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&mdaiv1.MdaiHub{}).
		WithValidator(&MdaiHubCustomValidator{}).
		WithDefaulter(&MdaiHubCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-hub-mydecisive-ai-v1-mdaihub,mutating=true,failurePolicy=fail,sideEffects=None,groups=hub.mydecisive.ai,resources=mdaihubs,verbs=create;update,versions=v1,name=mmdaihub-v1.kb.io,admissionReviewVersions=v1

// MdaiHubCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind MdaiHub when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type MdaiHubCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &MdaiHubCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind MdaiHub.
func (*MdaiHubCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	mdaihub, ok := obj.(*mdaiv1.MdaiHub)

	if !ok {
		return fmt.Errorf("expected an MdaiHub object but got %T", obj)
	}
	mdaihublog.Info("Defaulting for MdaiHub", "name", mdaihub.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-hub-mydecisive-ai-v1-mdaihub,mutating=false,failurePolicy=fail,sideEffects=None,groups=hub.mydecisive.ai,resources=mdaihubs,verbs=create;update,versions=v1,name=vmdaihub-v1.kb.io,admissionReviewVersions=v1

// MdaiHubCustomValidator struct is responsible for validating the MdaiHub resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MdaiHubCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &MdaiHubCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (v *MdaiHubCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaihub, ok := obj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object but got %T", obj)
	}
	mdaihublog.Info("Validation for MdaiHub upon creation", "name", mdaihub.GetName())

	return v.Validate(mdaihub)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (v *MdaiHubCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newMdaihub, ok := newObj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object for the newObj but got %T", newObj)
	}
	oldMdaihub, ok := oldObj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object for the oldObj but got %T", oldMdaihub)
	}

	// validate that meta variables keep the same references. Meta variables are immutable.
	if !containsMetaVariables(oldMdaihub.Spec.Variables) || !containsMetaVariables(newMdaihub.Spec.Variables) {
		mdaihublog.Info("Validation for MdaiHub upon update (no meta vars)", "name", newMdaihub.GetName())
		return v.Validate(newMdaihub)
	}

	// Build map of old meta variable refs
	oldMetaRefs := make(map[string][]string)
	for _, oldVar := range oldMdaihub.Spec.Variables {
		if oldVar.Type == mdaiv1.VariableTypeMeta {
			oldMetaRefs[oldVar.Key] = oldVar.VariableRefs
		}
	}

	// Compare new meta variables to old
	for _, newVar := range newMdaihub.Spec.Variables {
		if newVar.Type != mdaiv1.VariableTypeMeta {
			continue
		}
		if oldRefs, found := oldMetaRefs[newVar.Key]; found {
			if !reflect.DeepEqual(oldRefs, newVar.VariableRefs) {
				return nil, errors.New("meta variable references must not change; delete and recreate the variable to update references")
			}
		}
	}

	mdaihublog.Info("Validation for MdaiHub upon update", "name", newMdaihub.GetName())
	return v.Validate(newMdaihub)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (*MdaiHubCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaihub, ok := obj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object but got %T", obj)
	}
	mdaihublog.Info("Validation for MdaiHub upon deletion", "name", mdaihub.GetName())

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (v *MdaiHubCustomValidator) Validate(mdaihub *mdaiv1.MdaiHub) (admission.Warnings, error) {
	warnings := admission.Warnings{}

	_, warnings, err := v.validateVariables(mdaihub, warnings)
	if err != nil {
		return warnings, err
	}

	evaluations := mdaihub.Spec.PrometheusAlert
	if len(evaluations) == 0 {
		warnings = append(warnings, "Evaluations are not specified")
	} else {
		for _, evaluation := range evaluations {
			if _, err := parser.ParseExpr(evaluation.Expr.StrVal); err != nil {
				return warnings, err
			}
		}
	}

	eventRefsMap := map[string]any{}
	if mdaihub.Spec.Automations != nil {
		for _, automation := range mdaihub.Spec.Automations {
			if _, exists := eventRefsMap[automation.EventRef]; exists {
				return warnings, fmt.Errorf("hub %s, automationevent reference %s is duplicated", mdaihub.GetName(), automation.EventRef)
			}
			eventRefsMap[automation.EventRef] = struct{}{}
		}
	}

	return warnings, nil
}

func (*MdaiHubCustomValidator) validateVariables(mdaihub *mdaiv1.MdaiHub, warnings admission.Warnings) (map[string]struct{}, admission.Warnings, error) {
	keys := make(map[string]struct{})
	exported := make(map[string]struct{})
	variables := mdaihub.Spec.Variables
	hubName := mdaihub.GetName()

	if len(variables) == 0 {
		warnings = append(warnings, "variables are not specified")
		return keys, warnings, nil
	}
	for _, variable := range variables {
		key := variable.Key

		if variable.StorageType != mdaiv1.VariableSourceTypeBuiltInValkey {
			return nil, warnings, fmt.Errorf("hub %s, variable %s: unsupported storage type %s", hubName, key, variable.StorageType)
		}
		if _, exists := keys[variable.Key]; exists {
			return nil, warnings, fmt.Errorf("hub %s, variable key %s is duplicated", hubName, key)
		}
		keys[key] = struct{}{}

		refs := variable.VariableRefs
		isMeta := variable.Type == mdaiv1.VariableTypeMeta
		isHashSet := variable.DataType == mdaiv1.MetaVariableDataTypeHashSet

		if isMeta && len(refs) == 0 {
			return nil, warnings, fmt.Errorf("hub %s, variable %s: no variable references provided for meta variable", hubName, key)
		}
		if !isMeta && len(refs) > 0 {
			return nil, warnings, fmt.Errorf("hub %s, variable %s: variable references are not supported for non-meta variables", hubName, key)
		}
		if isMeta && isHashSet && len(refs) != 2 {
			return nil, warnings, fmt.Errorf("hub %s, variable %s: variable references for Meta HashSet must have exactly 2 elements", hubName, key)
		}

		for _, with := range variable.SerializeAs {
			if _, exists := exported[with.Name]; exists {
				return nil, warnings, fmt.Errorf("hub %s, variable %s: exported variable name %s is duplicated", hubName, key, with.Name)
			}
			exported[with.Name] = struct{}{}

			transformers := with.Transformers
			switch variable.DataType {
			case mdaiv1.VariableDataTypeSet,
				mdaiv1.MetaVariableDataTypePriorityList:
				if len(transformers) == 0 {
					return nil, warnings, fmt.Errorf("hub %s, variable %s: at least one transformer must be provided, such as 'join'", hubName, key)
				}
			case mdaiv1.VariableDataTypeString,
				mdaiv1.VariableDataTypeFloat,
				mdaiv1.VariableDataTypeInt,
				mdaiv1.VariableDataTypeBoolean,
				mdaiv1.VariableDataTypeMap,
				mdaiv1.MetaVariableDataTypeHashSet:
				if len(transformers) > 0 {
					return nil, warnings, fmt.Errorf("hub %s, variable %s: transformers are not supported for variable type %s", hubName, key, variable.DataType)
				}
			default:
				return nil, warnings, fmt.Errorf("hub %s, variable %s: unsupported variable type", hubName, key)
			}
		}
	}
	return keys, warnings, nil
}

func containsMetaVariables(vars []mdaiv1.Variable) bool {
	for _, v := range vars {
		if v.Type == mdaiv1.VariableTypeMeta {
			return true
		}
	}
	return false
}
