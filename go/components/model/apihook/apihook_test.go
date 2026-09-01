package apihook

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ---------------------------------------------------------------------------
// EnvironmentLabel tests (unchanged from PR #1912)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// SourcePipelineTypeLabel tests (Behavior #2)
// ---------------------------------------------------------------------------

func TestBeforeCreate_DefaultsPipelineTypeLabelWhenAbsent(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	request := &v2.CreateModelRequest{Model: &v2.Model{}}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, api.DefaultSourcePipelineType, request.Model.Labels[api.SourcePipelineTypeLabelName])
}

func TestBeforeCreate_InheritsPipelineTypeLabelFromSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.ObjectMeta.Labels = map[string]string{
				api.SourcePipelineTypeLabelName: "PIPELINE_TYPE_EVAL",
			}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "PIPELINE_TYPE_EVAL", request.Model.Labels[api.SourcePipelineTypeLabelName])
}

func TestBeforeCreate_PreservesExplicitPipelineTypeLabel(t *testing.T) {
	hook := apiHook{logger: zap.NewNop(), defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{api.SourcePipelineTypeLabelName: "PIPELINE_TYPE_PREDICTION"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "PIPELINE_TYPE_PREDICTION", request.Model.Labels[api.SourcePipelineTypeLabelName])
}

func TestBeforeCreate_PipelineTypeLabelKeepsDefaultWhenSourceLacks(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.ObjectMeta.Labels = map[string]string{}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, api.DefaultSourcePipelineType, request.Model.Labels[api.SourcePipelineTypeLabelName])
}

// ---------------------------------------------------------------------------
// Owner/actor copy tests (Behavior #3)
// ---------------------------------------------------------------------------

func TestBeforeCreate_CopiesOwnerFromSourceActor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Actor = &v2.UserInfo{Name: "alice", ProxyUser: "bob"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "alice", request.Model.Spec.GetOwner().GetName())
	assert.Equal(t, "bob", request.Model.Spec.GetOwner().GetProxyUser())
}

func TestBeforeCreate_DoesNotOverwriteOwnerWhenActorEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				Owner:             &v2.UserInfo{Name: "existing-owner"},
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "existing-owner", request.Model.Spec.GetOwner().GetName())
}

func TestBeforeCreate_CopiesPartialActorFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Actor = &v2.UserInfo{Name: "alice"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				Owner:             &v2.UserInfo{ProxyUser: "keep-me"},
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "alice", request.Model.Spec.GetOwner().GetName())
	assert.Equal(t, "keep-me", request.Model.Spec.GetOwner().GetProxyUser())
}

// ---------------------------------------------------------------------------
// Pipeline-name / revision label copy tests (Behavior #4)
// ---------------------------------------------------------------------------

func TestBeforeCreate_CopiesPipelineNameLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Pipeline = &apipb.ResourceIdentifier{Name: "my-pipeline"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "my-pipeline", request.Model.Labels[api.ModelSourcePipelineName])
}

func TestBeforeCreate_NoPipelineNameLabelWhenPipelineAbsent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	_, exists := request.Model.Labels[api.ModelSourcePipelineName]
	assert.False(t, exists)
}

func TestBeforeCreate_CopiesRevisionIdLabel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.AssignableToTypeOf(&v2.PipelineRun{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Revision = &apipb.ResourceIdentifier{Name: "rev-1", Namespace: "ns"}
			return nil
		})

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "rev-1", gomock.Any(), gomock.AssignableToTypeOf(&v2.Revision{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			rev := obj.(*v2.Revision)
			rev.Spec.RevisionId = "abc123"
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "abc123", request.Model.Labels[api.ModelSourcePipelineRevision])
}

func TestBeforeCreate_FallsBackToGitRefWhenRevisionIdEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.AssignableToTypeOf(&v2.PipelineRun{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Revision = &apipb.ResourceIdentifier{Name: "rev-2", Namespace: "ns"}
			return nil
		})

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "rev-2", gomock.Any(), gomock.AssignableToTypeOf(&v2.Revision{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			rev := obj.(*v2.Revision)
			rev.Spec.GitCommit = &v2.CommitInfo{GitRef: "refs/heads/main"}
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	assert.Equal(t, "refs/heads/main", request.Model.Labels[api.ModelSourcePipelineRevision])
}

func TestBeforeCreate_RevisionGetFailureIsNonFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.AssignableToTypeOf(&v2.PipelineRun{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			run := obj.(*v2.PipelineRun)
			run.Spec.Revision = &apipb.ResourceIdentifier{Name: "rev-gone", Namespace: "ns"}
			return nil
		})

	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "rev-gone", gomock.Any(), gomock.AssignableToTypeOf(&v2.Revision{})).
		Return(assert.AnError)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	_, exists := request.Model.Labels[api.ModelSourcePipelineRevision]
	assert.False(t, exists)
}

func TestBeforeCreate_NoRevisionLabelWhenRevisionAbsent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), "ns", "run", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			return nil
		})

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler, defaultEnv: "staging"}
	request := &v2.CreateModelRequest{
		Model: &v2.Model{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
			Spec: v2.ModelSpec{
				SourcePipelineRun: &apipb.ResourceIdentifier{Namespace: "ns", Name: "run"},
			},
		},
	}

	err := hook.BeforeCreate(context.Background(), request)

	assert.NoError(t, err)
	_, exists := request.Model.Labels[api.ModelSourcePipelineRevision]
	assert.False(t, exists)
}
