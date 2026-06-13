package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestCascadeRetainKinds_Exact guards the retain set against drift: it must be
// exactly {PipelineRun, TriggerRun} (see the cascade-delete plan §8).
func TestCascadeRetainKinds_Exact(t *testing.T) {
	assert.ElementsMatch(t, []string{"PipelineRun", "TriggerRun"}, cascadeRetainKinds,
		"cascade retain set must be exactly the Pipeline's children")
}

// TestCascadeRetainKinds_ExistInCrdObjects: every retained kind must be a real CRD
// kind (a key in v2pb.CrdObjects), so a typo can't silently disable retain.
func TestCascadeRetainKinds_ExistInCrdObjects(t *testing.T) {
	for _, k := range cascadeRetainKinds {
		_, ok := v2pb.CrdObjects[k]
		require.Truef(t, ok, "retain kind %q must exist in v2pb.CrdObjects", k)
	}
}

// TestCascadeRetainKinds_NotImmutableKinds: retain != immutable. The immutable
// kinds must NOT be in the retain set.
func TestCascadeRetainKinds_NotImmutableKinds(t *testing.T) {
	policy := cascadedelete.NewStaticRetainPolicy(cascadeRetainKinds...)
	for _, k := range []string{"CachedOutput", "EvaluationReport", "Model"} {
		assert.Falsef(t, policy.RetainOnCascade(k), "immutable kind %q must not be a cascade-retain kind", k)
	}
}

// TestCascadeRetainPolicy_Behavior: the provided policy retains the children and
// nothing else.
func TestCascadeRetainPolicy_Behavior(t *testing.T) {
	policy := cascadedelete.NewStaticRetainPolicy(cascadeRetainKinds...)
	assert.True(t, policy.RetainOnCascade("PipelineRun"))
	assert.True(t, policy.RetainOnCascade("TriggerRun"))
	assert.False(t, policy.RetainOnCascade("Deployment"))
	assert.False(t, policy.RetainOnCascade("Cluster"))
}
