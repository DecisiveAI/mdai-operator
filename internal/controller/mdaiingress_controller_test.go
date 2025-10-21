package controller

import (
	"bytes"
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/go-logr/zapr"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
				OtelCollector:  hubv1.OtelColRef{Name: "other"},
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
						OtelCollector:  hubv1.OtelColRef{Name: "other"},
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

			err := SetMdaiIngressIndexers(ctx, mgr)
			Expect(err).NotTo(HaveOccurred())

			err = mgr.GetFieldIndexer().IndexField(
				ctx,
				&corev1.Service{},
				resourceOwnerKey,
				func(rawObj client.Object) []string {
					service, ok := rawObj.(*corev1.Service)
					if !ok {
						return nil
					}
					owners := service.GetOwnerReferences()
					if len(owners) == 0 {
						return nil
					}
					return []string{owners[0].Name}
				})
			Expect(err).NotTo(HaveOccurred())

			err = mgr.GetFieldIndexer().IndexField(
				ctx,
				&networkingv1.Ingress{},
				resourceOwnerKey,
				func(rawObj client.Object) []string {
					service, ok := rawObj.(*corev1.Service)
					if !ok {
						return nil
					}
					owners := service.GetOwnerReferences()
					if len(owners) == 0 {
						return nil
					}
					return []string{owners[0].Name}
				})
			Expect(err).NotTo(HaveOccurred())

			logger := zap.NewNop()
			controllerReconciler := &MdaiIngressReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
				Logger: logger,
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

func fakeClient(scheme *runtime.Scheme, objs ...client.Object) client.WithWatch {
	builder := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&hubv1.MdaiIngress{}, "spec.otelCol.compositeKey", IndexerOtelCol)

	for _, obj := range objs {
		builder = builder.WithObjects(obj).WithStatusSubresource(obj)
	}

	return builder.Build()
}

func TestOnlyOneMdaiIngressPerOtelcol_SingleRef(t *testing.T) {
	scheme := createTestScheme()

	var buf bytes.Buffer
	zapLog := makeBufferLogger(&buf)
	logger := zapr.NewLogger(zapLog)
	ctx := log.IntoContext(t.Context(), logger)

	otelcol := otelv1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: "default",
		},
		Spec: otelv1beta1.OpenTelemetryCollectorSpec{
			Config: otelv1beta1.Config{},
		},
	}

	mdaiIngress1 := &hubv1.MdaiIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdai-ingress1",
			Namespace: "default",
		},
		Spec: hubv1.MdaiIngressSpec{
			OtelCollector: hubv1.OtelColRef{
				Name: "gateway",
			},
		},
	}

	fClient := fakeClient(scheme, &otelcol, mdaiIngress1)

	r := &MdaiIngressReconciler{
		Client: fClient,
		Scheme: scheme,
	}

	ok := r.otelColExists(ctx, "gateway", "default")
	require.True(t, ok)

	_ = zapLog.Sync()
}

func TestCoupledWithOtelcol(t *testing.T) {
	scheme := createTestScheme()

	var buf bytes.Buffer
	zapLog := makeBufferLogger(&buf)
	logger := zapr.NewLogger(zapLog)
	ctx := log.IntoContext(t.Context(), logger)

	otelcol1 := otelv1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway1",
			Namespace: "default",
		},
		Spec: otelv1beta1.OpenTelemetryCollectorSpec{
			Config: otelv1beta1.Config{},
		},
	}

	otelcol2 := otelv1beta1.OpenTelemetryCollector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway2",
			Namespace: "default",
		},
		Spec: otelv1beta1.OpenTelemetryCollectorSpec{
			Config: otelv1beta1.Config{},
		},
	}

	mdaiIngress := &hubv1.MdaiIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdai-ingress1",
			Namespace: "default",
		},
		Spec: hubv1.MdaiIngressSpec{
			OtelCollector: hubv1.OtelColRef{
				Name: "gateway1",
			},
		},
	}

	fakeClient := fakeClient(scheme, &otelcol1, &otelcol2, mdaiIngress)

	r := &MdaiIngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ok := r.otelColExists(ctx, "gateway1", "default")
	require.True(t, ok)
}

func makeBufferLogger(buf *bytes.Buffer) *zap.Logger {
	writer := zapcore.AddSync(buf)
	encoderCfg := zap.NewDevelopmentEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		writer,
		zapcore.DebugLevel,
	)
	return zap.New(core)
}
