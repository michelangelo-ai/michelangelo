package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// recordingK8sHandler counts the calls that reach the k8s layer and lets a test choose the answer to the scope query.
type recordingK8sHandler struct {
	K8sHandler
	calls         int
	namespaced    bool
	namespacedErr error
}

func (r *recordingK8sHandler) IsObjectNamespaced(runtime.Object) (bool, error) {
	return r.namespaced, r.namespacedErr
}

func (r *recordingK8sHandler) Create(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.CreateOptions) error {
	r.calls++
	return r.K8sHandler.Create(ctx, obj, opts)
}

func (r *recordingK8sHandler) Get(ctx context.Context, namespace, name string, obj ctrlRTClient.Object) error {
	r.calls++
	return r.K8sHandler.Get(ctx, namespace, name, obj)
}

func (r *recordingK8sHandler) Update(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	r.calls++
	return r.K8sHandler.Update(ctx, obj, opts)
}

func (r *recordingK8sHandler) UpdateStatus(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.UpdateOptions) error {
	r.calls++
	return r.K8sHandler.UpdateStatus(ctx, obj, opts)
}

func (r *recordingK8sHandler) Delete(ctx context.Context, obj ctrlRTClient.Object, opts *metav1.DeleteOptions) error {
	r.calls++
	return r.K8sHandler.Delete(ctx, obj, opts)
}

func newRecordingHandler(t *testing.T, namespaced bool, namespacedErr error) (*apiHandler, *recordingK8sHandler) {
	client, err := setupK8s()
	require.NoError(t, err)
	rec := &recordingK8sHandler{K8sHandler: NewK8sHandler(client), namespaced: namespaced, namespacedErr: namespacedErr}
	return &apiHandler{
		conf:              storage.MetadataStorageConfig{EnableMetadataStorage: false},
		logger:            zapr.NewLogger(zap.NewNop()),
		metrics:           tally.NoopScope,
		k8sHandler:        rec,
		metadataHandler:   NewMetadataHandler(nil, nil, zapr.NewLogger(zap.NewNop())),
		blobHandler:       NewBlobHandler(nil),
		validationHandler: NewValidationHandler(),
	}, rec
}

func rayJob(namespace, name string) *v2pb.RayJob {
	return &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       v2pb.RayJobSpec{JobId: "job"},
	}
}

func TestInvalidKeyReturnsInvalidArgument(t *testing.T) {
	ctx := context.Background()
	const (
		emptyName = "resource name may not be empty"
		emptyNS   = "an empty namespace may not be set when a resource name is provided"
		badName   = "invalid resource name"
	)
	tests := []struct {
		name    string
		call    func(h *apiHandler) error
		wantMsg string
	}{
		{
			name:    "get empty name",
			call:    func(h *apiHandler) error { return h.Get(ctx, "default", "", &metav1.GetOptions{}, &v2pb.RayJob{}) },
			wantMsg: "failed to get API object. namespace: default, name: : " + emptyName,
		},
		{
			name:    "get empty namespace",
			call:    func(h *apiHandler) error { return h.Get(ctx, "", "job01", &metav1.GetOptions{}, &v2pb.RayJob{}) },
			wantMsg: "failed to get API object. namespace: , name: job01: " + emptyNS,
		},
		{
			name:    "get name with slash",
			call:    func(h *apiHandler) error { return h.Get(ctx, "default", "a/b", &metav1.GetOptions{}, &v2pb.RayJob{}) },
			wantMsg: badName + ` "a/b"`,
		},
		{
			name:    "get name dot dot",
			call:    func(h *apiHandler) error { return h.Get(ctx, "default", "..", &metav1.GetOptions{}, &v2pb.RayJob{}) },
			wantMsg: badName + ` ".."`,
		},
		{
			name:    "get name with percent",
			call:    func(h *apiHandler) error { return h.Get(ctx, "default", "a%b", &metav1.GetOptions{}, &v2pb.RayJob{}) },
			wantMsg: badName + ` "a%b"`,
		},
		{
			name: "update empty name",
			call: func(h *apiHandler) error {
				return h.Update(ctx, rayJob("default", ""), &metav1.UpdateOptions{})
			},
			wantMsg: "failed to update API object. namespace: default, name: : " + emptyName,
		},
		{
			name: "update empty namespace",
			call: func(h *apiHandler) error {
				return h.Update(ctx, rayJob("", "job01"), &metav1.UpdateOptions{})
			},
			wantMsg: "failed to update API object. namespace: , name: job01: " + emptyNS,
		},
		{
			name: "update status empty name",
			call: func(h *apiHandler) error {
				return h.UpdateStatus(ctx, rayJob("default", ""), &metav1.UpdateOptions{})
			},
			wantMsg: "failed to updateStatus API object. namespace: default, name: : " + emptyName,
		},
		{
			name: "delete empty name",
			call: func(h *apiHandler) error {
				return h.Delete(ctx, rayJob("default", ""), &metav1.DeleteOptions{})
			},
			wantMsg: "failed to delete API object. namespace: default, name: : " + emptyName,
		},
		{
			name: "delete empty namespace",
			call: func(h *apiHandler) error {
				return h.Delete(ctx, rayJob("", "job01"), &metav1.DeleteOptions{})
			},
			wantMsg: "failed to delete API object. namespace: , name: job01: " + emptyNS,
		},
		{
			name: "create empty namespace",
			call: func(h *apiHandler) error {
				return h.Create(ctx, rayJob("", "job01"), &metav1.CreateOptions{})
			},
			wantMsg: "failed to create API object. namespace: , name: job01: an empty namespace may not be set during creation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, rec := newRecordingHandler(t, true, nil)
			err := tt.call(h)
			require.Error(t, err)
			s, ok := status.FromError(err)
			require.True(t, ok, "want a gRPC status error, got %T: %v", err, err)
			assert.Equal(t, codes.InvalidArgument, s.Code())
			assert.Contains(t, s.Message(), tt.wantMsg)
			assert.Zero(t, rec.calls, "the k8s layer must not be reached")
		})
	}
}

