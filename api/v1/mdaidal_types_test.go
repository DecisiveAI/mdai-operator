package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestImagePullPolicy_ToK8s(t *testing.T) {
	testCases := []struct {
		name     string
		policy   ImagePullPolicy
		expected corev1.PullPolicy
	}{
		{
			name:     "Empty policy should default to PullIfNotPresent",
			policy:   "",
			expected: corev1.PullIfNotPresent,
		},
		{
			name:     "Always policy",
			policy:   "Always",
			expected: corev1.PullAlways,
		},
		{
			name:     "IfNotPresent policy",
			policy:   "IfNotPresent",
			expected: corev1.PullIfNotPresent,
		},
		{
			name:     "Never policy",
			policy:   "Never",
			expected: corev1.PullNever,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.policy.ToK8s())
		})
	}
}
