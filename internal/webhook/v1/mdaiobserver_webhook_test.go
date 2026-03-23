package v1

import (
	"testing"

	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("MdaiObserver Webhook", func() {
	var (
		obj       *mdaiv1.MdaiObserver
		oldObj    *mdaiv1.MdaiObserver
		validator MdaiObserverCustomValidator
	)

	BeforeEach(func() {
		obj = &mdaiv1.MdaiObserver{}
		oldObj = &mdaiv1.MdaiObserver{}
		validator = *NewMdaiObserverCustomValidator()
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating MdaiObserver under Validating Webhook", func() {
		It("Should deny creation if a required field is missing", func() {
			By("simulating an invalid creation scenario")
			obj = createObserver()
			obj.Spec.ObserverResource.Resources = nil
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{
				"ObserverResource test-observer does not define resource requests/limits",
			}))
			Expect(err).Error().To(Not(HaveOccurred()))
		})

		It("Should admit creation if all required fields are present", func() {
			By("simulating a valid creation scenario")
			obj = createObserver()
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should validate updates correctly", func() {
			By("simulating a valid update scenario")
			oldObj = createObserver()
			obj = createObserver()
			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject spanmetrics otel observer with overlapping groupByAttrs and connector dimensions", func() {
			By("simulating an invalid spanmetrics otel config")
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-otel",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Otel: &mdaiv1.SpanMetricsOtelConfig{
							GroupByAttrs: []string{"host.name"},
							ConnectorConfig: &apiextensionsv1.JSON{
								Raw: []byte(`{"dimensions":[{"name":"host.name"},{"name":"server.address"}]}`),
							},
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate attribute"))
		})

		It("Should accept spanmetrics otel observer with distinct groupByAttrs and connector dimensions", func() {
			By("simulating a valid spanmetrics otel config")
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-otel",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Otel: &mdaiv1.SpanMetricsOtelConfig{
							GroupByAttrs: []string{"host.name"},
							ConnectorConfig: &apiextensionsv1.JSON{
								Raw: []byte(`{"dimensions":[{"name":"server.address"}]}`),
							},
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should accept spanmetrics greptime observer with valid sink table TTL", func() {
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:   []string{"service_name", "span_name"},
							PrimaryKey:   "service_name",
							SinkTableTtl: "1hour 12min 5s",
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject spanmetrics greptime observer with invalid sink table TTL", func() {
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:   []string{"service_name", "span_name"},
							PrimaryKey:   "service_name",
							SinkTableTtl: "1fortnight",
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sinkTableTtl"))
		})

		It("Should accept spanmetrics greptime observer with valid flow aggregate interval", func() {
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:            []string{"service_name", "span_name"},
							PrimaryKey:            "service_name",
							FlowAggregateInterval: "2 months",
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject spanmetrics greptime observer with invalid flow aggregate interval", func() {
			obj = createObserver()
			obj.Spec.Observers = []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:            []string{"service_name", "span_name"},
							PrimaryKey:            "service_name",
							FlowAggregateInterval: "2months",
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(warnings).To(Equal(admission.Warnings{}))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid flowAggregateInterval"))
		})
	})
})

type fakeGreptimeInspector struct {
	existingTables map[string]bool
	err            error
}

func (f *fakeGreptimeInspector) TableExists(schema, table string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.existingTables[schema+"."+table], nil
}

func TestValidateSinkTableTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ttl     string
		wantErr bool
	}{
		{name: "empty is allowed", ttl: "", wantErr: false},
		{name: "single unit short suffix", ttl: "5m", wantErr: false},
		{name: "single unit long suffix", ttl: "1hour", wantErr: false},
		{name: "multiple units", ttl: "1hour 12min 5s", wantErr: false},
		{name: "spaces between number and unit", ttl: "1 hour 12 min 5 s", wantErr: false},
		{name: "invalid unit", ttl: "1fortnight", wantErr: true},
		{name: "missing number", ttl: "hour", wantErr: true},
		{name: "garbage suffix", ttl: "5minsx", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateSinkTableTTL(tt.ttl)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.ttl)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.ttl, err)
			}
		})
	}
}

func TestValidateFlowAggregateInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty is allowed", value: "", wantErr: false},
		{name: "valid singular", value: "1 day", wantErr: false},
		{name: "valid plural", value: "2 months", wantErr: false},
		{name: "valid short unit", value: "5 m", wantErr: false},
		{name: "missing space", value: "2months", wantErr: true},
		{name: "combined values not allowed", value: "1 hour 5 minutes", wantErr: true},
		{name: "missing number", value: "day", wantErr: true},
		{name: "invalid unit", value: "1 fortnight", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateFlowAggregateInterval(tt.value)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.value)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.value, err)
			}
		})
	}
}

