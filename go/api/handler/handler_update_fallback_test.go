package handler

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	api "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// uidPreconditionConflict mirrors what a real Kubernetes API server returns when an
// update carries a UID but the object no longer exists in ETCD. The fake client does
// not emulate this, so tests inject it.
func uidPreconditionConflict(name string) error {
	return apiErrors.NewConflict(
		schema.GroupResource{Group: "michelangelo.api", Resource: "revisions"},
		name,
		&apiErrors.StatusError{ErrStatus: metav1.Status{
			Message: "StorageError: invalid object, Code: 4, Precondition failed: UID in precondition: 6fa91d7c, UID in object meta: ",
		}},
	)
}

func newFallbackTestHandler(t *testing.T, mockStorage storage.MetadataStorage, blobStorage storage.BlobStorage, initObjs []ctrlRTClient.Object,
	updateErr func(obj ctrlRTClient.Object) error) api.Handler {
	t.Helper()
	require.NoError(t, v2pb.AddToScheme(scheme.Scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(initObjs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c ctrlRTClient.WithWatch, obj ctrlRTClient.Object, opts ...ctrlRTClient.UpdateOption) error {
				return updateErr(obj)
			},
		}).
		Build()

	handler, err := NewAPIHandlerBuilder().
		WithK8sClient(k8sClient).
		WithMetadataStorage(mockStorage, storage.MetadataStorageConfig{EnableMetadataStorage: true}).
		WithBlobStorage(blobStorage).
		WithZapLogger(zap.Must(zap.NewDevelopment())).
		WithMetrics(tally.NoopScope).
		Build()
	require.NoError(t, err)
	return handler
}

func immutableRevision(name string) *v2pb.Revision {
	return &v2pb.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "default",
			Name:            name,
			UID:             "6fa91d7c",
			ResourceVersion: "42",
			Annotations: map[string]string{
				api.ImmutableAnnotation: "true",
				"example.com/stamp":     "v1",
			},
		},
		Spec: v2pb.RevisionSpec{Source: "Git"},
	}
}

// An object that only lives in metadata storage (evicted from ETCD) is read back with
// its UID; the K8s update then fails the UID precondition with a Conflict. Update must
// fall back to the metadata-storage direct update instead of surfacing the conflict.
func TestUpdate_UIDPreconditionConflictOnMissingObject_FallsBackToMetadataStorage(t *testing.T) {
	mockStorage, mockBlob := setupMocks(t)
	rev := immutableRevision("evicted-revision")
	h := newFallbackTestHandler(t, mockStorage, mockBlob, nil, func(obj ctrlRTClient.Object) error {
		return uidPreconditionConflict(obj.GetName())
	})

	mockStorage.EXPECT().
		Upsert(gomock.Any(), gomock.Any(), true, gomock.Any()).
		Times(1).
		DoAndReturn(func(_ context.Context, object runtime.Object, _ bool, _ []storage.IndexedField) error {
			metaObj := object.(metav1.Object)
			assert.Equal(t, "v1", metaObj.GetAnnotations()["example.com/stamp"])
			metaObj.SetResourceVersion("43")
			return nil
		})

	require.NoError(t, h.Update(context.Background(), rev, &metav1.UpdateOptions{}))
	assert.Equal(t, "43", rev.GetResourceVersion())
}

// A genuine resourceVersion conflict on an object that still exists in ETCD must still
// be surfaced as FailedPrecondition, without touching metadata storage.
func TestUpdate_ConflictOnExistingObject_IsSurfaced(t *testing.T) {
	mockStorage, mockBlob := setupMocks(t)
	existing := immutableRevision("live-revision")
	existing.ResourceVersion = ""
	h := newFallbackTestHandler(t, mockStorage, mockBlob, []ctrlRTClient.Object{existing}, func(obj ctrlRTClient.Object) error {
		return apiErrors.NewConflict(schema.GroupResource{Group: "michelangelo.api", Resource: "revisions"},
			obj.GetName(), assert.AnError)
	})

	mockStorage.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := h.Update(context.Background(), immutableRevision("live-revision"), &metav1.UpdateOptions{})
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// Errors other than NotFound/Conflict never fall back to metadata storage.
func TestUpdate_OtherK8sError_IsSurfaced(t *testing.T) {
	mockStorage, mockBlob := setupMocks(t)
	h := newFallbackTestHandler(t, mockStorage, mockBlob, nil, func(obj ctrlRTClient.Object) error {
		return apiErrors.NewBadRequest("boom")
	})

	mockStorage.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := h.Update(context.Background(), immutableRevision("bad-request-revision"), &metav1.UpdateOptions{})
	require.Error(t, err)
	assert.NotEqual(t, codes.OK, status.Code(err))
}
