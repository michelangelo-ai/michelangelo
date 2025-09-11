package actors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/jsonpb"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SourcePipelineType = "SourcePipeline"
)

type SourcePipelineActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	apiHandler api.Handler
	logger     *zap.Logger
}

func NewSourcePipelineActor(apiHandler api.Handler, logger *zap.Logger) *SourcePipelineActor {
	return &SourcePipelineActor{
		apiHandler: apiHandler,
		logger:     logger.With(zap.String("actor", "sourcepipeline")),
	}
}

var _ conditionInterfaces.ConditionActor[*v2.PipelineRun] = &SourcePipelineActor{}

func (a *SourcePipelineActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	if previousCondition == nil {
		logger.Info("pipeline run has no previous condition, setting to unknown")
		return &apipb.Condition{
			Type:   SourcePipelineType,
			Status: apipb.CONDITION_STATUS_UNKNOWN,
		}, nil
	}

	if previousCondition.Status != apipb.CONDITION_STATUS_UNKNOWN {
		// the previous condition is terminal, so we don't need to run the actor again
		logger.Info("pipeline run has a terminal condition, skipping")
		return previousCondition, nil
	}

	pipelineRunSpec := pipelineRun.GetSpec()

	// Check if this is a DevRun (pipeline_spec field is populated)
	pipeline := &v2.Pipeline{}
	if pipelineRunSpec.GetPipelineSpec() != nil {
		logger.Info("dev run detected, creating pipeline from inline pipeline_spec")

		// Create pipeline from inline spec
		devPipeline, err := a.createPipelineFromSpec(pipelineRunSpec, pipelineRun)
		if err != nil {
			logger.Error("failed to create pipeline from spec", zap.Error(err))
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to create pipeline from spec: %w", err)
		}
		pipeline = devPipeline
	} else {
		// Regular pipeline run - fetch from Kubernetes using pipeline reference
		if pipelineRunSpec.GetPipeline() == nil {
			logger.Info("pipeline run has no pipeline resource ID, setting to false")
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("pipeline resource ID is nil")
		}

		pipelineResourceID := pipelineRunSpec.GetPipeline()
		err := a.apiHandler.Get(ctx, pipelineRun.Namespace, pipelineResourceID.GetName(), &metav1.GetOptions{}, pipeline)
		if err != nil {
			logger.Error("failed to get pipeline", zap.Error(err))
			return &apipb.Condition{
				Type:   SourcePipelineType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to get pipeline: %w", err)
		}

		pipeline.ObjectMeta = metav1.ObjectMeta{
			Name:        pipeline.Name,
			Namespace:   pipeline.Namespace,
			Labels:      pipeline.Labels,
			Annotations: pipeline.Annotations,
		}
	}
	pipelineRun.Status.SourcePipeline = &v2.SourcePipeline{
		Pipeline: pipeline,
	}

	logger.Info("pipeline run has a pipeline resource, setting to true")
	return &apipb.Condition{
		Type:   SourcePipelineType,
		Status: apipb.CONDITION_STATUS_TRUE,
	}, nil
}

func (a *SourcePipelineActor) GetType() string {
	return SourcePipelineType
}

// createPipelineFromSpec creates a Pipeline CR from inline PipelineSpec for dev runs
func (a *SourcePipelineActor) createPipelineFromSpec(pipelineRunSpec v2.PipelineRunSpec, pipelineRun *v2.PipelineRun) (*v2.Pipeline, error) {
	pipelineSpec := pipelineRunSpec.GetPipelineSpec()

	// Validate required fields
	if err := a.validateDevRunSpec(pipelineSpec); err != nil {
		return nil, fmt.Errorf("dev run spec validation failed: %w", err)
	}

	// Create pipeline CR with metadata from pipeline run
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("devrun-%s", pipelineRun.Name),
			Namespace:   pipelineRun.Namespace,
			Annotations: make(map[string]string),
		},
		Spec: *pipelineSpec, // Use inline spec directly
	}

	// Copy annotations from pipeline run metadata (set by CLI during dev run creation)
	// For dev runs, annotations like image-id are stored in the PipelineRun metadata
	if pipelineRun.Annotations != nil {
		for k, v := range pipelineRun.Annotations {
			pipeline.ObjectMeta.Annotations[k] = v
		}
	}

	// Apply environment variable overrides
	var environOverrides *pbtypes.Struct
	if input := pipelineRunSpec.GetInput(); input != nil {
		if environField := input.Fields["environ"]; environField != nil {
			environOverrides = environField.GetStructValue()
		}
	}
	err := a.applyEnvironmentOverrides(pipeline, environOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to apply environment overrides: %w", err)
	}

	return pipeline, nil
}

