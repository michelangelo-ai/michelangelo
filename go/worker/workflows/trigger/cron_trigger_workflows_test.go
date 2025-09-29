package trigger

import (
	"testing"
	"time"

	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGeneratePipelineRunName(t *testing.T) {
	now := time.Date(2023, 1, 15, 10, 30, 45, 123456789, time.UTC)

	result := generatePipelineRunName(now)

	// Should generate a name based on timestamp
	assert.Contains(t, result, "20230115")
	assert.Contains(t, result, "103045")
	// Should contain random suffix
	assert.True(t, len(result) > 20) // Base timestamp + random suffix
}

func TestGeneratePipelineRunRequest_WithEmptyParameters(t *testing.T) {
	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-trigger",
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
			Trigger: &v2pb.Trigger{
				ParametersMap: map[string]*v2pb.PipelineExecutionParameters{},
			},
		},
	}

	paramID := ""
	pipelineRunName := "test-pipeline-run-123"
	ts := time.Date(2023, 1, 15, 10, 30, 45, 0, time.UTC)

	result, err := generatePipelineRunRequest(triggerRun, paramID, pipelineRunName, ts)

	require.NoError(t, err)
	assert.Equal(t, pipelineRunName, result.PipelineRun.ObjectMeta.Name)
	assert.Equal(t, "test-namespace", result.PipelineRun.ObjectMeta.Namespace)

	// Check labels
	labels := result.PipelineRun.ObjectMeta.Labels
	assert.Equal(t, "1673778645", labels[PipelineRunExecutionTimestampLabel])
	assert.Equal(t, "test-trigger", labels[TriggerredByLabel])
	assert.Equal(t, "test-trigger", labels[SourceTriggerLabel])
	assert.Equal(t, "production", labels[EnvironmentLabel]) // Default environment

	// Check annotations
	annotations := result.PipelineRun.ObjectMeta.Annotations
	assert.Equal(t, "condition", annotations["michelangelo.uber.com/pipelinerun.engine"])
	assert.NotEmpty(t, annotations["michelangelo/UpdateTimestamp"])
	assert.NotEmpty(t, annotations["michelangelo/SpecUpdateTimestamp"])
}

func TestGeneratePipelineRunRequest_WithParameters(t *testing.T) {
	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-trigger",
			Labels: map[string]string{
				EnvironmentLabel: "development",
			},
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
			Trigger: &v2pb.Trigger{
				ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
					"param1": {},
				},
			},
		},
	}

	paramID := "param1"
	pipelineRunName := "test-pipeline-run-123"
	ts := time.Date(2023, 1, 15, 10, 30, 45, 0, time.UTC)

	result, err := generatePipelineRunRequest(triggerRun, paramID, pipelineRunName, ts)

	require.NoError(t, err)

	// Check labels
	labels := result.PipelineRun.ObjectMeta.Labels
	assert.Equal(t, "param1", labels[ParameterIDLabel])
	assert.Equal(t, "development", labels[EnvironmentLabel])
}

func TestGeneratePipelineRunRequest_InvalidParameterID(t *testing.T) {
	triggerRun := &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-trigger",
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-pipeline",
			},
			Trigger: &v2pb.Trigger{
				ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
					"param1": {},
				},
			},
		},
	}

	_, err := generatePipelineRunRequest(triggerRun, "invalid-param", "test-run", time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid parameter id: invalid-param")
}

func TestConstants(t *testing.T) {
	// Test that important constants are defined correctly
	assert.Equal(t, 600, _defaultWaitSeconds)
	assert.Equal(t, "default", _defaultParameterID)

	// Test label constants
	assert.Equal(t, "pipelinerun.michelangelo/triggered-by", TriggerredByLabel)
	assert.Equal(t, "pipelinerun.michelangelo/environment", EnvironmentLabel)
	assert.Equal(t, "pipelinerun.michelangelo/source-trigger", SourceTriggerLabel)
	assert.Equal(t, "pipelinerun.michelangelo/execution-timestamp", PipelineRunExecutionTimestampLabel)
	assert.Equal(t, "pipelinerun.michelangelo/parameter-id", ParameterIDLabel)
	assert.Equal(t, "pipeline.michelangelo/PipelineManifestType", PipelineManifestTypeLabel)
}

func TestActivityOptions(t *testing.T) {
	// Test that activity options are configured correctly
	assert.Equal(t, 30*time.Second, _activityOptionsDefault.ScheduleToStartTimeout)
	assert.Equal(t, 30*time.Second, _activityOptionsDefault.StartToCloseTimeout)

	// Test retry policy
	retryPolicy := _activityOptionsDefault.RetryPolicy
	require.NotNil(t, retryPolicy)
	assert.Equal(t, 500*time.Millisecond, retryPolicy.InitialInterval)
	assert.Equal(t, 2.0, retryPolicy.BackoffCoefficient)
	assert.Equal(t, int32(5), retryPolicy.MaximumAttempts)

	// Test non-retriable errors
	expectedErrors := []string{
		"400",
		"404",
		"500",
		"cadenceInternal:Panic",
		"cadenceInternal:CanceledError",
		"no-retry",
	}
	assert.Equal(t, expectedErrors, NonRetriableErrorReasonsDefault)
}

func TestSensorRetryPolicy(t *testing.T) {
	// Test sensor retry policy configuration
	assert.Equal(t, 20*time.Second, SensorRetryPolicyDefault.InitialInterval)
	assert.Equal(t, float64(1), SensorRetryPolicyDefault.BackoffCoefficient)
	assert.Equal(t, 24*14*time.Hour, SensorRetryPolicyDefault.ExpirationInterval)
	assert.Equal(t, NonRetriableErrorReasonsDefault, SensorRetryPolicyDefault.NonRetriableErrorReasons)
}

func TestWorkflowsStruct(t *testing.T) {
	// Test that the workflows struct can be instantiated
	w := &workflows{}
	assert.NotNil(t, w)

	// Test that the global Workflows variable is properly typed
	assert.Nil(t, Workflows) // It's initialized as nil in the original code
}
