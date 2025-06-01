package conditionUtils

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/michelangelo-ai/michelangelo/proto/api"
)

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
