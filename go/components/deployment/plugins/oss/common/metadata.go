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

// ReadBatchLoadedClusters reads the per-cluster loaded map from a batch actor condition.
// Returns a map of clusterID → loaded (true/false). Returns nil if no metadata yet.
func ReadBatchLoadedClusters(condition *apipb.Condition) (map[string]bool, error) {
	if condition.Metadata == nil {
		return nil, nil
	}
	s := &types.Struct{}
	if err := types.UnmarshalAny(condition.Metadata, s); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(s.Fields))
	for k, v := range s.Fields {
		out[k] = v.GetBoolValue()
	}
	return out, nil
}

// WriteBatchLoadedClusters persists the per-cluster loaded map on a batch actor condition.
func WriteBatchLoadedClusters(condition *apipb.Condition, loaded map[string]bool) error {
	fields := make(map[string]*types.Value, len(loaded))
	for k, v := range loaded {
		fields[k] = &types.Value{Kind: &types.Value_BoolValue{BoolValue: v}}
	}
	metadata, err := types.MarshalAny(&types.Struct{Fields: fields})
	if err != nil {
		return err
	}
	condition.Metadata = metadata
	return nil
}
