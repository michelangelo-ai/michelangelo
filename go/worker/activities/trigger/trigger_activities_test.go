package trigger

import (
	"testing"

	"mock/github.com/michelangelo-ai/michelangelo/proto/api/v2/v2mock"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	"github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Suite struct {
	suite.Suite
	activitySuite          types.StarTestActivitySuite
	t                      *testing.T
	mockPipelineRunService *v2mock.MockPipelineRunServiceYARPCClient
	act                    *activities
}

func TestITCadence(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewCadTestActivitySuite(),
		t:             t,
	})
}

func TestITTemporal(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewTempTestActivitySuite(),
		t:             t,
	})
}

func (r *Suite) SetupSuite() {
	ctrl := gomock.NewController(r.t)
	r.mockPipelineRunService = v2mock.NewMockPipelineRunServiceYARPCClient(ctrl)
	r.act = &activities{
		pipelineRunService: r.mockPipelineRunService,
	}
	r.activitySuite.RegisterActivity(r.act)
}

func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {}

func (r *Suite) TestGenerateBatchRunParams() {

	tests := []struct {
		name           string
		triggerRun     *v2pb.TriggerRun
		expectedError  bool
		expectedParams [][]parameter.Params
	}{
		{
			name: "success - empty parameters map",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trigger-run",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{
								Cron: "0 0 * * *",
							},
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]parameter.Params{
				{{ParamID: ""}}, // Single batch with empty param
			},
		},
		{
			name: "success - with parameters map (single batch)",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trigger-run-single",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{
								Cron: "0 0 * * *",
							},
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"id1": {ParameterMap: map[string]string{"city": "los angelos"}},
							"id2": {ParameterMap: map[string]string{"city": "san jose"}},
							"id3": {ParameterMap: map[string]string{"city": "fremont"}},
						},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]parameter.Params{
				{ // Single batch with all 3 params
					{ParamID: "id1"},
					{ParamID: "id2"},
					{ParamID: "id3"},
				},
			},
		},
		{
			name: "success - multiple batches with batch policy",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trigger-run-multi",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{
								Cron: "0 0 * * *",
							},
						},
						BatchPolicy: &v2pb.BatchPolicy{BatchSize: 2}, // Force batch size of 2
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"id1": {ParameterMap: map[string]string{"city": "los angelos"}},
							"id2": {ParameterMap: map[string]string{"city": "san jose"}},
							"id3": {ParameterMap: map[string]string{"city": "fremont"}},
							"id4": {ParameterMap: map[string]string{"city": "san francisco"}},
							"id5": {ParameterMap: map[string]string{"city": "new york"}},
						},
					},
				},
			},
			expectedError: false,
			expectedParams: [][]parameter.Params{
				{ // First batch (2 params)
					{ParamID: "id1"},
					{ParamID: "id2"},
				},
				{ // Second batch (2 params)
					{ParamID: "id3"},
					{ParamID: "id4"},
				},
				{ // Third batch (1 param)
					{ParamID: "id5"},
				},
			},
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			result, err := r.activitySuite.ExecuteActivity(Activities.GenerateBatchRunParams, tt.triggerRun)

			if tt.expectedError {
				assert.Error(r.T(), err)
				return
			}

			assert.NoError(r.T(), err)
			assert.True(r.T(), result.HasValue())

			var actualResult [][]parameter.Params
			assert.NoError(r.T(), result.Get(&actualResult))

			// Verify the number of batches
			assert.Len(r.T(), actualResult, len(tt.expectedParams), "Number of batches should match")

			// Verify each batch individually
			for i, expectedParam := range tt.expectedParams {
				assert.ElementsMatch(r.T(), expectedParam, actualResult[i], "Batch %d should match exactly", i)
			}
		})
	}
}

