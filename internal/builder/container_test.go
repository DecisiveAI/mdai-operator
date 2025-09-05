package builder

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestContainerBuilderEquivalence(t *testing.T) {
	t.Parallel()

	want := wantContainerBase()
	got := buildContainerWithBuilder(nil)

	diff := cmp.Diff(want, got, cmpContainerOpts)
	require.Empty(t, diff, "container differs (-want +got):\n%s", diff)
}

func TestContainerBuilderEquivalenceWithAWSSecret(t *testing.T) {
	t.Parallel()

	want := wantContainerWithSecret(wantContainerBase(), *awsAccessKeySecret)
	got := buildContainerWithBuilder(awsAccessKeySecret)

	diff := cmp.Diff(want, got, cmpContainerOpts)
	require.Empty(t, diff, "container differs (-want +got):\n%s", diff)
}