// An empty namespace is only invalid for namespaced kinds. When the kind is cluster-scoped, or its scope cannot be
// determined, the request must reach the k8s client exactly as it did before.
func TestEmptyNamespaceNotRejectedWhenScopeIsNotNamespaced(t *testing.T) {
	tests := []struct {
		name          string
		namespaced    bool
		namespacedErr error
	}{
		{name: "cluster-scoped kind", namespaced: false},
		{name: "scope unknown", namespaced: true, namespacedErr: errors.New("no REST mapping")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, rec := newRecordingHandler(t, tt.namespaced, tt.namespacedErr)
			err := h.Get(context.Background(), "", "job01", &metav1.GetOptions{}, &v2pb.RayJob{})
			require.Error(t, err)
			s, _ := status.FromError(err)
			assert.NotEqual(t, codes.InvalidArgument, s.Code(), "got %v", err)
			assert.Equal(t, 1, rec.calls, "the request must reach the k8s layer")
		})
	}
}

// A K8sHandler that cannot report scope (no IsObjectNamespaced) must behave as before.
func TestEmptyNamespacePassesThroughWithoutScoper(t *testing.T) {
	client, err := setupK8s()
	require.NoError(t, err)
	h := NewFakeAPIHandler(client).(*apiHandler)
	h.k8sHandler = struct{ K8sHandler }{NewK8sHandler(client)}

	err = h.Get(context.Background(), "", "job01", &metav1.GetOptions{}, &v2pb.RayJob{})
	require.Error(t, err)
	s, _ := status.FromError(err)
	assert.NotEqual(t, codes.InvalidArgument, s.Code(), "got %v", err)
}

func TestValidKeysAreUnaffected(t *testing.T) {
	ctx := context.Background()
	client, err := setupK8s()
	require.NoError(t, err)
	h := NewFakeAPIHandler(client)

	// generateName with an empty name is a valid create.
	generated := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", GenerateName: "gen-"},
		Spec:       v2pb.RayJobSpec{JobId: "gen"},
	}
	require.NoError(t, h.Create(ctx, generated, &metav1.CreateOptions{}))
	assert.NotEmpty(t, generated.Name)

	// get, update and delete on an existing object.
	got := &v2pb.RayJob{}
	require.NoError(t, h.Get(ctx, "default", "job01", &metav1.GetOptions{}, got))
	require.NoError(t, h.Update(ctx, got, &metav1.UpdateOptions{}))
	require.NoError(t, h.Delete(ctx, got, &metav1.DeleteOptions{}))

	// a well-formed key that does not exist is still NotFound.
	err = h.Get(ctx, "default", "missing", &metav1.GetOptions{}, &v2pb.RayJob{})
	checkGrpcStatusCode(t, codes.NotFound, err)
}

// surfaceGrpcError must map every error that is not an empty or invalid key exactly as it did before.
func TestSurfaceGrpcErrorMapping(t *testing.T) {
	gr := schema.GroupResource{Group: "michelangelo.uber.com", Resource: "rayjobs"}
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantMsg  string
	}{
		{name: "nil", err: nil, wantCode: codes.OK},
		{name: "k8s not found", err: apiErrors.NewNotFound(gr, "x"), wantCode: codes.NotFound},
		{name: "k8s already exists", err: apiErrors.NewAlreadyExists(gr, "x"), wantCode: codes.AlreadyExists},
		{name: "k8s conflict", err: apiErrors.NewConflict(gr, "x", errors.New("stale")), wantCode: codes.FailedPrecondition},
		{name: "k8s forbidden", err: apiErrors.NewForbidden(gr, "x", errors.New("no")), wantCode: codes.PermissionDenied},
		{
			name:     "grpc status keeps its code",
			err:      status.Error(codes.ResourceExhausted, "slow down"),
			wantCode: codes.ResourceExhausted,
		},
		{name: "plain error", err: errors.New("boom"), wantCode: codes.Unknown, wantMsg: "boom"},
		{
			name:     "dial failure is still Unknown",
			err:      errors.New(`Get "https://k8s/apis/x": failed to dial API server`),
			wantCode: codes.Unknown,
			wantMsg:  "failed to dial API server",
		},
		{name: "unexpected EOF is still Unknown", err: errors.New("unexpected EOF"), wantCode: codes.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := surfaceGrpcError(tt.err, "get", "ns", "n")
			if tt.err == nil {
				assert.NoError(t, got)
				return
			}
			s, ok := status.FromError(got)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, s.Code())
			assert.Contains(t, s.Message(), "failed to get API object. namespace: ns, name: n: ")
			if tt.wantMsg != "" {
				assert.Contains(t, s.Message(), tt.wantMsg)
			}
		})
	}
}
