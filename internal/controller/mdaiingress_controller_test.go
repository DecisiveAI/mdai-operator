package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
)

var _ = Describe("MdaiIngress Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		mdaiingress := &hubv1.MdaiIngress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: "default",
			},
			Spec: hubv1.MdaiIngressSpec{
				GrpcService:    &hubv1.IngressService{Type: "NodePort"},
				NonGrpcService: &hubv1.IngressService{Type: "NodePort"},
				CloudType:      hubv1.CloudProviderAws,
			},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MdaiIngress")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := k8sClient.Get(ctx, typeNamespacedName, mdaiingress)
			if err != nil && errors.IsNotFound(err) {
				resource := &hubv1.MdaiIngress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: hubv1.MdaiIngressSpec{
						GrpcService:    &hubv1.IngressService{Type: "NodePort"},
						NonGrpcService: &hubv1.IngressService{Type: "NodePort"},
						CloudType:      hubv1.CloudProviderAws,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			resource := &hubv1.MdaiIngress{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MdaiIngress")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			scheme := runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			_ = otelv1beta1.AddToScheme(scheme)
			_ = hubv1.AddToScheme(scheme)

			cacheOptions := cache.Options{}

			metricsServerOptions := metricsserver.Options{
				BindAddress: "0",
			}
			mgr, errMgr := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:  scheme,
				Cache:   cacheOptions,
				Metrics: metricsServerOptions,
			})
			Expect(errMgr).NotTo(HaveOccurred())

			controllerReconciler := &MdaiIngressReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
				Cache:  mgr.GetCache(),
			}

			go func() {
				errMgrStart := mgr.Start(ctx)
				Expect(errMgrStart).NotTo(HaveOccurred())
			}()

			ok := mgr.GetCache().WaitForCacheSync(ctx)
			Expect(ok).To(BeTrue())

			_, errReconcile := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(errReconcile).NotTo(HaveOccurred())
		})
	})
})
