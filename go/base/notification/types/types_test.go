package types

import (
	"testing"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContainsEventType(t *testing.T) {
	tests := []struct {
		name       string
		eventTypes []v2pb.Notification_EventType
		state      v2pb.PipelineRunState
		expected   bool
	}{
		{
			name:       "Contains succeeded event type",
			eventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			state:      v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			expected:   true,
		},
		{
			name:       "Contains failed event type",
			eventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED},
			state:      v2pb.PIPELINE_RUN_STATE_FAILED,
			expected:   true,
		},
		{
			name:       "Does not contain event type",
			eventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			state:      v2pb.PIPELINE_RUN_STATE_FAILED,
			expected:   false,
		},
		{
			name: "Multiple event types, contains match",
			eventTypes: []v2pb.Notification_EventType{
				v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED,
				v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_KILLED,
			},
			state:    v2pb.PIPELINE_RUN_STATE_KILLED,
			expected: true,
		},
		{
			name:       "Running state without started event type returns false",
			eventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
			state:      v2pb.PIPELINE_RUN_STATE_RUNNING,
			expected:   false,
		},
		{
			name:       "Running state with started event type returns true",
			eventTypes: []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED},
			state:      v2pb.PIPELINE_RUN_STATE_RUNNING,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsEventType(tt.eventTypes, tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSubject(t *testing.T) {
	tests := []struct {
		name        string
		pipelineRun *v2pb.PipelineRun
		expected    string
	}{
		{
			name: "Pipeline run with succeeded state",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pipeline-run",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
				},
			},
			expected: "Pipeline Run (my-pipeline-run) state: SUCCEEDED",
		},
		{
			name: "Pipeline run with failed state",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name: "failed-pipeline",
				},
				Status: v2pb.PipelineRunStatus{
					State: v2pb.PIPELINE_RUN_STATE_FAILED,
				},
			},
			expected: "Pipeline Run (failed-pipeline) state: FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSubject(tt.pipelineRun)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateText(t *testing.T) {
	const testStudioURL = "https://ml.example.com/studio/"

	pipelineRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "test-project",
			Labels: map[string]string{
				sourcePipelineTypeLabelName:         "PIPELINE_TYPE_TRAIN",
				sourcePipelineManifestTypeLabelName: "PIPELINE_MANIFEST_TYPE_ASL",
			},
		},
		Status: v2pb.PipelineRunStatus{
			State:  v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			LogUrl: "https://workflow.example.com/run/123",
		},
	}

	emailText := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, testStudioURL, nil)
	assert.Contains(t, emailText, "test-run")
	assert.Contains(t, emailText, "test-project")
	assert.Contains(t, emailText, "SUCCEEDED")
	assert.Contains(t, emailText, "TRAIN")
	assert.Contains(t, emailText, testStudioURL)
	assert.Contains(t, emailText, "https://workflow.example.com/run/123")

	slackText := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_SLACK, testStudioURL, nil)
	assert.Contains(t, slackText, "test-run")
	assert.Contains(t, slackText, "test-project")
	assert.Contains(t, slackText, "SUCCEEDED")
	assert.Contains(t, slackText, "TRAIN")
	assert.Contains(t, slackText, testStudioURL)
	assert.Contains(t, slackText, "<https://workflow.example.com/run/123|Workflow Log URL>")

	// No studio link when studioBaseURL is empty.
	noLinkText := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, "", nil)
	assert.NotContains(t, noLinkText, "Studio URL")

	// StudioBaseURL without trailing slash produces the same link as with one.
	withSlash := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, "https://ml.example.com/studio/", nil)
	withoutSlash := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, "https://ml.example.com/studio", nil)
	assert.Equal(t, withSlash, withoutSlash)

	// Custom phase resolver is honoured.
	customText := GenerateText(pipelineRun, v2pb.NOTIFICATION_TYPE_EMAIL, testStudioURL, func(_ string) string { return "custom-phase" })
	assert.Contains(t, customText, "custom-phase")
}

func TestDefaultPhaseResolver(t *testing.T) {
	tests := []struct {
		pipelineType string
		want         string
	}{
		{"PIPELINE_TYPE_TRAIN", "train"},
		{"PIPELINE_TYPE_EVAL", "train"},
		{"PIPELINE_TYPE_SCORER", "deploy"},
		{"PIPELINE_TYPE_PREDICTION", "deploy"},
		{"PIPELINE_TYPE_RETRAIN", "retrain"},
		{"PIPELINE_TYPE_DATA_PREP", "data"},
		{"PIPELINE_TYPE_EMBEDDING_GENERATION", "genai-data"},
		{"PIPELINE_TYPE_TRAIN_LLM", "genai-finetune"},
		{"PIPELINE_TYPE_EVAL_PROMPT", "genai-prompt"},
		{"UNKNOWN_TYPE", "pipeline"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, DefaultPhaseResolver(tt.pipelineType), tt.pipelineType)
	}
}

func TestCropPipelineRun(t *testing.T) {
	tests := []struct {
		name        string
		pipelineRun *v2pb.PipelineRun
		expectNil   bool
	}{
		{
			name:        "Nil pipeline run",
			pipelineRun: nil,
			expectNil:   true,
		},
		{
			name: "Pipeline run with full data",
			pipelineRun: &v2pb.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-run",
					Namespace: "test-namespace",
					Labels:    map[string]string{"env": "test"},
				},
				Spec: v2pb.PipelineRunSpec{
					Pipeline: &apipb.ResourceIdentifier{
						Namespace: "test-namespace",
						Name:      "test-pipeline",
					},
					Notifications: []*v2pb.Notification{
						{
							NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
							Emails:           []string{"test@example.com"},
						},
					},
				},
				Status: v2pb.PipelineRunStatus{
					State:        v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
					LogUrl:       "https://workflow.example.com/run/123",
					ErrorMessage: "",
					Code:         0,
					Conditions: []*apipb.Condition{
						{Type: "SourcePipeline", Status: apipb.CONDITION_STATUS_TRUE},
					},
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CropPipelineRun(tt.pipelineRun)

			if tt.expectNil {
				assert.Nil(t, result)
				return
			}

			assert.NotNil(t, result)
			assert.Equal(t, tt.pipelineRun.Name, result.Name)
			assert.Equal(t, tt.pipelineRun.Namespace, result.Namespace)
			assert.Equal(t, tt.pipelineRun.Labels, result.Labels)
			assert.Equal(t, tt.pipelineRun.Spec, result.Spec)
			assert.Equal(t, tt.pipelineRun.Status.State, result.Status.State)
			assert.Equal(t, tt.pipelineRun.Status.LogUrl, result.Status.LogUrl)
			assert.Equal(t, tt.pipelineRun.Status.ErrorMessage, result.Status.ErrorMessage)
			assert.Equal(t, tt.pipelineRun.Status.Code, result.Status.Code)
			assert.Equal(t, tt.pipelineRun.Status.EndTime, result.Status.EndTime)
			assert.Nil(t, result.Status.Conditions)
		})
	}
}
