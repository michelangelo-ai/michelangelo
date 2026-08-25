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
	"k8s.io/apimachinery/pkg/runtime"

	mlapi "github.com/michelangelo-ai/michelangelo/go/api"
	apimocks "github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))
	return scheme
}

func TestBeforeCreateStampsSourcePipelineTypeLabel(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-pipeline", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2.Pipeline) error {
			obj.Name = "test-pipeline"
			obj.Namespace = "test-ns"
			obj.Spec = v2.PipelineSpec{Type: v2.PIPELINE_TYPE_TRAIN}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.PipelineRunSpec{
				Pipeline: &api.ResourceIdentifier{Name: "test-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, "PIPELINE_TYPE_TRAIN",
		request.PipelineRun.GetLabels()[mlapi.SourcePipelineTypeLabelName])
}

func TestBeforeCreatePipelineNotFoundDoesNotFailCreation(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.PipelineRunSpec{
				Pipeline: &api.ResourceIdentifier{Name: "missing-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Empty(t, request.PipelineRun.GetLabels())
}

func TestBeforeCreatePipelineLookupErrorDoesNotFailCreation(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("internal error"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.PipelineRunSpec{
				Pipeline: &api.ResourceIdentifier{Name: "test-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Empty(t, request.PipelineRun.GetLabels())
}

func TestBeforeCreateNoPipelineRefIsNoop(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	// No Get call is expected: BeforeCreate must return early without hitting the API.
	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreatePipelineRunRequest{
		PipelineRun: &v2.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.PipelineRunSpec{},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Empty(t, request.PipelineRun.GetLabels())
}
