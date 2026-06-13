package cascadedelete

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// testScheme returns a scheme with the v2 types registered so
// SetControllerReference can resolve the owner's GVK.
func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return scheme
}

func TestEnsureControllerRef(t *testing.T) {
	scheme := testScheme(t)
	owner := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-owner",
			Namespace: "test-namespace",
			UID:       types.UID("owner-uid-1"),
		},
	}

	t.Run("missing controller ref is set with correct fields", func(t *testing.T) {
		child := &v2pb.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "test-namespace"},
		}

		changed, err := EnsureControllerRef(child, owner, scheme)
		require.NoError(t, err)
		require.True(t, changed)

		refs := child.GetOwnerReferences()
		require.Len(t, refs, 1)
		ref := refs[0]
		require.Equal(t, v2pb.GroupVersion.String(), ref.APIVersion)
		require.Equal(t, "Pipeline", ref.Kind)
		require.Equal(t, "test-owner", ref.Name)
		require.Equal(t, types.UID("owner-uid-1"), ref.UID)
		require.NotNil(t, ref.Controller)
		require.True(t, *ref.Controller)
		require.NotNil(t, ref.BlockOwnerDeletion)
		require.True(t, *ref.BlockOwnerDeletion)
	})

	t.Run("already owned is a no-op", func(t *testing.T) {
		child := &v2pb.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "test-namespace"},
		}

		changed, err := EnsureControllerRef(child, owner, scheme)
		require.NoError(t, err)
		require.True(t, changed)
		// A second call must be a no-op: no change, no duplicate ref.
		changed, err = EnsureControllerRef(child, owner, scheme)
		require.NoError(t, err)
		require.False(t, changed)
		require.Len(t, child.GetOwnerReferences(), 1)
	})

	t.Run("preserves unrelated non-controller owner refs", func(t *testing.T) {
		f := false
		child := &v2pb.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "child",
				Namespace: "test-namespace",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "unrelated",
					UID:        types.UID("other-uid"),
					Controller: &f,
				}},
			},
		}

		changed, err := EnsureControllerRef(child, owner, scheme)
		require.NoError(t, err)
		require.True(t, changed)
		require.Len(t, child.GetOwnerReferences(), 2)
	})

	t.Run("conflicting different controller returns AlreadyOwnedError", func(t *testing.T) {
		tr := true
		child := &v2pb.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "child",
				Namespace: "test-namespace",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v2pb.GroupVersion.String(),
					Kind:       "Pipeline",
					Name:       "other-owner",
					UID:        types.UID("other-controller-uid"),
					Controller: &tr,
				}},
			},
		}

		changed, err := EnsureControllerRef(child, owner, scheme)
		require.Error(t, err)
		require.False(t, changed)
		// Must not have appended a second controller ref.
		require.Len(t, child.GetOwnerReferences(), 1)
	})
}
