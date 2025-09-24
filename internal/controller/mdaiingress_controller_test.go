package controller

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/errors"
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hubv1 "github.com/decisiveai/mdai-operator/api/v1"
)

var indexerOtelCol = func(obj client.Object) []string {
	a, ok := obj.(*hubv1.MdaiIngress)
	if !ok {
		return nil
	}
	otelCol := a.Spec.OtelCollector
	if otelCol.Name != "" && otelCol.Namespace != "" {
		return []string{fmt.Sprintf("%s/%s", otelCol.Namespace, otelCol.Name)}
	}
	return nil
}

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
						OtelCollector: hubv1.NamespacedName{
							Namespace: "default",
							Name:      "otel-collector",
						},
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
	builder := fake.NewClientBuilder().WithScheme(scheme).WithIndex(&hubv1.MdaiIngress{}, "spec.otelCol.compositeKey", indexerOtelCol)

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
	ctx := log.IntoContext(context.TODO(), logger)

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
			OtelCollector: hubv1.NamespacedName{
				Namespace: "default",
				Name:      "gateway",
			},
		},
	}

	fakeClient := fakeClient(scheme, &otelcol, mdaiIngress1)

	r := &MdaiIngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ok := r.onlyOneMdaiIngressPerOtelcol(ctx, "gateway", "default")
	require.True(t, ok)

	_ = zapLog.Sync()
}

func TestOnlyOneMdaiIngressPerOtelcol_MultipleRefs(t *testing.T) {
	scheme := createTestScheme()

	var buf bytes.Buffer
	zapLog := makeBufferLogger(&buf)
	logger := zapr.NewLogger(zapLog)
	ctx := log.IntoContext(context.TODO(), logger)

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
			OtelCollector: hubv1.NamespacedName{
				Namespace: "default",
				Name:      "gateway",
			},
		},
	}

	mdaiIngress2 := &hubv1.MdaiIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdai-ingress2",
			Namespace: "default",
		},
		Spec: hubv1.MdaiIngressSpec{
			OtelCollector: hubv1.NamespacedName{
				Namespace: "default",
				Name:      "gateway",
			},
		},
	}

	fakeClient := fakeClient(scheme, &otelcol, mdaiIngress1, mdaiIngress2)

	r := &MdaiIngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ok := r.onlyOneMdaiIngressPerOtelcol(ctx, "gateway", "default")
	require.False(t, ok)

	_ = zapLog.Sync()

	logs := buf.String()
	require.Contains(t, logs, "Multiple MdaiIngress instances referencing the same Otelcol")
}

func TestCoupledWithOtelcol(t *testing.T) {
	scheme := createTestScheme()

	var buf bytes.Buffer
	zapLog := makeBufferLogger(&buf)
	logger := zapr.NewLogger(zapLog)
	ctx := log.IntoContext(context.TODO(), logger)

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
			OtelCollector: hubv1.NamespacedName{
				Namespace: "default",
				Name:      "gateway",
			},
		},
	}

	mdaiIngress2 := &hubv1.MdaiIngress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mdai-ingress2",
			Namespace: "default",
		},
		Spec: hubv1.MdaiIngressSpec{
			OtelCollector: hubv1.NamespacedName{
				Namespace: "default",
				Name:      "non-existent",
			},
		},
	}

	fakeClient := fakeClient(scheme, &otelcol, mdaiIngress1, mdaiIngress2)

	r := &MdaiIngressReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ok := r.coupledWithOtelcol(ctx, "mdai-ingress1", "default")
	require.True(t, ok)

	ok = r.coupledWithOtelcol(ctx, "mdai-ingress2", "default")
	require.False(t, ok)
}

func makeBufferLogger(buf *bytes.Buffer) *zap.Logger {
	writer := zapcore.AddSync(buf)
	encoderCfg := zap.NewDevelopmentEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg), // or JSONEncoder if you prefer
		writer,
		zapcore.DebugLevel, // capture Info and Debug logs
	)
	return zap.New(core)
}
