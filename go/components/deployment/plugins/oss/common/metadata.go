package common

import (
	"github.com/gogo/protobuf/types"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// ReadModelLoadedFlag returns true if a previous Retrieve call confirmed the model is loaded
// and stored that result on the condition. This short-circuits repeated Triton status polls
// once the model has been confirmed ready.
func ReadModelLoadedFlag(condition *apipb.Condition) (bool, error) {
	if condition.Metadata == nil {
		return false, nil
	}
	val := &types.BoolValue{}
	if err := types.UnmarshalAny(condition.Metadata, val); err != nil {
		return false, err
	}
	return val.Value, nil
}

// WriteModelLoadedFlag records that the model is loaded on the condition's Metadata.
func WriteModelLoadedFlag(condition *apipb.Condition) error {
	metadata, err := types.MarshalAny(&types.BoolValue{Value: true})
	if err != nil {
		return err
	}
	condition.Metadata = metadata
	return nil
}
