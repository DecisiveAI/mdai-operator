package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hubv1 "github.com/mydecisive/mdai-operator/api/v1"
)

var _ = Describe("MdaiDal Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		mdaidal := &hubv1.MdaiDal{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MdaiDal")
			err := k8sClient.Get(ctx, typeNamespacedName, mdaidal)
			if err != nil && errors.IsNotFound(err) {
				resource := &hubv1.MdaiDal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: hubv1.MdaiDalSpec{
						S3: hubv1.MdaiDalS3Config{
							Bucket: "test-bucket",
						},
						AWS: hubv1.MdaiDalAWSConfig{
							Region: "us-east-1",
						},
						Granularity: "minutely",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &hubv1.MdaiDal{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MdaiDal")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MdaiDalReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling again to ensure all resources are created")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the Deployment was created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, "10s", "1s").Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("public.ecr.aws/decisiveai/mdai-dal"))

			By("Checking if the Service was created")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, service)
			}, "10s", "1s").Should(Succeed())
			Expect(service.Spec.Ports).To(HaveLen(2))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(4317)))
			Expect(service.Spec.Ports[1].Port).To(Equal(int32(4318)))

			By("Checking if the ConfigMap was created")
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-mdai-dal-config", Namespace: "default"}, configMap)
			}, "10s", "1s").Should(Succeed())
			Expect(configMap.Data["MDAI_DAL_S3_BUCKET"]).To(Equal("test-bucket"))
			Expect(configMap.Data["MDAI_DAL_GRANULARITY"]).To(Equal("minutely"))
		})
	})
})

var _ = Describe("MdaiDal controller setup", func() {
	It("registers the controller with the manager", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  scheme.Scheme,
			Metrics: server.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())

		reconciler := &MdaiDalReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		Expect(reconciler.SetupWithManager(mgr)).To(Succeed())
	})
})
