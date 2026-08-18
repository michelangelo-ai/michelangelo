package cascadedelete

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationtypes "github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestStampOwnerRefOnCreateNilOwner(t *testing.T) {
	scheme := testScheme(t)
	child := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "child"}}

	// A nil client.Object owner must be a no-op (no panic, no ref stamped).
	var owner client.Object
	require.NoError(t, StampOwnerRefOnCreate(context.Background(), zap.NewNop(), scheme, child, owner))
	require.Empty(t, child.GetOwnerReferences())
}

func TestStampOwnerRefOnCreateHappyPath(t *testing.T) {
	scheme := testScheme(t)
	owner := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name: "owner",
			UID:  types.UID("owner-uid"),
		},
	}
	child := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "child"}}

	require.NoError(t, StampOwnerRefOnCreate(context.Background(), zap.NewNop(), scheme, child, owner))

	refs := child.GetOwnerReferences()
	require.Len(t, refs, 1)
	require.Equal(t, "Pipeline", refs[0].Kind)
	require.Equal(t, types.UID("owner-uid"), refs[0].UID)
	require.NotNil(t, refs[0].Controller)
	require.True(t, *refs[0].Controller)
}

func TestStampOwnerRefOnCreateConflictIsNonFatal(t *testing.T) {
	scheme := testScheme(t)
	owner := &v2pb.Pipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", UID: types.UID("owner-uid")},
	}
	tr := true
	child := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "child",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v2pb.GroupVersion.String(),
				Kind:       "Pipeline",
				Name:       "other",
				UID:        types.UID("other-uid"),
				Controller: &tr,
			}},
		},
	}

	// AlreadyOwnedError must be swallowed (logged), returning nil so creation
	// is never broken.
	require.NoError(t, StampOwnerRefOnCreate(context.Background(), zap.NewNop(), scheme, child, owner))
	require.Len(t, child.GetOwnerReferences(), 1)
}

func TestStampSourcePipelineTypeLabelOnCreateHappyPath(t *testing.T) {
	child := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "child"}}

	StampSourcePipelineTypeLabelOnCreate(child, "PIPELINE_TYPE_TRAIN")

	require.Equal(t, "PIPELINE_TYPE_TRAIN", child.GetLabels()[notificationtypes.SourcePipelineTypeLabelName])
}

func TestStampSourcePipelineTypeLabelOnCreatePreservesExistingLabels(t *testing.T) {
	child := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "child",
			Labels: map[string]string{"other-label": "keep-me"},
		},
	}

	StampSourcePipelineTypeLabelOnCreate(child, "PIPELINE_TYPE_TRAIN")

	labels := child.GetLabels()
	require.Equal(t, "PIPELINE_TYPE_TRAIN", labels[notificationtypes.SourcePipelineTypeLabelName])
	require.Equal(t, "keep-me", labels["other-label"])
}

func TestStampSourcePipelineTypeLabelOnCreateEmptyTypeIsNoop(t *testing.T) {
	child := &v2pb.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "child"}}

	StampSourcePipelineTypeLabelOnCreate(child, "")

	require.Nil(t, child.GetLabels())
}
