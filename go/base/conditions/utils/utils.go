package conditionUtils

import (
	"time"

	api "github.com/michelangelo-ai/michelangelo/proto/api"
)

// GetCondition provides a utility method for retrieving a particular condition from a condition list.
// If there is no such condition that exists, nil is returned.
func GetCondition(t string, conditions []*api.Condition) *api.Condition {
	for _, condition := range conditions {
		if condition.Type == t {
			return condition
		}
	}

	return nil
}

// GenerateUnknownCondition is a helper method for modifying the api.Condition struct
// with a api.CONDITION_STATUS_UNKNOWN status. The LastUpdatedTimestamp will
// be updated if there are any changes from the original condition.
func GenerateUnknownCondition(
	condition *api.Condition,
	message string,
	reason string,
) *api.Condition {
	return generateCondition(condition, api.CONDITION_STATUS_UNKNOWN, message, reason)
}

// GenerateTrueCondition is a helper method for modifying the api.Condition struct
// with a api.CONDITION_STATUS_TRUE status. The message and reason aren't needed for these
// scenarios because a positively oriented condition is default and expected. The LastUpdatedTimestamp will
// be updated if there are any changes from the original condition.
func GenerateTrueCondition(condition *api.Condition) *api.Condition {
	return GenerateTrueConditionWithMessage(condition, "")
}

// GenerateTrueConditionWithMessage is a helper method for modifying the api.Condition struct
// with a api.CONDITION_STATUS_TRUE status. The reason isn't needed for these
// scenarios because a positively oriented condition is default and expected. The LastUpdatedTimestamp will
// be updated if there are any changes from the original condition.
func GenerateTrueConditionWithMessage(condition *api.Condition, message string) *api.Condition {
	return generateCondition(condition, api.CONDITION_STATUS_TRUE, message, "")
}

// GenerateFalseCondition is a helper method for modifying the api.Condition struct
// with a api.CONDITION_STATUS_FALSE status. The LastUpdatedTimestamp will
// be updated if there are any changes from the original condition.
func GenerateFalseCondition(condition *api.Condition, message string, reason string) *api.Condition {
	return generateCondition(condition, api.CONDITION_STATUS_FALSE, message, reason)
}

func generateCondition(
	condition *api.Condition,
	status api.ConditionStatus,
	message string,
	reason string,
) *api.Condition {
	if condition.Status != status || condition.Message != message || condition.Reason != reason {
		condition.LastUpdatedTimestamp = time.Now().Unix()
	}

	condition.Status = status
	condition.Message = message
	condition.Reason = reason

	return condition
}
