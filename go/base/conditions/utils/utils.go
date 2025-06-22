package conditionUtils

import (
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
