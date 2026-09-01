package apihook

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestBeforeCreate_RejectsOverlongDescription(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	longDesc := strings.Repeat("a", maxDescriptionLength+1)
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			Spec: v2.ModelSpec{Description: longDesc},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "model description exceeds maximum length")
}

func TestBeforeCreate_AcceptsDescriptionAtMaxLength(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	exactDesc := strings.Repeat("a", maxDescriptionLength)
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			Spec: v2.ModelSpec{Description: exactDesc},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
}

func TestBeforeUpdate_RejectsOverlongDescription(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	longDesc := strings.Repeat("a", maxDescriptionLength+1)
	request := &v2.UpdateModelRequest{
		Model: &v2.Model{
			Spec: v2.ModelSpec{Description: longDesc},
		},
	}

	err := hook.BeforeUpdate(context.Background(), request)

	assert.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestBeforeCreate_DefaultsEnvironmentLabelWhenAbsent(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	request := &v2.CreateModelRequest{Model: &v2.Model{}}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "staging", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_DefaultsToUnspecifiedWhenUnconfigured(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: ""}
	request := &v2.CreateModelRequest{Model: &v2.Model{}}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "unspecified", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_InheritsFromSourcePipelineRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "test-namespace", "source-run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.ObjectMeta.Labels = map[string]string{"michelangelo/environment": "staging"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "production"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "test-namespace", Name: "source-run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "staging", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_PreservesExplicitEnvironmentLabel(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "production"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"michelangelo/environment": "staging"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "staging", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_SourcePipelineRunNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "test-namespace", "source-run", gomock.Any(), gomock.Any()).
		Return(apiErrors.NewNotFound(schema.GroupResource{Resource: "pipelineruns"}, "source-run"))

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "production"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "test-namespace", Name: "source-run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "production", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_SourcePipelineRunGetErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "test-namespace", "source-run", gomock.Any(), gomock.Any()).
		Return(assert.AnError)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "production"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "test-namespace", Name: "source-run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	// Non-not-found Get errors are swallowed (logged as a warning) rather than
	// propagated: this is a best-effort label-inheritance lookup and must not
	// block Model creation. See package doc / applyEnvironmentLabel comment.
	assert.NoError(t, err)
	assert.Equal(t, "production", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeCreate_SourcePipelineRunHasNoEnvironmentLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "test-namespace", "source-run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.ObjectMeta.Labels = map[string]string{}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "production"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "test-namespace", Name: "source-run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "production", request.Model.Labels["michelangelo/environment"])
}

func TestBeforeUpdate_MirrorsBeforeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "test-namespace", "source-run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.ObjectMeta.Labels = map[string]string{"michelangelo/environment": "staging"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "production"}
	request := &v2.UpdateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "test-namespace", Name: "source-run"},
			},
		},
	}

	err := hook.BeforeUpdate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "staging", request.Model.Labels["michelangelo/environment"])
}

func TestRegisterModelAPIHook(t *testing.T) {
	RegisterModelAPIHook(zap.NewNop(), nil, "production")
}
