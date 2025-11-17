package plugin

import (
	"slices"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionsInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	uberconfig "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	Module = fx.Options(
		fx.Provide(NewPlugin),
	)
)

// Plugin implements the PipelineRun plugin with a collection of condition actors
// that execute different stages of the pipeline lifecycle.
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

// NewPlugin creates a new PipelineRun plugin with all required actors for managing
// pipeline execution stages.
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

// GetActors returns the list of ConditionActors for a particular plugin. The Engine will sequentially run through the
func (p *Plugin) GetActors() []conditionsInterfaces.ConditionActor[*v2.PipelineRun] {
	return p.Actors
}

// GetConditions gets the conditions for a particular Kubernetes custom resource.
func (p *Plugin) GetConditions(pipelineRun *v2.PipelineRun) []*apipb.Condition {
	return pipelineRun.Status.Conditions
}

// PutCondition puts a condition for a particular Kubernetes custom resource.
// If the condition is not found, it will be added. If the condition is found, it will be updated.
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
