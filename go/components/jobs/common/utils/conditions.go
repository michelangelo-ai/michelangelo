package utils

import (
	"time"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	protoTypes "github.com/gogo/protobuf/types"
	v2beta1pb "michelangelo/api/v2beta1"
)

// GetCondition retrieves a condition of the supplied
// type. The caller must ensure to update the object
// to persist the update.
func GetCondition(cs *[]*v2beta1pb.Condition, name string, generation int64) *v2beta1pb.Condition {
	for _, c := range *cs {
		if c.Type == name {
			return c
		}
	}

	cond := &v2beta1pb.Condition{
		Type:               name,
		Status:             v2beta1pb.CONDITION_STATUS_UNKNOWN,
		ObservedGeneration: generation,
	}

	*cs = append(*cs, cond)
	return cond
}

// ConditionUpdateParams provides params
// for update to a condition
type ConditionUpdateParams struct {
	Status     v2beta1pb.ConditionStatus // required
	Reason     string                    // required
	Generation int64                     // required

	Message  string          // optional
	Metadata *protoTypes.Any // optional
}

// UpdateCondition updates a given condition
func UpdateCondition(c *v2beta1pb.Condition, p ConditionUpdateParams) {
	c.Status = p.Status
	c.Reason = p.Reason
	c.ObservedGeneration = p.Generation
	c.Message = p.Message
	c.Metadata = p.Metadata
	c.LastUpdatedTimestamp = time.Now().Unix()
}

// IsJobScheduled returns true if the scheduled condition is true
func IsJobScheduled(cs []*v2beta1pb.Condition, generation int64) bool {
	condition := GetCondition(&cs, constants.ScheduledCondition, generation)
	if condition != nil && condition.Status == v2beta1pb.CONDITION_STATUS_TRUE {
		return true
	}

	return false
}
