package cascadedelete

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticRetainPolicy(t *testing.T) {
	policy := NewStaticRetainPolicy("Foo", "Bar")
	require.True(t, policy.RetainOnCascade("Foo"))
	require.True(t, policy.RetainOnCascade("Bar"))
	require.False(t, policy.RetainOnCascade("Baz"))
	require.False(t, policy.RetainOnCascade(""))
}

func TestStaticRetainPolicyEmpty(t *testing.T) {
	policy := NewStaticRetainPolicy()
	require.False(t, policy.RetainOnCascade("Foo"))
}

// TestRetainPolicyDriftGuard pins the kind set the composition root is expected
// to provide. It uses concrete kind names deliberately (a _test.go is exempt
// from the §7 gate) to catch accidental drift in the retained set: the run
// kinds must retain, while non-run kinds (incl. the immutable-by-default kinds)
// must not.
func TestRetainPolicyDriftGuard(t *testing.T) {
	policy := NewStaticRetainPolicy("PipelineRun", "TriggerRun")

	require.True(t, policy.RetainOnCascade("PipelineRun"))
	require.True(t, policy.RetainOnCascade("TriggerRun"))

	// retain != immutable: immutable-by-default kinds are not retained on cascade.
	require.False(t, policy.RetainOnCascade("Model"))
	require.False(t, policy.RetainOnCascade("Deployment"))
	require.False(t, policy.RetainOnCascade("CachedOutput"))
	require.False(t, policy.RetainOnCascade("EvaluationReport"))
}
