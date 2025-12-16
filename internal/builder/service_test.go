package builder

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceBuilderEquivalence(t *testing.T) {
	t.Parallel()

	want := wantServiceBase()
	got := buildServiceWithBuilder()

	diff := cmp.Diff(want.Spec, got.Spec)
	require.Empty(t, diff, "Spec differs (-want +got):\n%s", diff)
}

func TestServiceBuilderHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() *corev1.Service
		want  *corev1.Service
	}{
		{
			name: "labels and selector labels are set",
			build: func() *corev1.Service {
				svc := &corev1.Service{}
				Service(svc).
					WithLabel("a", "1").
					WithLabel("b", "2").
					WithSelectorLabel("sel", "match").
					WithSelectorLabel("app", "svc")
				return svc
			},
			want: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"a": "1", "b": "2"},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"sel": "match", "app": "svc"},
				},
			},
		},
		{
			name: "ports and service type are set",
			build: func() *corev1.Service {
				svc := &corev1.Service{}
				Service(svc).
					WithPorts(
						corev1.ServicePort{Name: "http", Port: 80},
						corev1.ServicePort{Name: "https", Port: 443},
					).
					WithType(corev1.ServiceTypeNodePort)
				return svc
			},
			want: &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "http", Port: 80},
						{Name: "https", Port: 443},
					},
					Type: corev1.ServiceTypeNodePort,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.build()
			diff := cmp.Diff(tt.want, got)
			require.Empty(t, diff, "service differs (-want +got):\n%s", diff)
		})
	}
}
