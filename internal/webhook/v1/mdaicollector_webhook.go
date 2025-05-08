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

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
)

// nolint:unused
// log is for logging in this package.
var mdaicollectorlog = logf.Log.WithName("mdaicollector-resource")

// SetupMdaiCollectorWebhookWithManager registers the webhook for MdaiCollector in the manager.
func SetupMdaiCollectorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&mdaiv1.MdaiCollector{}).
		WithValidator(&MdaiCollectorCustomValidator{}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-hub-mydecisive-ai-v1-mdaicollector,mutating=false,failurePolicy=fail,sideEffects=None,groups=hub.mydecisive.ai,resources=mdaicollectors,verbs=create;update,versions=v1,name=vmdaicollector-v1.kb.io,admissionReviewVersions=v1

// MdaiCollectorCustomValidator struct is responsible for validating the MdaiCollector resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type MdaiCollectorCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &MdaiCollectorCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type MdaiCollector.
func (v *MdaiCollectorCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaicollector, ok := obj.(*mdaiv1.MdaiCollector)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiCollector object but got %T", obj)
	}
	mdaicollectorlog.Info("Validation for MdaiCollector upon creation", "name", mdaicollector.GetName())

	return v.Validate(mdaicollector)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type MdaiCollector.
func (v *MdaiCollectorCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	mdaicollector, ok := newObj.(*mdaiv1.MdaiCollector)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiCollector object for the newObj but got %T", newObj)
	}
	mdaicollectorlog.Info("Validation for MdaiCollector upon update", "name", mdaicollector.GetName())

	return v.Validate(mdaicollector)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type MdaiCollector.
func (v *MdaiCollectorCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdaicollector, ok := obj.(*mdaiv1.MdaiCollector)
	if !ok {
		return nil, fmt.Errorf("expected a MdaiCollector object but got %T", obj)
	}
	mdaicollectorlog.Info("Validation for MdaiCollector upon deletion", "name", mdaicollector.GetName())

	return v.Validate(mdaicollector)
}

func (v *MdaiCollectorCustomValidator) Validate(mdaiCollector *mdaiv1.MdaiCollector) (admission.Warnings, error) {
	warnings := admission.Warnings{}

	if mdaiCollector == nil {
		return warnings, fmt.Errorf("expected a MdaiCollector object but got nil")
	}

	spec := mdaiCollector.Spec

	logsConfigPtr := spec.Logs
	awsConfigPtr := spec.AWSConfig

	if logsConfigPtr == nil {
		warnings = append(warnings, "logs configuration not present in MDAI Collector spec")
	} else {
		var (
			s3LogsConfigPtr *mdaiv1.S3LogsConfig
			s3LogsConfig    mdaiv1.S3LogsConfig
			accessKeySecret *string
		)

		s3LogsConfigPtr = logsConfigPtr.S3
		if s3LogsConfigPtr != nil {
			s3LogsConfig = *s3LogsConfigPtr
			if s3LogsConfig.S3Bucket == nil {
				return warnings, fmt.Errorf("s3 logs configuration given but s3Bucket not specified; cannot write logs to s3")
			}
			if s3LogsConfig.S3Region == nil {
				return warnings, fmt.Errorf("s3 logs configuration given but s3Region not specified; cannot write logs to s3")
			}
		}

		if awsConfigPtr == nil && s3LogsConfigPtr != nil {
			return warnings, fmt.Errorf("got s3 logs configuration, but AWSConfig not specified; cannot write logs to s3 without access secret")
		}

		if awsConfigPtr != nil {
			accessKeySecret = awsConfigPtr.AWSAccessKeySecret
		}

		if accessKeySecret == nil && s3LogsConfigPtr != nil {
			return warnings, fmt.Errorf("got s3 logs configuration, but awsConfig.accessKeySecret not specified; cannot write logs to s3 without access secret")
		}
	}

	return warnings, nil
}