// validateDevRunSpec validates required fields for dev run pipeline specs
func (a *SourcePipelineActor) validateDevRunSpec(pipelineSpec *v2.PipelineSpec) error {
	if pipelineSpec.Manifest == nil {
		return fmt.Errorf("pipeline_spec.manifest is required for dev runs")
	}
	if pipelineSpec.Manifest.UniflowTar == "" {
		return fmt.Errorf("pipeline_spec.manifest.uniflow_tar is required for dev runs")
	}
	return nil
}

// applyEnvironmentOverrides merges dev input environment variables with base pipeline config
func (a *SourcePipelineActor) applyEnvironmentOverrides(pipeline *v2.Pipeline, devInput *pbtypes.Struct) error {
	logger := a.logger.With(zap.String("applyEnvironmentOverrides", fmt.Sprintf("applyEnvironmentOverrides logs")))
	if devInput == nil || len(devInput.Fields) == 0 {
		logger.Info("no env overrides for dev run")
		return nil // No overrides to apply
	}

	// Deserialize existing manifest content
	baseConfig, err := a.decodePipelineManifestContent(pipeline.Spec)
	if err != nil {
		return fmt.Errorf("failed to decode base manifest: %w", err)
	}

	// Extract base environment
	baseEnv := make(map[string]interface{})
	if envVal, exists := baseConfig["environ"]; exists {
		if envMap, ok := envVal.(map[string]interface{}); ok {
			baseEnv = envMap
		}
	}

	// Apply dev input overrides - convert all values to strings for environment variables
	for key, value := range devInput.Fields {
		switch value.GetKind().(type) {
		case *pbtypes.Value_StringValue:
			baseEnv[key] = value.GetStringValue()
		case *pbtypes.Value_NumberValue:
			baseEnv[key] = fmt.Sprintf("%g", value.GetNumberValue())
		case *pbtypes.Value_BoolValue:
			baseEnv[key] = fmt.Sprintf("%t", value.GetBoolValue())
		}
	}

	// Update manifest content with merged environment
	baseConfig["environ"] = baseEnv

	// Re-encode manifest content
	updatedContent, err := a.encodePipelineManifestContent(baseConfig)
	if err != nil {
		return fmt.Errorf("failed to encode updated manifest: %w", err)
	}

	pipeline.Spec.Manifest.Content = updatedContent
	return nil
}

// decodePipelineManifestContent decodes pipeline manifest content from protobuf Any to map
func (a *SourcePipelineActor) decodePipelineManifestContent(pipelineSpec v2.PipelineSpec) (map[string]interface{}, error) {
	if pipelineSpec.Manifest.Content == nil {
		return map[string]interface{}{}, nil
	}

	pbStruct := &apipb.TypedStruct{}
	err := pbtypes.UnmarshalAny(pipelineSpec.Manifest.Content, pbStruct)
	if err != nil || pbStruct.Value == nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline manifest content to typed struct: %v", err)
	}

	marshaler := &jsonpb.Marshaler{}
	pipelineConfigStr, err := marshaler.MarshalToString(pbStruct.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal typed struct to JSON: %v", err)
	}

	pipelineConfig := make(map[string]interface{})
	err = json.Unmarshal([]byte(pipelineConfigStr), &pipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline manifest content to map: %v", err)
	}

	return pipelineConfig, nil
}

// encodePipelineManifestContent encodes map back to protobuf Any for pipeline manifest content
func (a *SourcePipelineActor) encodePipelineManifestContent(config map[string]interface{}) (*pbtypes.Any, error) {
	// Convert map to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to JSON: %v", err)
	}

	// Convert JSON to protobuf Struct
	pbStruct := &pbtypes.Struct{}
	unmarshaler := &jsonpb.Unmarshaler{}
	err = unmarshaler.Unmarshal(bytes.NewReader(configJSON), pbStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to protobuf Struct: %v", err)
	}

	// Wrap in TypedStruct
	typedStruct := &apipb.TypedStruct{
		Value: pbStruct,
	}

	// Marshal to Any
	anyContent, err := pbtypes.MarshalAny(typedStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TypedStruct to Any: %v", err)
	}

	return anyContent, nil
}
