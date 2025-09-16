package triggerrun

import (
	"testing"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
)

func TestGetTriggerType(t *testing.T) {
	t.Run("cron trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					TriggerType: &v2pb.Trigger_CronSchedule{
						CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
					},
				},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeCron, result)
	})

	t.Run("batch rerun trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{
					TriggerType: &v2pb.Trigger_BatchRerun{
						BatchRerun: &v2pb.BatchRerun{},
					},
				},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeBatchRerun, result)
	})

	t.Run("unknown trigger", func(t *testing.T) {
		triggerRun := &v2pb.TriggerRun{
			Spec: v2pb.TriggerRunSpec{
				Trigger: &v2pb.Trigger{},
			},
		}
		result := GetTriggerType(triggerRun)
		assert.Equal(t, TriggerTypeUnknown, result)
	})
}
