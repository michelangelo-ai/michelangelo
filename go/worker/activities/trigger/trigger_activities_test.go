package trigger

import (
	"testing"

	triggerrunUtil "github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	"github.com/stretchr/testify/assert"
)

func TestGetParameterGenerator(t *testing.T) {
	t.Run("cron trigger", func(t *testing.T) {
		generator := getParameterGenerator(triggerrunUtil.TriggerTypeCron)
		assert.NotNil(t, generator)
		assert.IsType(t, &parameter.CronParameterGenerator{}, generator)
	})

	t.Run("interval trigger", func(t *testing.T) {
		generator := getParameterGenerator(triggerrunUtil.TriggerTypeInterval)
		assert.NotNil(t, generator)
		assert.IsType(t, &parameter.CronParameterGenerator{}, generator)
	})

	t.Run("unknown trigger", func(t *testing.T) {
		generator := getParameterGenerator("unknown")
		assert.NotNil(t, generator)
		assert.IsType(t, &parameter.CronParameterGenerator{}, generator)
	})
}
