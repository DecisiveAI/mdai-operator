package v1

import (
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func createSampleMdaiHub() *mdaiv1.MdaiHub {
	storageType := "mdai-valkey"
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
			Variables: []mdaiv1.Variable{
				{
					Key: "service_list_1",
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "SERVICE_LIST_REGEX",
							Transformers: []mdaiv1.VariableTransformer{
								{
									Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: "|",
									},
								},
							},
						},
						{
							Name: "SERVICE_LIST_CSV",
							Transformers: []mdaiv1.VariableTransformer{
								{
									Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: ",",
									},
								},
							},
						},
					},
					DataType:    mdaiv1.VariableDataTypeSet,
					StorageType: mdaiv1.VariableStorageType(storageType),
				},
				{
					Key: "service_list_2",
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "SERVICE_LIST_2_REGEX",
							Transformers: []mdaiv1.VariableTransformer{
								{
									Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: "|",
									},
								},
							},
						},
						{
							Name: "SERVICE_LIST_2_CSV",
							Transformers: []mdaiv1.VariableTransformer{
								{
									Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: ",",
									},
								},
							},
						},
					},
					DataType:    mdaiv1.VariableDataTypeSet,
					StorageType: mdaiv1.VariableStorageType(storageType),
				},
				{
					Key:         "string",
					DataType:    mdaiv1.VariableDataTypeString,
					StorageType: mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{{Name: "STR"}},
				},
				{
					Key:         "bool",
					DataType:    mdaiv1.VariableDataTypeBoolean,
					StorageType: mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{{Name: "BOOL"}},
				},
				{
					Key:         "int",
					DataType:    mdaiv1.VariableDataTypeInt,
					StorageType: mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{{Name: "INT"}},
				},
				{
					Key:         "map",
					DataType:    mdaiv1.VariableDataTypeMap,
					StorageType: mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{{Name: "MAP"}},
				},
				{
					Key:          "priority_list",
					Type:         mdaiv1.VariableTypeMeta,
					DataType:     mdaiv1.MetaVariableDataTypePriorityList,
					VariableRefs: []string{"ref1", "ref2", "ref3"},
					StorageType:  mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "PRIORITY_LIST",
							Transformers: []mdaiv1.VariableTransformer{
								{
									Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: ",",
									},
								},
							},
						},
					},
				},
				{
					Key:          "hashset",
					Type:         mdaiv1.VariableTypeMeta,
					DataType:     mdaiv1.MetaVariableDataTypeHashSet,
					VariableRefs: []string{"ref1", "ref2"},
					StorageType:  mdaiv1.VariableStorageType(storageType),
					SerializeAs: []mdaiv1.Serializer{
						{
							Name: "HASH_SET",
						},
					},
				},
			},
			PrometheusAlerts: []mdaiv1.PrometheusAlert{
				{
					Name:     "logBytesOutTooHighBySvc",
					Expr:     intstr.FromString("increase(mdai_log_bytes_sent_total[1h]) > 100*1024*1024"),
					Severity: "warning",
					For:      &duration1,
				},
			},
			Rules: []mdaiv1.AutomationRule{
				{
					Name: "automation-1",
					When: mdaiv1.When{
						AlertName: ptr.To("logBytesOutTooHighBySvc"),
						Status:    ptr.To("firing"),
					},
					Then: []mdaiv1.Action{
						{
							AddToSet: &mdaiv1.SetAction{
								Set:   "service_list_1",
								Value: "service_name",
							},
						},
					},
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
		It("Should deny creation if variable keys are duplicated", func() {
			By("simulating an invalid creation scenario")
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[0].Key = "service_list_2"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).Error().To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(`"mdaihub-sample" is invalid: [spec.variables[1].key: Duplicate value: "service_list_2"`)))
		})

		It("Should admit creation if all required fields are present", func() {
			By("simulating a valid creation scenario")
			obj := createSampleMdaiHub()
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail creation if expr does not validate", func() {
			By("simulating an invalid creation scenario")
			obj := createSampleMdaiHub()
			(obj.Spec.PrometheusAlerts)[0].Expr = intstr.FromString("increaser(mdai_log_bytes_sent_total[1h]) > 100*1024*1024")
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(`parse error: unknown function with name "increaser"`)))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should validate updates correctly", func() {
			By("simulating a valid update scenario")
			oldObj = createSampleMdaiHub()
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[1].Key = "service_list_3"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail validation if references changed", func() {
			oldObj = createSampleMdaiHub()
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[6].VariableRefs[0] = "service_list_3"
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(MatchError(ContainSubstring(`meta variable references must not change; delete and recreate the variable to update references`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if no references provided for meta variable", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[6].VariableRefs = nil
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`spec.variables[6].variableRefs: Required value: required for meta variable`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if references provided for non meta variable", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[1].VariableRefs = []string{"ref1"}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`MdaiHub.hub.mydecisive.ai "mdaihub-sample" is invalid: spec.variables[1].variableRefs: Forbidden: not supported for non-meta variables`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if more than two ref provided for hashmap", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[7].VariableRefs = []string{"ref1", "ref2", "ref3"}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`MdaiHub.hub.mydecisive.ai "mdaihub-sample" is invalid: spec.variables[7].variableRefs: Invalid value: []string{"ref1", "ref2", "ref3"}: Meta HashSet must have exactly 2 elements`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if exported variable name is duplicated", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[7].SerializeAs[0].Name = "SERVICE_LIST_CSV"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`MdaiHub.hub.mydecisive.ai "mdaihub-sample" is invalid: spec.variables[7].serializeAs[0].name: Duplicate value: "SERVICE_LIST_CSV"`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if priority list doesn't have transformers", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[6].SerializeAs[0].Transformers = nil
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring("MdaiHub.hub.mydecisive.ai \"mdaihub-sample\" is invalid: spec.variables[6].serializeAs[0].transformers: Required value: at least one transformer (e.g., 'join')")))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if transformers specified for boolean", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[3].SerializeAs[0].Transformers = []mdaiv1.VariableTransformer{
				{
					Type: mdaiv1.TransformerTypeJoin,
					Join: &mdaiv1.JoinTransformer{
						Delimiter: "|",
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`MdaiHub.hub.mydecisive.ai "mdaihub-sample" is invalid: spec.variables[3].serializeAs[0].transformers: Forbidden: transformers are not supported for variable type boolean`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if status is set without alertName", func() {
			By("setting status but clearing alertName in a rule")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].When.AlertName = nil

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alertName and status must be set together"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if updateType is set without variableUpdated", func() {
			By("setting updateType but leaving variableUpdated empty")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].When.UpdateType = ptr.To("added")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].when.updateType"))
			Expect(err.Error()).To(ContainSubstring("can only be set when variableUpdated is set"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if variableUpdated is not defined in spec.variables", func() {
			By("referencing an unknown variable in when.variableUpdated")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].When.VariableUpdated = ptr.To("does_not_exist")
			obj.Spec.Rules[0].When.UpdateType = ptr.To("set")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].when.variableUpdated"))
			Expect(err.Error()).To(ContainSubstring("not defined in spec.variables"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if alertName is not defined in spec.alerts", func() {
			By("referencing an unknown alert in when.alertName")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].When.AlertName = ptr.To("UnknownAlert")
			// keep status to trigger the existence check
			obj.Spec.Rules[0].When.Status = ptr.To("firing")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].when.alertName"))
			Expect(err.Error()).To(ContainSubstring("not defined in spec.alerts"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if an action item has no fields set", func() {
			By("providing an empty action (no addToSet/removeFromSet/callWebhook)")
			obj := createSampleMdaiHub()
			// Replace the default valid action with an empty one
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{}}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].then[0]"))
			Expect(err.Error()).To(ContainSubstring("at least one action must be specified"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if addToSet targets an unknown variable", func() {
			By("pointing addToSet.set to a variable that does not exist")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then[0].AddToSet.Set = "does_not_exist"

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].then[0].addToSet.set"))
			Expect(err.Error()).To(ContainSubstring("not defined in spec.variables"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should admit creation when removeFromSet references an existing set variable", func() {
			By("switching to removeFromSet with a valid set variable")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{
				{
					RemoveFromSet: &mdaiv1.SetAction{
						Set:   "service_list_1",
						Value: "service_name",
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if removeFromSet targets an unknown variable", func() {
			By("using removeFromSet on a non-existent variable")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{
				{
					RemoveFromSet: &mdaiv1.SetAction{
						Set:   "ghost_variable",
						Value: "service_name",
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].then[0].removeFromSet.set"))
			Expect(err.Error()).To(ContainSubstring("not defined in spec.variables"))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if callWebhook.url is an explicit empty string", func() {
			By("setting url to an empty string")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{CallWebhook: &mdaiv1.CallWebhookAction{}}}
			obj.Spec.Rules[0].Then[0].CallWebhook.URL.Value = ptr.To("")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`spec.rules[0].then[0].callWebhook.url`))
			Expect(err.Error()).To(ContainSubstring(`Required value: required`))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if callWebhook.url is only whitespace", func() {
			By("setting url to whitespace")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{CallWebhook: &mdaiv1.CallWebhookAction{}}}
			obj.Spec.Rules[0].Then[0].CallWebhook.URL.Value = ptr.To("   \t  ")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`spec.rules[0].then[0].callWebhook.url`))
			Expect(err.Error()).To(ContainSubstring(`Required value`))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should fail if callWebhook.url is not an absolute http(s) URL", func() {
			By("using a non-absolute/non-http URL")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{CallWebhook: &mdaiv1.CallWebhookAction{}}}
			obj.Spec.Rules[0].Then[0].CallWebhook.URL.Value = ptr.To("example.com/hook") // missing scheme

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`spec.rules[0].then[0].callWebhook.url`))
			Expect(err.Error()).To(ContainSubstring(`must be an absolute http(s) URL or a template`))
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should admit creation when callWebhook has a valid absolute URL and allowed method", func() {
			By("providing a proper https URL and a supported method")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{CallWebhook: &mdaiv1.CallWebhookAction{}}}
			obj.Spec.Rules[0].Then[0].CallWebhook.URL.Value = ptr.To("https://hooks.example.com/x")
			obj.Spec.Rules[0].Then[0].CallWebhook.Method = "PUT" // allowed

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should admit creation when callWebhook has a valid absolute URL and empty method (defaults to POST)", func() {
			By("supplying a valid URL without method")
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{{CallWebhook: &mdaiv1.CallWebhookAction{}}}
			obj.Spec.Rules[0].Then[0].CallWebhook.URL.Value = ptr.To("http://example.com/hook")

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should admit creation when callWebhook.url comes from a Secret", func() {
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{
				{
					CallWebhook: &mdaiv1.CallWebhookAction{
						URL: mdaiv1.StringOrFrom{
							ValueFrom: &mdaiv1.ValueFromSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "webhook-secret"},
									Key:                  "url",
								},
							},
						},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})

		It("Should admit creation when callWebhook.url comes from a ConfigMap", func() {
			obj := createSampleMdaiHub()
			obj.Spec.Rules[0].Then = []mdaiv1.Action{
				{
					CallWebhook: &mdaiv1.CallWebhookAction{
						URL: mdaiv1.StringOrFrom{
							ValueFrom: &mdaiv1.ValueFromSource{
								ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "webhook-config"},
									Key:                  "url",
								},
							},
						},
						Method: "PATCH",
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(Equal(admission.Warnings{}))
		})
	})
})
