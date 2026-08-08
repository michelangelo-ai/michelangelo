package triggerrun

import (
	"context"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	interfaceMock "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScheduleInputHashDeterministic(t *testing.T) {
	first := scheduleInputTestTriggerRun()
	first.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
		"global": {},
		"market": {},
	}

	second := scheduleInputTestTriggerRun()
	second.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
		"market": {},
		"global": {},
	}

	assert.Equal(t, mustScheduleInputHash(t, first), mustScheduleInputHash(t, second))
}

func TestScheduleInputHashTracksEffectiveInput(t *testing.T) {
	base := scheduleInputTestTriggerRun()
	baseHash := mustScheduleInputHash(t, base)

	tests := []struct {
		name   string
		mutate func(*v2pb.TriggerRun)
	}{
		{
			name: "parameters",
			mutate: func(triggerRun *v2pb.TriggerRun) {
				triggerRun.Spec.Trigger.ParametersMap["market"] = &v2pb.PipelineExecutionParameters{}
			},
		},
		{
			name: "max concurrency",
			mutate: func(triggerRun *v2pb.TriggerRun) {
				triggerRun.Spec.Trigger.MaxConcurrency++
			},
		},
		{
			name: "revision",
			mutate: func(triggerRun *v2pb.TriggerRun) {
				triggerRun.Spec.Revision.Name = "revision-2"
			},
		},
		{
			name: "environment label",
			mutate: func(triggerRun *v2pb.TriggerRun) {
				triggerRun.Labels[scheduleInputEnvironmentLabel] = "staging"
			},
		},
		{
			name: "pipeline manifest type label",
			mutate: func(triggerRun *v2pb.TriggerRun) {
				triggerRun.Labels[scheduleInputPipelineManifestTypeLabel] = "PIPELINE_MANIFEST_TYPE_ASL"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base.DeepCopy()
			test.mutate(changed)
			assert.NotEqual(t, baseHash, mustScheduleInputHash(t, changed))
		})
	}
}

func TestScheduleInputHashIgnoresSeparatelyTrackedNotifications(t *testing.T) {
	base := scheduleInputTestTriggerRun()
	changed := base.DeepCopy()
	changed.Spec.Notifications = []*v2pb.Notification{{
		NotificationType: v2pb.NOTIFICATION_TYPE_EMAIL,
		Emails:           []string{"owner@example.com"},
	}}

	assert.Equal(t, mustScheduleInputHash(t, base), mustScheduleInputHash(t, changed))
	assert.Equal(t, changed.Spec.Notifications, scheduleWorkflowInput(changed).Spec.Notifications,
		"notifications must remain in the Temporal schedule workflow input")
}

func TestScheduleInputHashIgnoresReconciliationOnlyFields(t *testing.T) {
	base := scheduleInputTestTriggerRun()
	changed := base.DeepCopy()
	changed.TypeMeta = metav1.TypeMeta{Kind: "TriggerRun", APIVersion: "michelangelo.api/v2"}
	changed.UID = "uid-1"
	changed.ResourceVersion = "123"
	changed.Generation = 9
	changed.Labels["unrelated"] = "ignored"
	changed.Status = v2pb.TriggerRunStatus{
		State:                   v2pb.TRIGGER_RUN_STATE_PAUSED,
		ErrorMessage:            "old error",
		ActualScheduleInputHash: "old-hash",
	}
	changed.Spec.Action = v2pb.TRIGGER_RUN_ACTION_RESUME
	changed.Spec.Kill = true

	assert.Equal(t, mustScheduleInputHash(t, base), mustScheduleInputHash(t, changed))

	workflowInput := scheduleWorkflowInput(changed)
	assert.Equal(t, v2pb.TRIGGER_RUN_ACTION_NO_ACTION, workflowInput.Spec.Action)
	assert.False(t, workflowInput.Spec.Kill)
	assert.Equal(t, v2pb.TriggerRunStatus{}, workflowInput.Status)
	assert.Equal(t, changed.Labels, workflowInput.Labels)
}

