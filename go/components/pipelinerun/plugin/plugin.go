// Package plugin provides the PipelineRun plugin and its dependencies.
//
// The plugin implements a condition-based execution model where pipeline runs
// progress through multiple stages, each handled by a specialized actor:
//   - SourcePipelineActor: Retrieves and validates the pipeline definition
//   - ImageBuildActor: Manages container image resolution
//   - ExecuteWorkflowActor: Orchestrates workflow execution via Cadence/Temporal
//
// The plugin integrates with the condition engine to coordinate actor execution
// and manage pipeline run state transitions.
package plugin

import (
	"slices"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionsInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	uberconfig "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	// Module is the Uber FX module for the PipelineRun plugin.
	//
	// It provides the Plugin instance which contains all ConditionActors
	// needed for pipeline execution. The module is automatically included
	// when using the pipelinerun.Module.
	Module = fx.Options(
		fx.Provide(NewPlugin),
	)
)

// Plugin implements the condition-based plugin interface for PipelineRun execution.
//
// It contains a collection of ConditionActors that handle different stages of
// pipeline execution. The plugin is used by the condition engine to orchestrate
// pipeline run progress through various states.
type Plugin struct {
	conditionsInterfaces.Plugin[*v2.PipelineRun]
	Actors []conditionsInterfaces.ConditionActor[*v2.PipelineRun]
	Logger *zap.Logger
}

// PluginParams contains the dependencies required to create a PipelineRun plugin.
type PluginParams struct {
	fx.In
	ApiHandler     api.Handler
	WorkflowClient clientInterfaces.WorkflowClient
	BlobStore      *blobstore.BlobStore
	ConfigProvider uberconfig.Provider
	Logger         *zap.Logger
}

// NewPlugin creates a new PipelineRun plugin with all required actors.
//
// The plugin is initialized with three actors that execute in sequence:
//  1. SourcePipelineActor: Retrieves the pipeline definition
//  2. ImageBuildActor: Resolves container images for execution
//  3. ExecuteWorkflowActor: Starts and monitors workflow execution
//
// Dependencies are injected via FX using the PluginParams struct.
//
// Returns a configured Plugin ready for use by the condition engine.
func NewPlugin(params PluginParams) *Plugin {
	logger := params.Logger.With(zap.String("plugin", "pipelinerun"))
	return &Plugin{
		Actors: []conditionsInterfaces.ConditionActor[*v2.PipelineRun]{
			actors.NewSourcePipelineActor(params.ApiHandler, logger),
			actors.NewImageBuildActor(logger),
			actors.NewExecuteWorkflowActor(logger, params.WorkflowClient, params.BlobStore, params.ApiHandler, params.ConfigProvider),
		},
		Logger: logger,
	}
}

// GetActors returns the ordered list of ConditionActors for pipeline execution.
//
// The condition engine executes these actors sequentially, with each actor
// checking prerequisites and performing its stage of pipeline execution.
func (p *Plugin) GetActors() []conditionsInterfaces.ConditionActor[*v2.PipelineRun] {
	return p.Actors
}

// GetConditions retrieves the current conditions from a PipelineRun resource.
//
// Conditions track the status of each actor's execution stage and are used
// by the condition engine to determine which actors need to run.
func (p *Plugin) GetConditions(pipelineRun *v2.PipelineRun) []*apipb.Condition {
	return pipelineRun.Status.Conditions
}

// PutCondition updates or adds a condition to a PipelineRun resource.
//
// If a condition with the same type already exists, it is updated with the new
// values. Otherwise, the condition is appended to the conditions list. This
// allows actors to persist their execution state.
func (p *Plugin) PutCondition(pipelineRun *v2.PipelineRun, condition *apipb.Condition) {
	conditionIndex := slices.IndexFunc(pipelineRun.Status.Conditions, func(c *apipb.Condition) bool {
		return c.Type == condition.Type
	})
	if conditionIndex == -1 {
		pipelineRun.Status.Conditions = append(pipelineRun.Status.Conditions, condition)
	} else {
		pipelineRun.Status.Conditions[conditionIndex] = condition
	}
}
