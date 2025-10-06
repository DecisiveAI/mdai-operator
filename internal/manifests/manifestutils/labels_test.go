package manifestutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabels_Basic(t *testing.T) {
	meta := metav1.ObjectMeta{
		Name:      "example",
		Namespace: "default",
		Labels: map[string]string{
			"keep":      "yes",
			"filtered":  "no",
			"component": "old",
		},
	}

	image := "ghcr.io/example/app:1.2.3"
	filterLabels := []string{"filtered"}

	result := Labels(meta, "my-app", image, filterLabels)

	assert.Equal(t, "yes", result["keep"])
	assert.NotContains(t, result, "filtered")
	assert.Equal(t, "1.2.3", result["app.kubernetes.io/version"])
	assert.Equal(t, "my-app", result["app.kubernetes.io/name"])
}
