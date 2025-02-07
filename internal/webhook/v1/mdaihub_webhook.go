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

// +kubebuilder:webhook:path=/mutate-mdai-mdai-ai-v1-mdaihub,mutating=true,failurePolicy=fail,sideEffects=None,groups=mdai.mdai.ai,resources=mdaihubs,verbs=create;update,versions=v1,name=mmdaihub-v1.kb.io,admissionReviewVersions=v1

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
// +kubebuilder:webhook:path=/validate-mdai-mdai-ai-v1-mdaihub,mutating=false,failurePolicy=fail,sideEffects=None,groups=mdai.mdai.ai,resources=mdaihubs,verbs=create;update,versions=v1,name=vmdaihub-v1.kb.io,admissionReviewVersions=v1

// MdaiHubCustomValidator struct is responsible for validating the MdaiHub resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MdaiHubCustomValidator struct {
	//TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &MdaiHubCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (v *MdaiHubCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaihub, ok := obj.(*mdaiv1.MdaiHub)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiHub object but got %T", obj)
	}
	mdaihublog.Info("Validation for MdaiHub upon creation", "name", mdaihub.GetName())

	return v.Validate(mdaihub)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type MdaiHub.
func (v *MdaiHubCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
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

	// TODO cross validate variables and evaluations
	variables := mdaihub.Spec.Variables
	if variables == nil {
		warnings = append(warnings, "Variables are not specified")
		return warnings, nil
	}

	storageKeys := make(map[string]bool)
	exportedVariableNames := make(map[string]bool)
	for _, variable := range *variables {
		switch *variable.StorageType {
		case mdaiv1.VariableSourceTypeBultInValkey:
			if storageKeys[variable.StorageKey] {
				return warnings, fmt.Errorf("storage key %s is duplicated", variable.StorageKey)
			}
			storageKeys[variable.StorageKey] = true

			if variable.With == nil {
				warnings = append(warnings, fmt.Sprintf("exported variable 'with' block is not specified, the storage key %s will be used and converted to uppercase", variable.StorageKey))
				continue
			}
			for _, with := range *variable.With {
				if exportedVariableNames[with.ExportedVariableName] == true {
					return warnings, fmt.Errorf("variable %s: exported variable name %s is duplicated", variable.StorageKey, with.ExportedVariableName)
				}
				exportedVariableNames[with.ExportedVariableName] = true

				transformer := with.Transformer
				if transformer == nil {
					continue
				}

				switch variable.Type {
				case mdaiv1.VariableTypeString:
					if transformer.Join != nil {
						return warnings, fmt.Errorf("variable %s: 'join' transformation is not supported for variable type 'string'", variable.StorageKey)
					}
				case mdaiv1.VariableTypeSet:
					if transformer.Join == nil {
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

	return warnings, nil
}

func validateWith(variable *mdaiv1.Variable, check func(transformer *mdaiv1.VariableTransformer) error) error {
	for _, with := range *variable.With {
		if with.Transformer == nil {
			continue
		}
		if err := check(with.Transformer); err != nil {
			return err
		}
	}
	return nil
}
