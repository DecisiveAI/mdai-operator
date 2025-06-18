package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	// TODO (user): Add any additional imports if needed
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
								{Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: "|",
									},
								},
							},
						},
						{
							Name: "SERVICE_LIST_CSV",
							Transformers: []mdaiv1.VariableTransformer{
								{Type: mdaiv1.TransformerTypeJoin,
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
								{Type: mdaiv1.TransformerTypeJoin,
									Join: &mdaiv1.JoinTransformer{
										Delimiter: "|",
									},
								},
							},
						},
						{
							Name: "SERVICE_LIST_2_CSV",
							Transformers: []mdaiv1.VariableTransformer{
								{Type: mdaiv1.TransformerTypeJoin,
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
								{Type: mdaiv1.TransformerTypeJoin,
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
			PrometheusAlert: []mdaiv1.PrometheusAlert{
				{
					Name:     "logBytesOutTooHighBySvc",
					Expr:     intstr.FromString("increase(mdai_log_bytes_sent_total[1h]) > 100*1024*1024"),
					Severity: "warning",
					For:      &duration1,
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
			(obj.Spec.Variables)[0].Key = "service_list_2"
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

		It("Should fail creation if expr does not validate", func() {
			By("simulating an invalid creation scenario")
			obj := createSampleMdaiHub()
			(obj.Spec.PrometheusAlert)[0].Expr = intstr.FromString("increaser(mdai_log_bytes_sent_total[1h]) > 100*1024*1024")
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
			Expect(err).To(MatchError(ContainSubstring(`meta variable references must not change, delete and recreate the variable to update references`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if no references provided for meta variable", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[6].VariableRefs = nil
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`hub mdaihub-sample, variable priority_list: no variable references provided for meta variable`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if references provided for non meta variable", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[1].VariableRefs = []string{"ref1"}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`hub mdaihub-sample, variable service_list_2: variable references are not supported for non-meta variables`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if more than two ref provided for hashmap", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[7].VariableRefs = []string{"ref1", "ref2", "ref3"}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`hub mdaihub-sample, variable hashset: variable references for Meta HashSet must have exactly 2 elements`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if exported variable name is duplicated", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[7].SerializeAs[0].Name = "SERVICE_LIST_CSV"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring(`hub mdaihub-sample, variable hashset: exported variable name SERVICE_LIST_CSV is duplicated`)))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if priority list doesn't have transformers", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[6].SerializeAs[0].Transformers = nil
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring("hub mdaihub-sample, variable priority_list: at least one transformer must be provided, such as 'join'")))
			Expect(warnings).To(BeEmpty())
		})

		It("Should fail if transformers specified for boolean", func() {
			obj := createSampleMdaiHub()
			(obj.Spec.Variables)[3].SerializeAs[0].Transformers = []mdaiv1.VariableTransformer{
				{Type: mdaiv1.TransformerTypeJoin,
					Join: &mdaiv1.JoinTransformer{
						Delimiter: "|",
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(MatchError(ContainSubstring("hub mdaihub-sample, variable bool: transformers are not supported for variable type boolean")))
			Expect(warnings).To(BeEmpty())
		})
	})

})
