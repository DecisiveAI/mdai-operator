package builder

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestDeploymentBuilderEquivalence(t *testing.T) {
	t.Parallel()

	container := wantContainerBase()
	want := wantDeploymentBase(container)
	got := buildDeploymentWithBuilder(container)

	diff := cmp.Diff(want, got)
	require.Empty(t, diff, "Spec differs (-want +got):\n%s", diff)
}

func TestDeploymentBuilderEquivalenceWithAWSSecret(t *testing.T) {
	t.Parallel()

	container := wantContainerWithSecret(wantContainerBase(), *awsAccessKeySecret)
	want := wantDeploymentBase(container)
	got := buildDeploymentWithBuilder(container)

	diff := cmp.Diff(want, got)
	require.Empty(t, diff, "Spec differs (-want +got):\n%s", diff)
}
