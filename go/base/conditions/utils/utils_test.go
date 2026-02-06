package conditionUtils

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestGenerateUnknownCondition(t *testing.T) {
	inputs := map[string]struct {
		originalCondition api.Condition
		inputMessage      string
		inputReason       string

		isTimestampChanged bool
	}{
		"If original condition contains the same message and reason as input, then timestamp shouldn't change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_UNKNOWN,
			},

			isTimestampChanged: false,
		},
		"If original condition is not unknown, then timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
			},

			isTimestampChanged: true,
		},
		"If the reason has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_UNKNOWN,
				Reason: "foo",
			},
			inputReason: "",

			isTimestampChanged: true,
		},
		"If the message has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status:  api.CONDITION_STATUS_UNKNOWN,
				Message: "foo",
			},
			inputMessage: "",

			isTimestampChanged: true,
		},
	}

	for _, input := range inputs {
		originalTimestamp := input.originalCondition.LastUpdatedTimestamp
		newCondition := GenerateUnknownCondition(&input.originalCondition, input.inputMessage, input.inputReason)
		require.Equal(t, api.CONDITION_STATUS_UNKNOWN, newCondition.Status)
		require.Equal(t, input.inputMessage, newCondition.Message)
		require.Equal(t, input.inputReason, newCondition.Reason)

		if input.isTimestampChanged {
			require.NotEqual(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		} else {
			require.Equal(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		}
	}
}

func TestGenerateTrueCondition(t *testing.T) {
	inputs := map[string]struct {
		originalCondition api.Condition

		isTimestampChanged bool
	}{
		"If original condition has a different status, then the timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_UNKNOWN,
			},

			isTimestampChanged: true,
		},
		"If original condition has the same status the the timestamp shouldn't change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
			},

			isTimestampChanged: false,
		},
		"If the reason has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
				Reason: "foo",
			},

			isTimestampChanged: true,
		},
		"If the message has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status:  api.CONDITION_STATUS_TRUE,
				Message: "foo",
			},

			isTimestampChanged: true,
		},
	}

	for _, input := range inputs {
		originalTimestamp := input.originalCondition.LastUpdatedTimestamp
		newCondition := GenerateTrueCondition(&input.originalCondition)
		require.Equal(t, api.CONDITION_STATUS_TRUE, newCondition.Status)
		require.Equal(t, "", newCondition.Message)
		require.Equal(t, "", newCondition.Reason)

		if input.isTimestampChanged {
			require.NotEqual(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		} else {
			require.Equal(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		}
	}
}

func TestGenerateFalseCondition(t *testing.T) {
	inputs := map[string]struct {
		originalCondition api.Condition
		inputMessage      string
		inputReason       string

		isTimestampChanged bool
	}{
		"If original condition contains the same message and reason as input, then timestamp shouldn't change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_FALSE,
			},

			isTimestampChanged: false,
		},
		"If original condition is not unknown, then timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
			},

			isTimestampChanged: true,
		},
		"If the reason has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status: api.CONDITION_STATUS_FALSE,
				Reason: "foo",
			},
			inputReason: "",

			isTimestampChanged: true,
		},
		"If the message has changed, then timestamp should change": {
			originalCondition: api.Condition{
				Status:  api.CONDITION_STATUS_FALSE,
				Message: "foo",
			},
			inputMessage: "",

			isTimestampChanged: true,
		},
	}

	for _, input := range inputs {
		originalTimestamp := input.originalCondition.LastUpdatedTimestamp
		newCondition := GenerateFalseCondition(&input.originalCondition, input.inputMessage, input.inputReason)
		require.Equal(t, api.CONDITION_STATUS_FALSE, newCondition.Status)
		require.Equal(t, input.inputMessage, newCondition.Message)
		require.Equal(t, input.inputReason, newCondition.Reason)

		if input.isTimestampChanged {
			require.NotEqual(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		} else {
			require.Equal(t, originalTimestamp, newCondition.LastUpdatedTimestamp)
		}
	}
}

func TestGetCondition(t *testing.T) {
	testCases := []struct {
		msg           string
		conditionType string
		conditions    []*api.Condition
		expected      *api.Condition
	}{
		{
			msg:           "get test condition successfully",
			conditionType: "test",
			conditions: []*api.Condition{
				{
					Type: "test",
				},
			},
			expected: &api.Condition{
				Type: "test",
			},
		},
		{
			msg:           "Can not find condition",
			conditionType: "test2",
			conditions: []*api.Condition{
				{
					Type: "test",
				},
			},
			expected: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.msg, func(t *testing.T) {
			condition := GetCondition(testCase.conditionType, testCase.conditions)
			if testCase.expected == nil {
				require.Nil(t, condition)
			} else {
				require.NotNil(t, condition)
				require.Equal(t, testCase.expected.Type, condition.Type)
			}
		})
	}
}
