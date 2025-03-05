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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/DecisiveAI/mdai-operator/api/v1"
	// TODO (user): Add any additional imports if needed
)

func createSampleMdaiHub() *mdaiv1.MdaiHub {
	storageType := "mdai-valkey"
	relevantLabels := []string{"service_name"}
	defaultValue := "n/a"
	var duration1 prometheusv1.Duration = "5m"

	return &mdaiv1.MdaiHub{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdaihub-sample",
			Namespace: "mdai",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "mdai-operator",
				"app.kubernetes.io/managed-by": "kustomize",
			},
		},
		Spec: mdaiv1.MdaiHubSpec{
			Variables: &[]mdaiv1.Variable{
				{
					StorageKey:   "service_list_1",
					DefaultValue: &defaultValue,
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "SERVICE_LIST_REGEX",
							Transformer: &mdaiv1.VariableTransformer{
								Join: &mdaiv1.JoinFunction{
									Delimiter: "|",
								},
							},
						},
						{
							Name: "SERVICE_LIST_CSV",
							Transformer: &mdaiv1.VariableTransformer{
								Join: &mdaiv1.JoinFunction{
									Delimiter: ",",
								},
							},
						},
					},
					Type:        mdaiv1.VariableTypeSet,
					StorageType: mdaiv1.VariableStorageType(storageType),
				},
				{
					StorageKey:   "service_list_2",
					DefaultValue: &defaultValue,
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "SERVICE_LIST_2_REGEX",
							Transformer: &mdaiv1.VariableTransformer{
								Join: &mdaiv1.JoinFunction{
									Delimiter: "|",
								},
							},
						},
						{
							Name: "SERVICE_LIST_2_CSV",
							Transformer: &mdaiv1.VariableTransformer{
								Join: &mdaiv1.JoinFunction{
									Delimiter: ",",
								},
							},
						},
					},
					Type:        mdaiv1.VariableTypeSet,
					StorageType: mdaiv1.VariableStorageType(storageType),
				},
			},
			Observers: &[]mdaiv1.Observer{
				{
					Name:                    "watcher1",
					ResourceRef:             "watcher-collector",
					LabelResourceAttributes: []string{"service.name"},
					CountMetricName:         stringToStringPointer("mdai_watcher_one_count_total"),
					BytesMetricName:         stringToStringPointer("mdai_watcher_one_bytes_total"),
				},
				{
					Name:                    "watcher2",
					ResourceRef:             "watcher-collector",
					LabelResourceAttributes: []string{"team", "log_level"},
					CountMetricName:         stringToStringPointer("mdai_watcher_two_count_total"),
				},
				{
					Name:                    "watcher3",
					ResourceRef:             "watcher-nother-collector",
					LabelResourceAttributes: []string{"region", "log_level"},
					BytesMetricName:         stringToStringPointer("mdai_watcher_three_count_total"),
				},
				{
					Name:                    "watcher4",
					ResourceRef:             "watcher-collector",
					LabelResourceAttributes: []string{"service.name", "team", "region"},
					CountMetricName:         stringToStringPointer("mdai_watcher_four_count_total"),
					BytesMetricName:         stringToStringPointer("mdai_watcher_four_bytes_total"),
					Filter: &mdaiv1.ObserverFilter{
						ErrorMode: stringToStringPointer("ignore"),
						Logs: &mdaiv1.ObserverLogsFilter{
							LogRecord: []string{`attributes["log_level"] == "INFO"`},
						},
					},
				},
			},
			ObserverResources: &[]mdaiv1.ObserverResource{
				{
					Name:     "watcher-collector",
					Image:    stringToStringPointer("watcher-image:9.9.9"),
					Replicas: ptr.To(int32(3)),
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"Cpu":    resource.MustParse("500m"),
							"Memory": resource.MustParse("1Gi"),
						},
						Requests: v1.ResourceList{
							"Cpu":    resource.MustParse("200m"),
							"Memory": resource.MustParse("256Mi")},
					},
				},
				{
					Name:     "watcher-nother-collector",
					Image:    stringToStringPointer("watcher-image:4.2.0"),
					Replicas: ptr.To(int32(2)),
					Resources: &v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"Cpu":    resource.MustParse("300m"),
							"Memory": resource.MustParse("512Mi"),
						},
						Requests: v1.ResourceList{
							"Cpu":    resource.MustParse("100m"),
							"Memory": resource.MustParse("128Mi")},
					},
				},
			},
			Evaluations: &[]mdaiv1.Evaluation{
				{
					Name:     "logBytesOutTooHighBySvc",
					Type:     "mdai/prometheus_alert",
					Expr:     intstr.FromString("increase(mdai_log_bytes_sent_total[1h]) > 100*1024*1024"),
					Severity: "warning",
					OnStatus: &mdaiv1.PrometheusAlertEvaluationStatus{
						Firing: &mdaiv1.Action{
							VariableUpdate: &mdaiv1.VariableUpdate{
								VariableRef: "service_list_1",
								Operation:   "mdai/add_element",
							},
						},
						Resolved: &mdaiv1.Action{
							VariableUpdate: &mdaiv1.VariableUpdate{
								VariableRef: "service_list_1",
								Operation:   "mdai/remove_element",
							},
						},
					},
					For:            &duration1,
					RelevantLabels: &relevantLabels,
				},
			},
		},
	}
}

var _ = Describe("MdaiHub Webhook", func() {
	var (
		obj       *mdaiv1.MdaiHub
		oldObj    *mdaiv1.MdaiHub
		validator MdaiHubCustomValidator
		defaulter MdaiHubCustomDefaulter
	)

	BeforeEach(func() {
		obj = &mdaiv1.MdaiHub{}
		oldObj = &mdaiv1.MdaiHub{}
		validator = MdaiHubCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = MdaiHubCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating MdaiHub under Defaulting Webhook", func() {
		// TODO (user): Add logic for defaulting webhooks
		// Example:
		// It("Should apply defaults when a required field is empty", func() {
		//     By("simulating a scenario where defaults should be applied")
		//     obj.SomeFieldWithDefault = ""
		//     By("calling the Default method to apply defaults")
		//     defaulter.Default(ctx, obj)
		//     By("checking that the default values are set")
		//     Expect(obj.SomeFieldWithDefault).To(Equal("default_value"))
		// })
	})

	Context("When creating or updating MdaiHub under Validating Webhook", func() {
		It("Should deny creation if a required field is missing", func() {
			By("simulating an invalid creation scenario")
			obj := createSampleMdaiHub()
			(*obj.Spec.Variables)[0].StorageKey = "service_list_2"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).Error().To(HaveOccurred())
		})

		It("Should admit creation if all required fields are present", func() {
			By("simulating a valid creation scenario")
			obj := createSampleMdaiHub()
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should validate updates correctly", func() {
			By("simulating a valid update scenario")
			oldObj = createSampleMdaiHub()
			obj := createSampleMdaiHub()
			(*obj.Spec.Variables)[1].StorageKey = "service_list_3"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})
	})

})

func stringToStringPointer(s string) *string {
	return &s
}
