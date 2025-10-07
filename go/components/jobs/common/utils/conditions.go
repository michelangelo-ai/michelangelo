package utils

import (
	"time"

	protoTypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
)

// GetCondition retrieves a condition of the supplied
// type. The caller must ensure to update the object
// to persist the update.
func GetCondition(cs *[]*apipb.Condition, name string, generation int64) *apipb.Condition {
	for _, c := range *cs {
		if c.Type == name {
			return c
		}
	}

	cond := &apipb.Condition{
		Type:   name,
		Status: apipb.CONDITION_STATUS_UNKNOWN,
	}

	*cs = append(*cs, cond)
	return cond
}

// ConditionUpdateParams provides params
// for update to a condition
type ConditionUpdateParams struct {
	Status     apipb.ConditionStatus // required
	Reason     string                // required
	Generation int64                 // required

	Message  string          // optional
	Metadata *protoTypes.Any // optional
}

// UpdateCondition updates a given condition
func UpdateCondition(c *apipb.Condition, p ConditionUpdateParams) {
	c.Status = p.Status
	c.Reason = p.Reason
	c.ObservedGeneration = p.Generation
	c.Message = p.Message
	c.Metadata = p.Metadata
	c.LastUpdatedTimestamp = time.Now().Unix()
}

// IsJobScheduled returns true if the scheduled condition is true
func IsJobScheduled(cs []*apipb.Condition, generation int64) bool {
	condition := GetCondition(&cs, constants.ScheduledCondition, generation)
	if condition != nil && condition.Status == apipb.CONDITION_STATUS_TRUE {
		return true
	}

	return false
}