func TestCronTriggerUpdateRefreshesChangedScheduleInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
	triggerRun := scheduleInputTestTriggerRun()
	triggerRun.Status = v2pb.TriggerRunStatus{
		State: v2pb.TRIGGER_RUN_STATE_RUNNING,
		ActualTrigger: &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: triggerRun.Spec.Trigger.GetCronSchedule().GetCron()},
			},
		},
	}

	appliedTriggerRun := triggerRun.DeepCopy()
	appliedTriggerRun.Spec.Trigger.MaxConcurrency = 1
	triggerRun.Status.ActualScheduleInputHash = mustScheduleInputHash(t, appliedTriggerRun)
	desiredHash := mustScheduleInputHash(t, triggerRun)

	mockClient.EXPECT().UpdateTrigger(
		gomock.Any(),
		"test-namespace.test-trigger",
		"",
		gomock.Nil(),
		gomock.Eq([]interface{}{CreateTriggerRequest{TriggerRun: scheduleWorkflowInput(triggerRun)}}),
	).Return(nil)

	status, handled, err := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient).Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)

	require.NoError(t, err)
	assert.False(t, handled)
	assert.Equal(t, desiredHash, status.ActualScheduleInputHash)
}

func TestCronTriggerUpdateBackfillsMissingHashOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
	triggerRun := scheduleInputTestTriggerRun()
	triggerRun.Status = v2pb.TriggerRunStatus{
		State: v2pb.TRIGGER_RUN_STATE_RUNNING,
		ActualTrigger: &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: triggerRun.Spec.Trigger.GetCronSchedule().GetCron()},
			},
		},
	}

	mockClient.EXPECT().UpdateTrigger(
		gomock.Any(),
		"test-namespace.test-trigger",
		"",
		gomock.Nil(),
		gomock.Any(),
	).Return(nil).Times(1)

	runner := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient)
	firstStatus, _, err := runner.Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)
	require.NoError(t, err)
	require.NotEmpty(t, firstStatus.ActualScheduleInputHash)

	triggerRun.Status = firstStatus
	secondStatus, _, err := runner.Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)
	require.NoError(t, err)
	assert.Equal(t, firstStatus, secondStatus)
}

func TestCronTriggerUpdateDoesNotAdvanceHashOnFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
	triggerRun := scheduleInputTestTriggerRun()
	triggerRun.Status = v2pb.TriggerRunStatus{
		State:                   v2pb.TRIGGER_RUN_STATE_RUNNING,
		ActualScheduleInputHash: "previous-hash",
		ActualTrigger: &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: triggerRun.Spec.Trigger.GetCronSchedule().GetCron()},
			},
		},
	}

	mockClient.EXPECT().UpdateTrigger(
		gomock.Any(),
		"test-namespace.test-trigger",
		"",
		gomock.Nil(),
		gomock.Any(),
	).Return(assert.AnError)

	status, _, err := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient).Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_NO_ACTION)

	require.Error(t, err)
	assert.Equal(t, "previous-hash", status.ActualScheduleInputHash)
}

func TestCronTriggerUpdatePauseWithInputDriftIsAtomic(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := interfaceMock.NewMockWorkflowClient(ctrl)
	triggerRun := scheduleInputTestTriggerRun()
	triggerRun.Status = v2pb.TriggerRunStatus{
		State:                   v2pb.TRIGGER_RUN_STATE_RUNNING,
		ActualScheduleInputHash: "previous-hash",
		ActualTrigger: &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: triggerRun.Spec.Trigger.GetCronSchedule().GetCron()},
			},
		},
	}
	triggerRun.Spec.Action = v2pb.TRIGGER_RUN_ACTION_PAUSE
	desiredHash := mustScheduleInputHash(t, triggerRun)

	mockClient.EXPECT().UpdateTrigger(
		gomock.Any(),
		"test-namespace.test-trigger",
		"",
		gomock.Eq(boolPointer(true)),
		gomock.Eq([]interface{}{CreateTriggerRequest{TriggerRun: scheduleWorkflowInput(triggerRun)}}),
	).Return(nil)

	status, handled, err := NewCronTrigger(zapr.NewLogger(zap.NewNop()), mockClient).Update(
		context.Background(), triggerRun, v2pb.TRIGGER_RUN_ACTION_PAUSE)

	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, v2pb.TRIGGER_RUN_STATE_PAUSED, status.State)
	assert.Equal(t, desiredHash, status.ActualScheduleInputHash)
}

func boolPointer(value bool) *bool {
	return &value
}

func scheduleInputTestTriggerRun() *v2pb.TriggerRun {
	return &v2pb.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-trigger",
			Labels: map[string]string{
				scheduleInputEnvironmentLabel: "production",
			},
		},
		Spec: v2pb.TriggerRunSpec{
			Revision: &api.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "revision-1",
			},
			Trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
				},
				ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
					"global": {},
				},
				MaxConcurrency: 2,
			},
		},
	}
}
