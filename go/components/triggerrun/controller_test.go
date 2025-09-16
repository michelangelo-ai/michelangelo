package triggerrun

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	workflowclientInterfacemocks "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface/interface_mock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewReconciler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkflowClient := workflowclientInterfacemocks.NewMockWorkflowClient(ctrl)
	mockCronTrigger := NewCronTrigger(logr.Discard(), mockWorkflowClient)

	params := Params{
		Logger:            logr.Discard(),
		WorkflowClient:    mockWorkflowClient,
		CronTrigger:       mockCronTrigger,
		IntervalTrigger:   mockCronTrigger,
		BackfillTrigger:   mockCronTrigger,
		BatchRerunTrigger: mockCronTrigger,
	}

	reconciler := NewReconciler(params)

	assert.NotNil(t, reconciler)
	assert.Equal(t, mockCronTrigger, reconciler.CronTrigger)
	assert.Equal(t, mockCronTrigger, reconciler.IntervalTrigger)
	assert.Equal(t, mockCronTrigger, reconciler.BackfillTrigger)
	assert.Equal(t, mockCronTrigger, reconciler.BatchRerunTrigger)
}

func TestGetRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkflowClient := workflowclientInterfacemocks.NewMockWorkflowClient(ctrl)
	mockCronTrigger := NewCronTrigger(logr.Discard(), mockWorkflowClient)
	mockIntervalTrigger := NewCronTrigger(logr.Discard(), mockWorkflowClient)
	mockBackfillTrigger := NewCronTrigger(logr.Discard(), mockWorkflowClient)
	mockBatchRerunTrigger := NewCronTrigger(logr.Discard(), mockWorkflowClient)

	reconciler := &Reconciler{
		CronTrigger:       mockCronTrigger,
		IntervalTrigger:   mockIntervalTrigger,
		BackfillTrigger:   mockBackfillTrigger,
		BatchRerunTrigger: mockBatchRerunTrigger,
	}

	tests := []struct {
		name           string
		triggerRun     *v2pb.TriggerRun
		expectedRunner Runner
	}{
		{
			name: "cron trigger",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{
							CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
						},
					},
				},
			},
			expectedRunner: mockCronTrigger,
		},
		{
			name: "batch rerun trigger",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_BatchRerun{
							BatchRerun: &v2pb.BatchRerun{},
						},
					},
				},
			},
			expectedRunner: mockBatchRerunTrigger,
		},
		{
			name: "unknown trigger defaults to cron",
			triggerRun: &v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{},
				},
			},
			expectedRunner: mockCronTrigger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := reconciler.getRunner(tt.triggerRun)
			assert.Equal(t, tt.expectedRunner, runner)
		})
	}
}
