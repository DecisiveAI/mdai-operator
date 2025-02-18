/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/DecisiveAI/mdai-operator/api/v1"
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

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

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
func (d *MdaiHubCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
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
	mdaihub, ok := newObj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object for the newObj but got %T", newObj)
	}
	mdaihublog.Info("Validation for MdaiHub upon update", "name", mdaihub.GetName())

	return v.Validate(mdaihub)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (v *MdaiHubCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaihub, ok := obj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object but got %T", obj)
	}
	mdaihublog.Info("Validation for MdaiHub upon deletion", "name", mdaihub.GetName())

	return v.Validate(mdaihub)
}

func (v *MdaiHubCustomValidator) Validate(mdaihub *mdaiv1.MdaiHub) (admission.Warnings, error) {
	warnings := admission.Warnings{}

	storageKeys := map[string]struct{}{}
	exportedVariableNames := map[string]struct{}{}
	variables := mdaihub.Spec.Variables
	if variables == nil {
		warnings = append(warnings, "Variables are not specified")
	} else {
		for _, variable := range *variables {
			// TODO validate change of storage type, change between types
			switch *variable.StorageType {
			case mdaiv1.VariableSourceTypeBultInValkey:
				if _, exists := storageKeys[variable.StorageKey]; exists {
					return warnings, fmt.Errorf("storage key %s is duplicated", variable.StorageKey)
				}
				storageKeys[variable.StorageKey] = struct{}{}

				for _, with := range variable.With {
					if _, exists := exportedVariableNames[with.ExportedVariableName]; exists {
						return warnings, fmt.Errorf("variable %s: exported variable name %s is duplicated", variable.StorageKey, with.ExportedVariableName)
					}
					exportedVariableNames[with.ExportedVariableName] = struct{}{}

					transformer := with.Transformer
					switch variable.Type {
					case mdaiv1.VariableTypeSet:
						if transformer == nil || transformer.Join == nil {
							return warnings, fmt.Errorf("variable %s: at least one transformer must be provided, such as 'join'", variable.StorageKey)
						}
					default:
						return warnings, fmt.Errorf("variable %s: unsupported variable type", variable.StorageKey)
					}
				}
			default:
				return warnings, fmt.Errorf("variable %s: unsupported storage type", variable.StorageKey)
			}
		}
	}

	evaluations := mdaihub.Spec.Evaluations
	if evaluations == nil || len(*evaluations) == 0 {
		warnings = append(warnings, "Evaluations are not specified")
	} else {
		for _, evaluation := range *evaluations {
			switch evaluation.Type {
			case mdaiv1.EvaluationTypePrometheusAlert:
				if evaluation.OnStatus == nil || (evaluation.OnStatus.Resolved == nil && evaluation.OnStatus.Firing == nil) {
					warnings = append(warnings, fmt.Sprintf("evaluation %s: 'onStatus' is not specified for evaluation type Prometheus Alert", evaluation.Name))
					continue
				}

				if err := v.validateOnStatus(evaluation.OnStatus.Firing, storageKeys, evaluation, "firing"); err != nil {
					return warnings, err
				}

				if err := v.validateOnStatus(evaluation.OnStatus.Resolved, storageKeys, evaluation, "resolved"); err != nil {
					return warnings, err
				}
			default:
				return warnings, fmt.Errorf("evaluation %s: unsupported type", evaluation.Name)
			}

		}
	}

	return warnings, nil
}

func (v *MdaiHubCustomValidator) validateOnStatus(action *mdaiv1.Action, storageKeys map[string]struct{}, evaluation mdaiv1.Evaluation, actionName string) error {
	if action == nil {
		return nil
	}
	if action.VariableUpdate == nil {
		return fmt.Errorf("action has to be specified for '%s', ex. 'variableUpdate', evaluation name: %s", actionName, evaluation.Name)
	}
	ref := action.VariableUpdate.VariableRef
	if _, exists := storageKeys[ref]; !exists {
		return fmt.Errorf("storage key %s does not exist, evaluation: %s", ref, evaluation.Name)
	}

	return nil
}
