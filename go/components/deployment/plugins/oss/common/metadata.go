package common

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
)

// Cluster states used by rollout, cleanup, and rollback actors
const (
	ClusterStatePending = "PENDING"

	// Deployment states
	ClusterStateDeploymentInProgress = "DEPLOYMENT_IN_PROGRESS"
	ClusterStateDeployed             = "DEPLOYED"

	// Cleanup states
	ClusterStateCleanupInProgress = "CLEANUP_IN_PROGRESS"
	ClusterStateCleaned           = "CLEANED"
	// Rollback states
	ClusterStateRollbackInProgress = "ROLLBACK_IN_PROGRESS"
	ClusterStateRolledBack         = "ROLLED_BACK"
)

// ClusterEntry tracks the state for a single cluster.
type ClusterEntry struct {
	ClusterId             string `json:"cluster_id"`
	Host                  string `json:"host"`
	Port                  string `json:"port"`
	TokenTag              string `json:"token_tag"`
	CaDataTag             string `json:"ca_data_tag"`
	State                 string `json:"state"`
	IsControlPlaneCluster bool   `json:"is_control_plane_cluster"`
}

// ClusterMetadata stores the multi-cluster state in condition metadata.
// Used by both RollingRolloutActor and ModelCleanupActor.
type ClusterMetadata struct {
	BackendType  string         `json:"backend_type"`
	Clusters     []ClusterEntry `json:"clusters"`
	CurrentIndex int            `json:"current_index"`
}

// GetClusterMetadata extracts the cluster metadata from the condition.
// Returns nil if metadata doesn't exist or is invalid.
func GetClusterMetadata(condition *apipb.Condition) *ClusterMetadata {
	if condition.Metadata == nil {
		return nil
	}

	structVal := &types.Struct{}
	if err := types.UnmarshalAny(condition.Metadata, structVal); err != nil {
		return nil
	}

	fields := structVal.GetFields()
	if fields == nil {
		return nil
	}

	// Check if this is cluster metadata (has "clusters" field)
	clustersField, ok := fields["clusters"]
	if !ok || clustersField == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(convertStructToMap(structVal))
	if err != nil {
		return nil
	}

	var metadata ClusterMetadata
	if err := json.Unmarshal(jsonBytes, &metadata); err != nil {
		return nil
	}

	return &metadata
}

// SetClusterMetadata stores the cluster metadata in the condition.
func SetClusterMetadata(condition *apipb.Condition, metadata *ClusterMetadata) error {
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var mapData map[string]interface{}
	if err = json.Unmarshal(jsonBytes, &mapData); err != nil {
		return fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	structVal := convertMapToStruct(mapData)
	condition.Metadata, err = types.MarshalAny(structVal)
	if err != nil {
		return fmt.Errorf("failed to marshal to Any: %w", err)
	}

	return nil
}

// GetClusterTargetConnection reconstructs a ClusterTargetMetadata from the metadata entry.
func GetClusterTargetConnection(entry *ClusterEntry) *gateways.TargetClusterConnection {
	return &gateways.TargetClusterConnection{
		ClusterId:             entry.ClusterId,
		Host:                  entry.Host,
		Port:                  entry.Port,
		TokenTag:              entry.TokenTag,
		CaDataTag:             entry.CaDataTag,
		IsControlPlaneCluster: entry.IsControlPlaneCluster,
	}
}

// convertStructToMap converts a types.Struct to a map[string]interface{}.
func convertStructToMap(s *types.Struct) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range s.GetFields() {
		result[k] = convertValueToInterface(v)
	}
	return result
}

// convertValueToInterface converts a types.Value to interface{}.
func convertValueToInterface(v *types.Value) interface{} {
	if v == nil {
		return nil
	}
	switch k := v.GetKind().(type) {
	case *types.Value_StringValue:
		return k.StringValue
	case *types.Value_NumberValue:
		return k.NumberValue
	case *types.Value_BoolValue:
		return k.BoolValue
	case *types.Value_StructValue:
		return convertStructToMap(k.StructValue)
	case *types.Value_ListValue:
		list := make([]interface{}, len(k.ListValue.GetValues()))
		for i, item := range k.ListValue.GetValues() {
			list[i] = convertValueToInterface(item)
		}
		return list
	default:
		return nil
	}
}

// convertMapToStruct converts a map[string]interface{} to a types.Struct.
func convertMapToStruct(m map[string]interface{}) *types.Struct {
	fields := make(map[string]*types.Value)
	for k, v := range m {
		fields[k] = convertInterfaceToValue(v)
	}
	return &types.Struct{Fields: fields}
}

// convertInterfaceToValue converts an interface{} to a types.Value.
func convertInterfaceToValue(v interface{}) *types.Value {
	switch val := v.(type) {
	case string:
		return &types.Value{Kind: &types.Value_StringValue{StringValue: val}}
	case float64:
		return &types.Value{Kind: &types.Value_NumberValue{NumberValue: val}}
	case bool:
		return &types.Value{Kind: &types.Value_BoolValue{BoolValue: val}}
	case map[string]interface{}:
		return &types.Value{Kind: &types.Value_StructValue{StructValue: convertMapToStruct(val)}}
	case []interface{}:
		list := make([]*types.Value, len(val))
		for i, item := range val {
			list[i] = convertInterfaceToValue(item)
		}
		return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: list}}}
	default:
		return &types.Value{Kind: &types.Value_NullValue{}}
	}
}
