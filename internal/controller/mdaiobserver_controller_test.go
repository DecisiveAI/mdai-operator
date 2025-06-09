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

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
)

var _ = Describe("MdaiObserver Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		mdaiobserver := &hubv1.MdaiObserver{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MdaiObserver")
			err := k8sClient.Get(ctx, typeNamespacedName, mdaiobserver)
			if err != nil && errors.IsNotFound(err) {
				resource := &hubv1.MdaiObserver{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &hubv1.MdaiObserver{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MdaiObserver")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MdaiObserverReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

func TestBuildCollectorConfig(t *testing.T) {
	cr := &hubv1.MdaiObserver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-observer",
			Namespace: "default",
		},
		Spec:   hubv1.MdaiObserverSpec{},
		Status: hubv1.MdaiObserverStatus{},
	}
	observers := []hubv1.Observer{
		{
			Name:                    "obs1",
			LabelResourceAttributes: []string{"label1", "label2"},
		},
	}
	cr.Spec.Observers = observers
	observerResource := hubv1.ObserverResource{}

	scheme := createTestScheme()
	fakeClient := observerFakeClient(scheme, cr)
	recorder := record.NewFakeRecorder(10)

	adapter := NewObserverAdapter(cr, logr.Discard(), fakeClient, recorder, scheme)
	config, err := adapter.getObserverCollectorConfig(observers, observerResource)
	if err != nil {
		t.Fatalf("getObserverCollectorConfig returned error: %v", err)
	}
	if !strings.Contains(config, "obs1") {
		t.Errorf("Expected collector config to contain observer name %q, got: %s", "obs1", config)
	}
}

func observerFakeClient(scheme *runtime.Scheme, cr *hubv1.MdaiObserver) client.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cr).
		WithStatusSubresource(cr).
		Build()
}

func TestEnsureObserversSynchronized_WithObservers(t *testing.T) {
	ctx := context.TODO()
	scheme := createTestScheme()

	observer := hubv1.Observer{
		Name:                    "observer4",
		LabelResourceAttributes: []string{"service.name", "team", "region"},
		CountMetricName:         ptr.To("mdai_observer_four_count_total"),
		BytesMetricName:         ptr.To("mdai_observer_four_bytes_total"),
		Filter: &hubv1.ObserverFilter{
			ErrorMode: ptr.To("ignore"),
			Logs: &hubv1.ObserverLogsFilter{
				LogRecord: []string{`attributes["log_level"] == "INFO"`},
			},
		},
	}
	observers := []hubv1.Observer{observer}
	observerResource := hubv1.ObserverResource{
		Image: ptr.To("public.ecr.aws/p3k6k6h3/observer-observer"),
	}
	observerResources := observerResource

	mdaiCR := &hubv1.MdaiObserver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-observer",
			Namespace: "default",
		},
		Spec: hubv1.MdaiObserverSpec{
			Observers:        observers,
			ObserverResource: observerResources,
		},
		Status: hubv1.MdaiObserverStatus{},
	}

	fakeClient := observerFakeClient(scheme, mdaiCR)
	recorder := record.NewFakeRecorder(10)
	adapter := NewObserverAdapter(mdaiCR, logr.Discard(), fakeClient, recorder, scheme)

	// Call ensureObserversSynchronized.
	opResult, err := adapter.ensureObserversSynchronized(ctx)
	if err != nil {
		t.Fatalf("ensureObserversSynchronized returned error: %v", err)
	}
	if opResult != ContinueOperationResult() {
		t.Errorf("expected ContinueOperationResult, got: %v", opResult)
	}

	configMapName := adapter.getScopedObserverResourceName("config")
	cm := &v1core.ConfigMap{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: mdaiCR.Namespace}, cm); err != nil {
		t.Fatalf("failed to get ConfigMap %q: %v", configMapName, err)
	}
	if _, ok := cm.Data["collector.yaml"]; !ok {
		t.Errorf("expected collector.yaml key in ConfigMap data, got: %v", cm.Data)
	}

	deploymentName := mdaiCR.Name + "-" + mdaiObserverResourceBaseName
	deploy := &appsv1.Deployment{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: mdaiCR.Namespace}, deploy); err != nil {
		t.Fatalf("failed to get Deployment %q: %v", deploymentName, err)
	}
	hash, ok := deploy.Spec.Template.Annotations["mdai-collector-config/sha256"]
	if !ok || hash == "" {
		t.Errorf("expected mdai-collector-config/sha256 annotation to be set in Deployment, got: %v", deploy.Spec.Template.Annotations)
	}

	serviceName := mdaiCR.Name + "-" + mdaiObserverResourceBaseName + "-service"
	svc := &v1core.Service{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: mdaiCR.Namespace}, svc); err != nil {
		t.Fatalf("failed to get Service %q: %v", serviceName, err)
	}
	expectedAppLabel := mdaiCR.Name + "-" + mdaiObserverResourceBaseName
	if svc.Spec.Selector["app"] != expectedAppLabel {
		t.Errorf("expected service selector app to be %q, got: %q", expectedAppLabel, svc.Spec.Selector["app"])
	}
	if len(svc.Spec.Ports) != 2 {
		t.Errorf("expected service to have two ports, got %d", len(svc.Spec.Ports))
	} else {
		port := svc.Spec.Ports[0]
		if port.Name != "otlp-grpc" || port.Port != 4317 || port.TargetPort.String() != "otlp-grpc" {
			t.Errorf("unexpected service port configuration: %+v", port)
		}
	}
}
