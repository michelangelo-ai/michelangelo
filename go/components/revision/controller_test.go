package revision

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	apimocks "github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

type mockRevisionHandler struct {
	typeMeta    metav1.TypeMeta
	reconcileFn func(ctx context.Context, rev *v2pb.Revision) (ctrl.Result, error)
}

func (m *mockRevisionHandler) TypeMeta() metav1.TypeMeta { return m.typeMeta }
func (m *mockRevisionHandler) Reconcile(ctx context.Context, rev *v2pb.Revision) (ctrl.Result, error) {
	return m.reconcileFn(ctx, rev)
}

// pipelineTypeMeta is the dispatch key shared between handler registration and
// BaseType assertions. All tests that register a pipeline handler use this so
// the key is guaranteed to agree on both sides.
var pipelineTypeMeta = metav1.TypeMeta{
	APIVersion: "michelangelo.api/v2",
	Kind:       "Pipeline",
}

func newTestReconciler(t *testing.T, apiHandler *apimocks.MockHandler, handlers ...Handler) *Reconciler {
	t.Helper()
	m := make(map[metav1.TypeMeta]Handler, len(handlers))
	for _, h := range handlers {
		m[h.TypeMeta()] = h
	}
	return &Reconciler{
		Handler:  apiHandler,
		logger:   zaptest.NewLogger(t),
		handlers: m,
	}
}

func TestReconcile_GetNotFound(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	r := newTestReconciler(t, mockAPI)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_GetOtherError(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	sentinel := errors.New("internal error")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		Return(sentinel)

	r := newTestReconciler(t, mockAPI)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.ErrorIs(t, err, sentinel)
}

func TestReconcile_DeletionTimestampSet(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	now := metav1.Now()
	gracePeriod := int64(0)
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.DeletionTimestamp = &now
			obj.DeletionGracePeriodSeconds = &gracePeriod
			obj.Finalizers = []string{"test-finalizer"}
			return nil
		})

	r := newTestReconciler(t, mockAPI)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_BaseTypeNil(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = nil
			return nil
		})

	r := newTestReconciler(t, mockAPI)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_NoHandlerRegistered_NoOp(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &metav1.TypeMeta{APIVersion: "other/v1", Kind: "Other"}
			return nil
		})
	// No UpdateStatus call expected — unregistered BaseType leaves status unchanged.

	r := newTestReconciler(t, mockAPI)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_HandlerError(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	handlerErr := errors.New("handler failure")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &pipelineTypeMeta
			return nil
		})

	h := &mockRevisionHandler{
		typeMeta:    pipelineTypeMeta,
		reconcileFn: func(_ context.Context, _ *v2pb.Revision) (ctrl.Result, error) { return ctrl.Result{}, handlerErr },
	}

	r := newTestReconciler(t, mockAPI, h)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.ErrorIs(t, err, handlerErr)
}

func TestReconcile_StatusChanged_CallsUpdateStatus(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &pipelineTypeMeta
			obj.Status.State = v2pb.REVISION_STATE_CREATED
			return nil
		})
	mockAPI.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	h := &mockRevisionHandler{
		typeMeta: pipelineTypeMeta,
		reconcileFn: func(_ context.Context, rev *v2pb.Revision) (ctrl.Result, error) {
			rev.Status.State = v2pb.REVISION_STATE_READY
			return ctrl.Result{}, nil
		},
	}

	r := newTestReconciler(t, mockAPI, h)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_StatusUnchanged_SkipsUpdateStatus(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &pipelineTypeMeta
			obj.Status.State = v2pb.REVISION_STATE_CREATED
			return nil
		})
	// UpdateStatus must NOT be called — no EXPECT for it.

	h := &mockRevisionHandler{
		typeMeta: pipelineTypeMeta,
		reconcileFn: func(_ context.Context, _ *v2pb.Revision) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
	}

	r := newTestReconciler(t, mockAPI, h)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_ImmutableSkipped(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Annotations = map[string]string{"michelangelo/Immutable": "true"}
			obj.Spec.BaseType = &pipelineTypeMeta
			return nil
		})

	h := &mockRevisionHandler{
		typeMeta: pipelineTypeMeta,
		reconcileFn: func(_ context.Context, _ *v2pb.Revision) (ctrl.Result, error) {
			t.Fatal("handler should not be called for immutable revision")
			return ctrl.Result{}, nil
		},
	}

	r := newTestReconciler(t, mockAPI, h)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_TerminalState_MarksImmutable(t *testing.T) {
	for _, terminalState := range []v2pb.RevisionState{v2pb.REVISION_STATE_READY, v2pb.REVISION_STATE_ERROR} {
		t.Run(terminalState.String(), func(t *testing.T) {
			mc := gomock.NewController(t)
			defer mc.Finish()

			mockAPI := apimocks.NewMockHandler(mc)
			mockAPI.EXPECT().
				Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
					obj.Status.State = terminalState
					obj.Spec.BaseType = &pipelineTypeMeta
					return nil
				})
			mockAPI.EXPECT().
				Update(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, obj *v2pb.Revision, _ *metav1.UpdateOptions) error {
					require.Equal(t, "true", obj.Annotations["michelangelo/Immutable"])
					return nil
				})

			h := &mockRevisionHandler{
				typeMeta: pipelineTypeMeta,
				reconcileFn: func(_ context.Context, _ *v2pb.Revision) (ctrl.Result, error) {
					t.Fatal("handler should not be called for terminal-state revision")
					return ctrl.Result{}, nil
				},
			}

			r := newTestReconciler(t, mockAPI, h)
			result, err := r.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
			})
			require.NoError(t, err)
			require.Equal(t, ctrl.Result{}, result)
		})
	}
}

func TestReconcile_HandlerError_PersistsStatus(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	handlerErr := errors.New("image build failed")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &pipelineTypeMeta
			obj.Status.State = v2pb.REVISION_STATE_INVALID
			return nil
		})
	mockAPI.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	h := &mockRevisionHandler{
		typeMeta: pipelineTypeMeta,
		reconcileFn: func(_ context.Context, rev *v2pb.Revision) (ctrl.Result, error) {
			rev.Status.State = v2pb.REVISION_STATE_BUILDING
			return ctrl.Result{}, handlerErr
		},
	}

	r := newTestReconciler(t, mockAPI, h)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.ErrorIs(t, err, handlerErr)
}

func TestReconcile_HandlerResult_PassedThrough(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-rev", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Revision) error {
			obj.Spec.BaseType = &pipelineTypeMeta
			obj.Status.State = v2pb.REVISION_STATE_CREATED
			return nil
		})

	expected := ctrl.Result{RequeueAfter: 10}
	h := &mockRevisionHandler{
		typeMeta: pipelineTypeMeta,
		reconcileFn: func(_ context.Context, _ *v2pb.Revision) (ctrl.Result, error) {
			return expected, nil
		},
	}

	r := newTestReconciler(t, mockAPI, h)
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-rev"},
	})
	require.NoError(t, err)
	require.Equal(t, expected, result)
}
