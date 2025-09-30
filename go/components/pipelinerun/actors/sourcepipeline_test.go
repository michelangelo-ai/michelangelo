package actors

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	pbtypes "github.com/gogo/protobuf/types"
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
			errMsg:                 "get pipeline",
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

func TestSourcePipelineActor_DevRun(t *testing.T) {
	testCases := []struct {
		name                       string
		pipelineRun                v2.PipelineRun
		expectedCondition          *apipb.Condition
		expectedSourcePipelineName string
		errMsg                     string
	}{
		{
			name: "dev run with inline pipeline_spec",
			pipelineRun: v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-devrun",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"michelangelo/uniflow-image": "test-image:latest",
					},
				},
				Spec: v2.PipelineRunSpec{
					PipelineSpec: &v2.PipelineSpec{
						Type: v2.PIPELINE_TYPE_TRAIN,
						Manifest: &v2.PipelineManifest{
							Type:            v2.PIPELINE_MANIFEST_TYPE_UNIFLOW,
							FilePath:        "test.pipeline",
							UniflowTar:      "s3://bucket/test.tar.gz",
							UniflowFunction: "test_workflow",
							Content: createTestManifestContent(t, map[string]interface{}{
								"environ": map[string]interface{}{
									"BASE_VAR": "base_value",
								},
							}),
						},
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
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedSourcePipelineName: "devrun-test-devrun",
		},
		{
			name: "dev run with environment variable overrides",
			pipelineRun: v2.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-devrun-env",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"michelangelo/uniflow-image": "test-image:latest",
					},
				},
				Spec: v2.PipelineRunSpec{
					Input: &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							"environ": {
								Kind: &pbtypes.Value_StructValue{
									StructValue: &pbtypes.Struct{
										Fields: map[string]*pbtypes.Value{
											"foo": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "bar",
												},
											},
											"lorem": {
												Kind: &pbtypes.Value_StringValue{
													StringValue: "ipsum",
												},
											},
											"number_var": {
												Kind: &pbtypes.Value_NumberValue{
													NumberValue: 42,
												},
											},
											"bool_var": {
												Kind: &pbtypes.Value_BoolValue{
													BoolValue: true,
												},
											},
										},
									},
								},
							},
						},
					},
					PipelineSpec: &v2.PipelineSpec{
						Type: v2.PIPELINE_TYPE_TRAIN,
						Manifest: &v2.PipelineManifest{
							Type:            v2.PIPELINE_MANIFEST_TYPE_UNIFLOW,
							FilePath:        "test.pipeline",
							UniflowTar:      "s3://bucket/test.tar.gz",
							UniflowFunction: "test_workflow",
							Content: createTestManifestContent(t, map[string]interface{}{
								"environ": map[string]interface{}{
									"BASE_VAR": "base_value",
									"foo":      "original_value", // Should be overridden
								},
							}),
						},
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
			expectedCondition: &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
			expectedSourcePipelineName: "devrun-test-devrun-env",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actor := setUpSourcePipelineActor(t, []runtime.Object{})
			previousCondition := conditionUtils.GetCondition(SourcePipelineType, testCase.pipelineRun.Status.Conditions)
			condition, err := actor.Run(context.Background(), &testCase.pipelineRun, previousCondition)

			if testCase.errMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.errMsg)
				require.Equal(t, testCase.expectedCondition, condition)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedCondition, condition)

				// Verify source pipeline was created
				require.NotNil(t, testCase.pipelineRun.Status.SourcePipeline)
				require.NotNil(t, testCase.pipelineRun.Status.SourcePipeline.Pipeline)
				require.Equal(t, testCase.expectedSourcePipelineName, testCase.pipelineRun.Status.SourcePipeline.Pipeline.Name)
				require.Equal(t, "test-namespace", testCase.pipelineRun.Status.SourcePipeline.Pipeline.Namespace)

				// Verify annotations were copied for dev runs
				if testCase.pipelineRun.Annotations != nil {
					for k, v := range testCase.pipelineRun.Annotations {
						require.Equal(t, v, testCase.pipelineRun.Status.SourcePipeline.Pipeline.Annotations[k])
					}
				}
			}
		})
	}
}

// Helper function to create test manifest content
func createTestManifestContent(t *testing.T, config map[string]interface{}) *pbtypes.Any {
	// Convert map to JSON
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	// Convert JSON to protobuf Struct
	pbStruct := &pbtypes.Struct{}
	unmarshaler := &jsonpb.Unmarshaler{}
	err = unmarshaler.Unmarshal(bytes.NewReader(configJSON), pbStruct)
	require.NoError(t, err)

	// Wrap in TypedStruct
	typedStruct := &apipb.TypedStruct{
		Value: pbStruct,
	}

	// Marshal to Any
	anyContent, err := pbtypes.MarshalAny(typedStruct)
	require.NoError(t, err)

	return anyContent
}

// Helper function to extract environment variables from pipeline manifest
func extractEnvironmentFromPipeline(t *testing.T, pipeline *v2.Pipeline) map[string]string {
	if pipeline.Spec.Manifest.Content == nil {
		return make(map[string]string)
	}

	pbStruct := &apipb.TypedStruct{}
	err := pbtypes.UnmarshalAny(pipeline.Spec.Manifest.Content, pbStruct)
	require.NoError(t, err)

	marshaler := &jsonpb.Marshaler{}
	pipelineConfigStr, err := marshaler.MarshalToString(pbStruct.Value)
	require.NoError(t, err)

	pipelineConfig := make(map[string]interface{})
	err = json.Unmarshal([]byte(pipelineConfigStr), &pipelineConfig)
	require.NoError(t, err)

	envVars := make(map[string]string)
	if envVal, exists := pipelineConfig["environ"]; exists {
		if envMap, ok := envVal.(map[string]interface{}); ok {
			for k, v := range envMap {
				envVars[k] = v.(string)
			}
		}
	}

	return envVars
}
