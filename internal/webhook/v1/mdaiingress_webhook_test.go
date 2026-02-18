package v1

import (
	"context"

	"github.com/mydecisive/mdai-operator/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"

	mdaiv1 "github.com/mydecisive/mdai-operator/api/v1"
)

var _ = Describe("MdaiIngress Webhook", func() {
	var (
		obj       *mdaiv1.MdaiIngress
		oldObj    *mdaiv1.MdaiIngress
		defaulter MdaiIngressCustomDefaulter
		scheme    *runtime.Scheme
		ctx       context.Context
		validator *MdaiIngressCustomValidator
	)

	BeforeEach(func() {
		obj = &mdaiv1.MdaiIngress{
			Spec: mdaiv1.MdaiIngressSpec{
				GrpcService: &mdaiv1.IngressService{
					Type: "",
				},
				NonGrpcService: &mdaiv1.IngressService{
					Type: "",
				},
				CloudType: "",
			},
		}
		oldObj = &mdaiv1.MdaiIngress{}
		defaulter = MdaiIngressCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")

		scheme = runtime.NewScheme()
		Expect(mdaiv1.AddToScheme(scheme)).To(Succeed())

		ctx = context.Background()
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating MdaiIngress under Defaulting Webhook", func() {
		It("Should apply defaults when a required field is empty", func() {
			By("simulating a scenario where defaults should be applied")
			obj.Spec.GrpcService.Type = ""
			obj.Spec.NonGrpcService.Type = ""
			obj.Spec.CloudType = ""
			By("calling the Default method to apply defaults")
			err := defaulter.Default(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
			By("checking that the default values are set")
			Expect(obj.Spec.CloudType).To(Equal(mdaiv1.CloudProviderAws))
			Expect(obj.Spec.GrpcService.Type).To(Equal(corev1.ServiceTypeNodePort))
			Expect(obj.Spec.NonGrpcService.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
		})

		It("Should successfully validate upon Creation", func() {
			ingress := &mdaiv1.MdaiIngress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: "ns",
				},
				Spec: mdaiv1.MdaiIngressSpec{
					OtelCollector: mdaiv1.OtelColRef{
						Name: "otel",
					},
				},
			}

			builder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ingress)

			cl := builder.WithIndex(&mdaiv1.MdaiIngress{}, controller.MdaiIngressOtelColLookupKey, func(obj client.Object) []string {
				ing, ok := obj.(*mdaiv1.MdaiIngress)
				Expect(ok).To(BeTrue())
				if ing.Spec.OtelCollector.Name == "" {
					return nil
				}
				return []string{ing.Spec.OtelCollector.Name}
			}).
				Build()

			validator = &MdaiIngressCustomValidator{client: cl}

			_, err := validator.ValidateCreate(ctx, ingress)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns error when OtelCol reference is duplicated by another MdaiIngress", func() {
			ingress1 := &mdaiv1.MdaiIngress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a",
					Namespace: "ns",
				},
				Spec: mdaiv1.MdaiIngressSpec{
					OtelCollector: mdaiv1.OtelColRef{
						Name: "otel",
					},
				},
			}

			ingress2 := &mdaiv1.MdaiIngress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "b",
					Namespace: "ns",
				},
				Spec: mdaiv1.MdaiIngressSpec{
					OtelCollector: mdaiv1.OtelColRef{
						Name: "otel",
					},
				},
			}

			builder := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ingress1, ingress2)

			cl := builder.WithIndex(&mdaiv1.MdaiIngress{}, controller.MdaiIngressOtelColLookupKey, func(obj client.Object) []string {
				ing, ok := obj.(*mdaiv1.MdaiIngress)
				Expect(ok).To(BeTrue())
				if ing.Spec.OtelCollector.Name == "" {
					return nil
				}
				return []string{ing.Spec.OtelCollector.Name}
			}).
				Build()

			validator = &MdaiIngressCustomValidator{client: cl}

			_, err := validator.Validate(ctx, ingress2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OtelCol reference is already used by MdaiIngress ns/a"))
		})
	})
})