func TestValidateGreptimeRecreatePolicy(t *testing.T) {
	t.Parallel()

	makeGreptimeObserver := func(dimensions []string, primaryKey string, allow bool) mdaiv1.Observer {
		return mdaiv1.Observer{
			Name:     "span-greptime",
			Provider: mdaiv1.GREPTIME_FLOW,
			Type:     mdaiv1.SPAN_METRICS,
			SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
				Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
					Dimensions:             dimensions,
					PrimaryKey:             primaryKey,
					AllowSinkTableRecreate: allow,
				},
			},
		}
	}

	tests := []struct {
		name           string
		oldObservers   []mdaiv1.Observer
		newObservers   []mdaiv1.Observer
		existingTables map[string]bool
		wantErr        bool
	}{
		{
			name: "deny dimension change when sink tables exist and recreate disabled",
			oldObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name"}, "service_name", false),
			},
			newObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name", "span_name"}, "service_name", false),
			},
			existingTables: map[string]bool{"public.golden_signals_traffic": true},
			wantErr:        true,
		},
		{
			name: "deny primary key change when sink tables exist and recreate disabled",
			oldObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name"}, "service_name", false),
			},
			newObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name"}, "span_name", false),
			},
			existingTables: map[string]bool{"public.golden_signals_errors": true},
			wantErr:        true,
		},
		{
			name: "allow dimension change when recreate enabled",
			oldObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name"}, "service_name", false),
			},
			newObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name", "span_name"}, "service_name", true),
			},
			existingTables: map[string]bool{"public.golden_signals_traffic": true},
			wantErr:        false,
		},
		{
			name: "allow dimension change when sink tables do not exist yet",
			oldObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name"}, "service_name", false),
			},
			newObservers: []mdaiv1.Observer{
				makeGreptimeObserver([]string{"service_name", "span_name"}, "service_name", false),
			},
			existingTables: map[string]bool{},
			wantErr:        false,
		},
		{
			name: "allow ttl-only change when recreate disabled",
			oldObservers: []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:   []string{"service_name"},
							PrimaryKey:   "service_name",
							SinkTableTtl: "3months",
						},
					},
				},
			},
			newObservers: []mdaiv1.Observer{
				{
					Name:     "span-greptime",
					Provider: mdaiv1.GREPTIME_FLOW,
					Type:     mdaiv1.SPAN_METRICS,
					SpanMetricsObserver: &mdaiv1.SpanMetricsObserverConfig{
						Greptime: &mdaiv1.SpanMetricsGreptimeConfig{
							Dimensions:   []string{"service_name"},
							PrimaryKey:   "service_name",
							SinkTableTtl: "7d",
						},
					},
				},
			},
			existingTables: map[string]bool{"public.golden_signals_traffic": true},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validator := &MdaiObserverCustomValidator{
				greptimeInspector: &fakeGreptimeInspector{existingTables: tt.existingTables},
			}
			oldObj := createObserver()
			oldObj.Spec.Observers = tt.oldObservers
			newObj := createObserver()
			newObj.Spec.Observers = tt.newObservers

			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			if tt.wantErr && err == nil {
				t.Fatal("expected update validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected update validation error: %v", err)
			}
		})
	}
}

func createObserver() *mdaiv1.MdaiObserver {
	return &mdaiv1.MdaiObserver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-observer",
			Namespace: "default",
		},
		Spec: mdaiv1.MdaiObserverSpec{
			Observers: []mdaiv1.Observer{
				{
					Name:     "watcher1",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.DATA_VOLUME,
					DataVolumeObserver: &mdaiv1.DataVolumeObserverConfig{
						LabelResourceAttributes: []string{"service.name"},
						CountMetricName:         ptr.To("mdai_watcher_one_count_total"),
						BytesMetricName:         ptr.To("mdai_watcher_one_bytes_total"),
					},
				},
				{
					Name:     "watcher2",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.DATA_VOLUME,
					DataVolumeObserver: &mdaiv1.DataVolumeObserverConfig{
						LabelResourceAttributes: []string{"team", "log_level"},
						CountMetricName:         ptr.To("mdai_watcher_two_count_total"),
					},
				},
				{
					Name:     "watcher3",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.DATA_VOLUME,
					DataVolumeObserver: &mdaiv1.DataVolumeObserverConfig{
						LabelResourceAttributes: []string{"region", "log_level"},
						BytesMetricName:         ptr.To("mdai_watcher_three_count_total"),
					},
				},
				{
					Name:     "watcher4",
					Provider: mdaiv1.OTEL_COLLECTOR,
					Type:     mdaiv1.DATA_VOLUME,
					Filter: &mdaiv1.ObserverFilter{
						ErrorMode: ptr.To("ignore"),
						Logs: &mdaiv1.ObserverLogsFilter{
							LogRecord: []string{`attributes["log_level"] == "INFO"`},
						},
					},
					DataVolumeObserver: &mdaiv1.DataVolumeObserverConfig{
						LabelResourceAttributes: []string{"service.name", "team", "region"},
						CountMetricName:         ptr.To("mdai_watcher_four_count_total"),
						BytesMetricName:         ptr.To("mdai_watcher_four_bytes_total"),
					},
				},
			},
			ObserverResource: mdaiv1.ObserverResource{
				Image:    "watcher-image:9.9.9",
				Replicas: int32(3),
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"Cpu":    resource.MustParse("500m"),
						"Memory": resource.MustParse("1Gi"),
					},
					Requests: corev1.ResourceList{
						"Cpu":    resource.MustParse("200m"),
						"Memory": resource.MustParse("256Mi"),
					},
				},
			},
		},
	}
}
