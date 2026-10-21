package apihook

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

	apimocks "github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func runInState(name string, state v2.TriggerRunState) v2.TriggerRun {
	return v2.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Status:     v2.TriggerRunStatus{State: state},
	}
}

func TestBeforeDeleteOnlyTerminalTriggerRunsCanBeDeleted(t *testing.T) {
	tests := []struct {
		state    v2.TriggerRunState
		rejected bool
	}{
		{v2.TRIGGER_RUN_STATE_INVALID, true},
		{v2.TRIGGER_RUN_STATE_RUNNING, true},
		{v2.TRIGGER_RUN_STATE_PENDING_KILL, true},
		{v2.TRIGGER_RUN_STATE_PAUSED, true},
		{v2.TRIGGER_RUN_STATE_FAILED, false},
		{v2.TRIGGER_RUN_STATE_KILLED, false},
		{v2.TRIGGER_RUN_STATE_SUCCEEDED, false},
	}
	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			mc := gomock.NewController(t)
			defer mc.Finish()

			mockAPI := apimocks.NewMockHandler(mc)
			mockAPI.EXPECT().
				Get(gomock.Any(), "test-ns", "run", gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2.TriggerRun) error {
					*obj = runInState("run", tt.state)
					return nil
				})

			h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
			err := h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "run"})

			if !tt.rejected {
				require.NoError(t, err)
				return
			}
			require.Equal(t, codes.FailedPrecondition, status.Code(err))
			require.ErrorContains(t, err, "test-ns/run is "+tt.state.String())
			require.ErrorContains(t, err, "TRIGGER_RUN_ACTION_KILL")
		})
	}
}

func TestBeforeDeleteNotFoundIsLeftToTheDelete(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.NoError(t, h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "missing"}))
}

func TestBeforeDeleteLookupErrorIsReturned(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	lookupErr := errors.New("internal error")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(lookupErr)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.ErrorIs(t, h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "run"}), lookupErr)
}

func TestBeforeDeleteCollectionHonoursListOptions(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), "test-ns", gomock.Any(), gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts *metav1.ListOptions, _ *api.ListOptionsExt, list *v2.TriggerRunList) error {
			require.Equal(t, "team=ml", opts.LabelSelector, "the request's selector scopes the check")
			list.Items = []v2.TriggerRun{
				runInState("done", v2.TRIGGER_RUN_STATE_SUCCEEDED),
				runInState("stopped", v2.TRIGGER_RUN_STATE_KILLED),
			}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.NoError(t, h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{
		Namespace:   "test-ns",
		ListOptions: &metav1.ListOptions{LabelSelector: "team=ml"},
	}))
}

func TestBeforeDeleteCollectionRejectsIfAnySelectedTriggerRunIsLive(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), "test-ns", gomock.Any(), gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts *metav1.ListOptions, _ *api.ListOptionsExt, list *v2.TriggerRunList) error {
			require.NotNil(t, opts, "absent list options are replaced by an empty set")
			list.Items = []v2.TriggerRun{
				runInState("done", v2.TRIGGER_RUN_STATE_SUCCEEDED),
				runInState("live", v2.TRIGGER_RUN_STATE_RUNNING),
			}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	err := h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{Namespace: "test-ns"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.ErrorContains(t, err, "test-ns/live is TRIGGER_RUN_STATE_RUNNING")
}

func TestBeforeDeleteCollectionListErrorIsReturned(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	listErr := errors.New("internal error")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(listErr)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.ErrorIs(t, h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{Namespace: "test-ns"}), listErr)
}
