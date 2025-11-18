package utils

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	"github.com/stretchr/testify/require"
)

func TestGetConditions(t *testing.T) {
	type test struct {
		// input
		conditions []*apipb.Condition
		generation int64

		// expected
		added          bool
		expectedStatus apipb.ConditionStatus
		assertion      require.BoolAssertionFunc

		msg string
	}

	tt := []test{
		{
			conditions:     nil,
			generation:     2,
			added:          true,
			expectedStatus: apipb.CONDITION_STATUS_UNKNOWN,
			assertion:      require.True,
			msg:            "Get from nil set",
		},
		{
			conditions: []*apipb.Condition{
				{
					Type:               constants.PendingCondition,
					Status:             apipb.CONDITION_STATUS_TRUE,
					ObservedGeneration: 2,
				},
			},
			generation:     2,
			added:          false,
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			assertion:      require.True,
			msg:            "Get existing condition",
		},
		{
			conditions: []*apipb.Condition{
				{
					Type:               constants.PendingCondition,
					Status:             apipb.CONDITION_STATUS_TRUE,
					ObservedGeneration: 2,
				},
			},
			generation:     3,
			added:          false,
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			assertion:      require.False,
			msg:            "Get existing condition but outdated generation",
		},
		{
			conditions: []*apipb.Condition{
				{
					Type:               constants.ScheduledCondition,
					Status:             apipb.CONDITION_STATUS_FALSE,
					ObservedGeneration: 3,
				},
			},
			generation:     4,
			added:          true,
			expectedStatus: apipb.CONDITION_STATUS_UNKNOWN,
			assertion:      require.True,
			msg:            "Get a condition that's not found in the existing set",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			beforeLen := len(test.conditions)

			c := GetCondition(&test.conditions, constants.PendingCondition, test.generation)
			require.Equal(t, test.expectedStatus, c.Status)

			afterLen := len(test.conditions)
			if test.added {
				require.Equal(t, 1+beforeLen, afterLen)
			} else {
				require.Equal(t, beforeLen, afterLen)
			}
		})
	}
}

func TestIsJobScheduled(t *testing.T) {
	tt := []struct {
		generation    int64
		conditions    []*apipb.Condition
		wantScheduled bool
	}{
		{
			generation: 3,
			conditions: []*apipb.Condition{
				{
					Type:               constants.ScheduledCondition,
					Status:             apipb.CONDITION_STATUS_FALSE,
					ObservedGeneration: 3,
				},
			},
			wantScheduled: false,
		},
		{
			generation:    3,
			conditions:    nil,
			wantScheduled: false,
		},
		{
			generation: 3,
			conditions: []*apipb.Condition{
				{
					Type:               constants.ScheduledCondition,
					Status:             apipb.CONDITION_STATUS_UNKNOWN,
					ObservedGeneration: 3,
				},
			},
			wantScheduled: false,
		},
		{
			generation: 3,
			conditions: []*apipb.Condition{
				{
					Type:               constants.ScheduledCondition,
					Status:             apipb.CONDITION_STATUS_TRUE,
					ObservedGeneration: 3,
				},
			},
			wantScheduled: true,
		},
		{
			generation: 3,
			conditions: []*apipb.Condition{
				{
					Type:               constants.ScheduledCondition,
					Status:             apipb.CONDITION_STATUS_TRUE,
					ObservedGeneration: 4,
				},
			},
			wantScheduled: true,
		},
	}

	for _, test := range tt {
		isScheduled := IsJobScheduled(test.conditions, test.generation)
		if test.wantScheduled {
			require.True(t, isScheduled)
			continue
		}

		require.False(t, isScheduled)
	}
}

func TestUpdateConditions(t *testing.T) {
	type test struct {
		// input
		condition             *apipb.Condition
		conditionUpdateParams ConditionUpdateParams

		// expected
		expectedStatus apipb.ConditionStatus
		assertion      require.ErrorAssertionFunc

		msg string
	}

	tt := []test{
		{
			condition: &apipb.Condition{
				Type:                 "type",
				Status:               apipb.CONDITION_STATUS_UNKNOWN,
				Reason:               "reason",
				Message:              "msg",
				LastUpdatedTimestamp: 0,
				Metadata:             nil,
				ObservedGeneration:   0,
			},
			conditionUpdateParams: ConditionUpdateParams{
				Status:     apipb.CONDITION_STATUS_TRUE,
				Reason:     "update-reason",
				Generation: 1,
				Message:    "update-msg",
			},
			expectedStatus: apipb.CONDITION_STATUS_UNKNOWN,
			assertion:      require.NoError,
			msg:            "Update condition",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			require.NotEqual(t, test.condition.Status, test.conditionUpdateParams.Status)
			require.NotEqual(t, test.condition.Reason, test.conditionUpdateParams.Reason)
			require.NotEqual(t, test.condition.ObservedGeneration, test.conditionUpdateParams.Generation)
			require.NotEqual(t, test.condition.Message, test.conditionUpdateParams.Message)

			UpdateCondition(test.condition, test.conditionUpdateParams)

			require.Equal(t, test.condition.Status, test.conditionUpdateParams.Status)
			require.Equal(t, test.condition.Reason, test.conditionUpdateParams.Reason)
			require.Equal(t, test.condition.ObservedGeneration, test.conditionUpdateParams.Generation)
			require.Equal(t, test.condition.Message, test.conditionUpdateParams.Message)
		})
	}
}
