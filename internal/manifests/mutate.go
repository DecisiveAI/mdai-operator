// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// nolint:gofumpt
package manifests

import (
	"errors"
	"fmt"
	"reflect"

	"dario.cat/mergo"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ImmutableFieldChangeError struct {
	Field string
}

func (e *ImmutableFieldChangeError) Error() string {
	// nolint:perfsprint
	return fmt.Sprintf("Immutable field change attempted: %s", e.Field)
}

var (
	ErrImmutableChange *ImmutableFieldChangeError
)

// MutateFuncFor returns a mutate function based on the
// existing resource's concrete type. It supports currently
// only the following types or else panics:
// - Service
// - Ingress
// - TargetAllocator
// In order for the operator to reconcile other types, they must be added here.
// The function returned takes no arguments but instead uses the existing and desired inputs here. Existing is expected
// to be set by the controller-runtime package through a client get call.
func MutateFuncFor(existing, desired client.Object) controllerutil.MutateFn {
	return func() error {
		// Get the existing annotations and override any conflicts with the desired annotations
		// This will preserve any annotations on the existing set.
		existingAnnotations := existing.GetAnnotations()
		if err := mergeWithOverride(&existingAnnotations, desired.GetAnnotations()); err != nil {
			return err
		}
		existing.SetAnnotations(existingAnnotations)

		// Get the existing labels and override any conflicts with the desired labels
		// This will preserve any labels on the existing set.
		existingLabels := existing.GetLabels()
		if err := mergeWithOverride(&existingLabels, desired.GetLabels()); err != nil {
			return err
		}
		existing.SetLabels(existingLabels)

		if ownerRefs := desired.GetOwnerReferences(); len(ownerRefs) > 0 {
			existing.SetOwnerReferences(ownerRefs)
		}

		//nolint:gocritic
		switch existing.(type) {
		case *corev1.Service:
			svc, ok := existing.(*corev1.Service)
			if !ok {
				return errors.New("cannot cast to Service")
			}
			wantSvc, ok := desired.(*corev1.Service)
			if !ok {
				return errors.New("cannot cast to Service")
			}
			mutateService(svc, wantSvc)

		case *networkingv1.Ingress:
			ing, ok := existing.(*networkingv1.Ingress)
			if !ok {
				return errors.New("cannot cast to Ingress")
			}
			wantIng, ok := desired.(*networkingv1.Ingress)
			if !ok {
				return errors.New("cannot cast to Ingress")
			}
			mutateIngress(ing, wantIng)

		default:
			t := reflect.TypeOf(existing).String()
			return fmt.Errorf("missing mutate implementation for resource type: %s", t)
		}
		return nil
	}
}

func mergeWithOverride(dst, src any) error {
	return mergo.Merge(dst, src, mergo.WithOverride)
}

func mutateIngress(existing, desired *networkingv1.Ingress) {
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations
	existing.Spec.DefaultBackend = desired.Spec.DefaultBackend
	existing.Spec.Rules = desired.Spec.Rules
	existing.Spec.TLS = desired.Spec.TLS
}

func mutateService(existing, desired *corev1.Service) {
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
}

func hasImmutableLabelChange(existingSelectorLabels, desiredLabels map[string]string) error {
	for k, v := range existingSelectorLabels {
		if vv, ok := desiredLabels[k]; !ok || vv != v {
			return &ImmutableFieldChangeError{Field: "Spec.Template.Metadata.Labels"}
		}
	}
	return nil
}
