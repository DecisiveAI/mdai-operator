package builder

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestServiceBuilderEquivalence(t *testing.T) {
	t.Parallel()

	want := wantServiceBase()
	got := buildServiceWithBuilder()

	diff := cmp.Diff(want.Spec, got.Spec)
	require.Empty(t, diff, "Spec differs (-want +got):\n%s", diff)
}
