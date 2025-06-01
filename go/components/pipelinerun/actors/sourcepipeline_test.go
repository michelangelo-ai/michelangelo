package actors

import (
	"context"
	"testing"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSourcePipelineActor(t *testing.T) {
	testCases := []struct {
		name                   string
		pipelineRun            v2.PipelineRun
		initialObjects         []runtime.Object
		expectedCondition      *apipb.Condition
		expectedSourcePipeline *v2.SourcePipeline
		errMsg                 string
	}{
		{
			name: "pipeline run with nil pipeline condition",
			pipelineRun: v2.PipelineRun{
				Spec: v2.PipelineRunSpec{
					Pipeline: &apipb.ResourceIdentifier{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			initialObjects: []runtime.Object{
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_UNKNOWN,
			},
			expectedSourcePipeline: nil,
			errMsg:                 "",
		},
		{
			name: "pipeline run without resource id",
			pipelineRun: v2.PipelineRun{
				Spec: v2.PipelineRunSpec{},
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   SourcePipelineType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			initialObjects: []runtime.Object{
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedSourcePipeline: nil,
			errMsg:                 "pipeline resource ID is nil",
		},
		{
			name: "pipeline run with pipeline resource ID",
			pipelineRun: v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
				Spec: v2.PipelineRunSpec{
					Pipeline: &apipb.ResourceIdentifier{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   SourcePipelineType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			initialObjects: []runtime.Object{
				&v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedSourcePipeline: &v2.SourcePipeline{
				Pipeline: &v2.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
			},
			errMsg: "",
		},
		{
			name: "pipeline run with source pipeline not found",
			pipelineRun: v2.PipelineRun{
				Spec: v2.PipelineRunSpec{
					Pipeline: &apipb.ResourceIdentifier{
						Name:      "test-pipeline",
						Namespace: "test-namespace",
					},
				},
				Status: v2.PipelineRunStatus{
					Conditions: []*apipb.Condition{
						{
							Type:   SourcePipelineType,
							Status: apipb.CONDITION_STATUS_UNKNOWN,
						},
					},
				},
			},
			initialObjects: []runtime.Object{},
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			},
			expectedSourcePipeline: nil,
			errMsg:                 "failed to get pipeline",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actor := setUpSourcePipelineActor(t, testCase.initialObjects)
			previousCondition := conditionUtils.GetCondition(SourcePipelineType, testCase.pipelineRun.Status.Conditions)
			condition, err := actor.Run(context.Background(), &testCase.pipelineRun, previousCondition)
			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedCondition, condition)
			}
			if testCase.expectedSourcePipeline != nil {
				require.Equal(t, testCase.expectedSourcePipeline.Pipeline.Name, testCase.pipelineRun.Status.SourcePipeline.Pipeline.Name)
				require.Equal(t, testCase.expectedSourcePipeline.Pipeline.Namespace, testCase.pipelineRun.Status.SourcePipeline.Pipeline.Namespace)
			} else {
				require.Nil(t, testCase.pipelineRun.Status.SourcePipeline)
			}
		})
	}
}

func setUpSourcePipelineActor(t *testing.T, initialObjects []runtime.Object) *SourcePipelineActor {
	scheme := runtime.NewScheme()
	err := v2.AddToScheme(scheme)
	require.NoError(t, err)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(initialObjects...).Build()
	apiHandler := apiHandler.NewFakeAPIHandler(k8sClient)
	return NewSourcePipelineActor(apiHandler, zaptest.NewLogger(t))
}
