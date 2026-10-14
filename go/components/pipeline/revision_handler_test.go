package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	revisionctrl "github.com/michelangelo-ai/michelangelo/go/components/revision"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestRevisionHandler_TypeMeta(t *testing.T) {
	h := NewRevisionHandler(nil, zaptest.NewLogger(t))
	assert.Equal(t, metav1.TypeMeta{APIVersion: "michelangelo.api/v2", Kind: "Pipeline"}, h.TypeMeta())
}

func TestRevisionHandler_Reconcile(t *testing.T) {
	pipelineWithState := func(namespace string, state v2pb.PipelineState) *v2pb.Pipeline {
		return &v2pb.Pipeline{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline", Namespace: namespace},
			Status:     v2pb.PipelineStatus{State: state},
		}
	}
	baseResource := &apipb.ResourceIdentifier{Name: "test-pipeline", Namespace: "test-namespace"}

	testCases := []struct {
		name           string
		initialObjects []client.Object
		baseResource   *apipb.ResourceIdentifier
		initialState   v2pb.RevisionState
		expectedState  v2pb.RevisionState
		expectedResult ctrl.Result
	}{
		{
			name:           "pipeline READY -> revision READY",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_READY)},
			baseResource:   baseResource,
			expectedState:  v2pb.REVISION_STATE_READY,
		},
		{
			name:           "pipeline ERROR -> revision ERROR",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_ERROR)},
			baseResource:   baseResource,
			expectedState:  v2pb.REVISION_STATE_ERROR,
		},
		{
			name:           "pipeline INVALID -> revision ERROR",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_INVALID)},
			baseResource:   baseResource,
			expectedState:  v2pb.REVISION_STATE_ERROR,
		},
		{
			name:           "pipeline CREATED -> revision CREATED with requeue",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_CREATED)},
			baseResource:   baseResource,
			expectedState:  v2pb.REVISION_STATE_CREATED,
			expectedResult: ctrl.Result{RequeueAfter: reconcileInterval},
		},
		{
			name:           "pipeline BUILDING -> revision BUILDING with requeue",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_BUILDING)},
			baseResource:   baseResource,
			initialState:   v2pb.REVISION_STATE_CREATED,
			expectedState:  v2pb.REVISION_STATE_BUILDING,
			expectedResult: ctrl.Result{RequeueAfter: reconcileInterval},
		},
		{
			name:           "base namespace defaults to revision namespace",
			initialObjects: []client.Object{pipelineWithState("test-namespace", v2pb.PIPELINE_STATE_READY)},
			baseResource:   &apipb.ResourceIdentifier{Name: "test-pipeline"},
			expectedState:  v2pb.REVISION_STATE_READY,
		},
		{
			name:          "missing base pipeline -> revision ERROR",
			baseResource:  baseResource,
			expectedState: v2pb.REVISION_STATE_ERROR,
		},
		{
			name:          "nil base resource -> revision ERROR",
			expectedState: v2pb.REVISION_STATE_ERROR,
		},
		{
			name:          "empty base resource name -> revision ERROR",
			baseResource:  &apipb.ResourceIdentifier{Name: " ", Namespace: "test-namespace"},
			expectedState: v2pb.REVISION_STATE_ERROR,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v2pb.AddToScheme(scheme))
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.initialObjects...).Build()
			h := NewRevisionHandler(apiHandler.NewFakeAPIHandler(k8sClient), zaptest.NewLogger(t))

			rev := &v2pb.Revision{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-test-pipeline-abc123", Namespace: "test-namespace"},
				Spec: v2pb.RevisionSpec{
					BaseType:     &metav1.TypeMeta{APIVersion: pipelineAPIVersion, Kind: pipelineKind},
					BaseResource: tc.baseResource,
				},
				Status: v2pb.RevisionStatus{State: tc.initialState},
			}

			result, err := h.Reconcile(context.Background(), rev)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, result)
			assert.Equal(t, tc.expectedState, rev.Status.State)
		})
	}
}

// stubManager satisfies manager.Manager for FX wiring tests; only GetClient is used.
type stubManager struct {
	manager.Manager
	client client.Client
}

func (m *stubManager) GetClient() client.Client { return m.client }

func TestModule_ProvidesRevisionHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	factory := handlermocks.NewMockFactory(mockCtrl)
	factory.EXPECT().GetAPIHandler(gomock.Any()).Return(apiHandler.NewFakeAPIHandler(nil), nil)

	var handlers []revisionctrl.Handler
	app := fx.New(
		fx.NopLogger,
		fx.Supply(fx.Annotate(&stubManager{}, fx.As(new(manager.Manager)))),
		fx.Supply(fx.Annotate(factory, fx.As(new(apiHandler.Factory)))),
		fx.Supply(zaptest.NewLogger(t)),
		revisionHandlerProvider,
		fx.Invoke(fx.Annotate(func(hs []revisionctrl.Handler) { handlers = hs }, fx.ParamTags(`group:"revision-handler"`))),
	)
	require.NoError(t, app.Err())
	require.Len(t, handlers, 1)
	assert.Equal(t, metav1.TypeMeta{APIVersion: pipelineAPIVersion, Kind: pipelineKind}, handlers[0].TypeMeta())
}

func TestNewRevisionHandler_FactoryError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	factory := handlermocks.NewMockFactory(mockCtrl)
	factory.EXPECT().GetAPIHandler(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := newRevisionHandler(&stubManager{}, factory, zaptest.NewLogger(t))
	require.EqualError(t, err, "boom")
}