func (r *Suite) TestGenerateConcurrentRunParams() {
	tests := []struct {
		name           string
		triggerRun     *v2pb.TriggerRun
		expectedError  bool
		expectedParams []parameter.Params
	}{
		{
			name: "success - empty parameters map",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trigger-run",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{
								Cron: "0 0 * * *",
							},
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
					},
				},
			},
			expectedError:  false,
			expectedParams: []parameter.Params{}, // Empty slice when no parameters
		},
		{
			name: "success - with parameters map",
			triggerRun: &v2pb.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-trigger-run",
				},
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{
								Cron: "0 0 * * *",
							},
						},
						ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
							"param1": {
								ParameterMap: map[string]string{
									"key1": "value1",
								},
							},
							"param2": {
								ParameterMap: map[string]string{
									"key2": "value2",
								},
							},
						},
					},
				},
			},
			expectedError: false,
			expectedParams: []parameter.Params{
				{ParamID: "param1"},
				{ParamID: "param2"},
			},
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			result, err := r.activitySuite.ExecuteActivity(Activities.GenerateConcurrentRunParams, tt.triggerRun)

			if tt.expectedError {
				assert.Error(r.T(), err)
				return
			}

			assert.NoError(r.T(), err)
			assert.True(r.T(), result.HasValue())

			var actualResult []parameter.Params
			assert.NoError(r.T(), result.Get(&actualResult))

			// Verify the number of parameters
			assert.Len(r.T(), actualResult, len(tt.expectedParams), "Number of parameters should match")

			// Verify each parameter matches expected
			assert.ElementsMatch(r.T(), tt.expectedParams, actualResult, "Parameters should match exactly")
		})
	}
}

func (r *Suite) TestCreatePipelineRun() {
	tests := []struct {
		name          string
		pipelineRun   *v2pb.PipelineRun
		mockResponse  *v2pb.CreatePipelineRunResponse
		mockError     error
		expectedError bool
	}{
		{
			name: "success - create pipeline run",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pipeline-run",
				},
				Spec: v2pb.PipelineRunSpec{
					Pipeline: &api.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
				},
			},
			mockResponse: &v2pb.CreatePipelineRunResponse{
				PipelineRun: &v2pb.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pipeline-run",
					},
				},
			},
			mockError:     nil,
			expectedError: false,
		},
		{
			name: "error - service returns error",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pipeline-run",
				},
				Spec: v2pb.PipelineRunSpec{
					Pipeline: &api.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
				},
			},
			mockResponse:  nil,
			mockError:     assert.AnError,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			request := &v2pb.CreatePipelineRunRequest{
				PipelineRun: tt.pipelineRun,
			}

			r.mockPipelineRunService.EXPECT().CreatePipelineRun(gomock.Any(), request).Return(tt.mockResponse, tt.mockError)

			res, err := r.activitySuite.ExecuteActivity(Activities.CreatePipelineRun, request)

			if tt.expectedError {
				assert.Error(r.T(), err)
				return
			}

			assert.NoError(r.T(), err)
			assert.True(r.T(), res.HasValue())

			var resp *v2pb.PipelineRun
			res.Get(&resp)
			assert.NotNil(r.T(), resp)
			assert.Equal(r.T(), tt.pipelineRun.Name, resp.Name)
		})
	}
}

func (r *Suite) TestPipelineRunSensor() {
	tests := []struct {
		name          string
		pipelineRun   *v2pb.PipelineRun
		mockResponse  *v2pb.GetPipelineRunResponse
		mockError     error
		expectedError bool
	}{
		{
			name: "success - get pipeline run",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
			},
			mockResponse: &v2pb.GetPipelineRunResponse{
				PipelineRun: &v2pb.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipeline-run",
						Namespace: "test-namespace",
					},
				},
			},
			mockError:     nil,
			expectedError: false,
		},
		{
			name: "error - service returns error",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
				},
			},
			mockResponse:  nil,
			mockError:     assert.AnError,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		r.Run(tt.name, func() {
			request := &v2pb.GetPipelineRunRequest{
				Namespace: tt.pipelineRun.Namespace,
				Name:      tt.pipelineRun.Name,
			}

			r.mockPipelineRunService.EXPECT().GetPipelineRun(gomock.Any(), request).Return(tt.mockResponse, tt.mockError)

			res, err := r.activitySuite.ExecuteActivity(Activities.PipelineRunSensor, tt.pipelineRun)

			if tt.expectedError {
				assert.Error(r.T(), err)
				return
			}

			assert.NoError(r.T(), err)
			assert.True(r.T(), res.HasValue())

			var resp *v2pb.PipelineRun
			res.Get(&resp)
			assert.NotNil(r.T(), resp)
			assert.Equal(r.T(), tt.pipelineRun.Name, resp.Name)
		})
	}
}
