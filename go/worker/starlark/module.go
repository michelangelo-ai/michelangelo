package starlark

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/storage"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflowfx"
	"go.uber.org/fx"
)

// RegisterStoragePlugin adds the storage plugin to the plugin registry.
func RegisterStoragePlugin(registry map[string]service.IPlugin) {
	registry[storage.Plugin.ID()] = storage.Plugin
}

// RegisterCachedOutputPlugin adds the cachedoutput plugin to the plugin registry.
func RegisterCachedOutputPlugin(registry map[string]service.IPlugin) {
	registry[cachedoutput.Plugin.ID()] = cachedoutput.Plugin
}

// RegisterRayPlugin adds the ray plugin to the plugin registry.
func RegisterRayPlugin(registry map[string]service.IPlugin) {
	registry[ray.Plugin.ID()] = ray.Plugin
}

// RegisterSparkPlugin adds the spark plugin to the plugin registry.
func RegisterSparkPlugin(registry map[string]service.IPlugin) {
	registry[spark.Plugin.ID()] = spark.Plugin
}

// ProvideActivityOptions creates ActivityOptions from configuration
func ProvideActivityOptions(config workflowfx.Config) workflow.ActivityOptions {
	// Use default options if no config or workers provided
	if len(config.Workers) == 0 || config.Workers[0].ActivityOptions == nil {
		return workflow.ActivityOptions{}
	}

	return parseActivityOptions(config.Workers[0].ActivityOptions)
}

// CreateStarlarkServiceParams defines the parameters for creating the starlark service
type CreateStarlarkServiceParams struct {
	fx.In
	Registry        map[string]service.IPlugin
	Workers         []worker.Worker
	Backend         service.BackendType
	ActivityOptions workflow.ActivityOptions
}

// CreateStarlarkService creates the starlark service with all registered plugins and activity options.
func CreateStarlarkService(params CreateStarlarkServiceParams) error {
	if len(params.Workers) == 0 {
		return fmt.Errorf("no workers provided")
	}

	// Use ServiceBuilder to create the service with activity options
	builder := service.NewServiceBuilder(params.Backend)
	builder.SetPlugins(params.Registry)
	builder.SetActivityOptions(params.ActivityOptions)

	workerService, err := builder.Build()
	if err != nil {
		return err
	}
	for _, w := range params.Workers {
		workerService.Register(w)
	}

	return nil
}

// parseActivityOptions converts map[string]interface{} from YAML to workflow.ActivityOptions
func parseActivityOptions(options map[string]interface{}) workflow.ActivityOptions {
	activityOptions := workflow.ActivityOptions{}

	if taskList, ok := options["taskList"].(string); ok {
		activityOptions.TaskList = taskList
	}

	if scheduleToCloseTimeout, ok := options["scheduleToCloseTimeout"].(string); ok {
		if duration, err := time.ParseDuration(scheduleToCloseTimeout); err == nil {
			activityOptions.ScheduleToCloseTimeout = duration
		}
	}

	if scheduleToStartTimeout, ok := options["scheduleToStartTimeout"].(string); ok {
		if duration, err := time.ParseDuration(scheduleToStartTimeout); err == nil {
			activityOptions.ScheduleToStartTimeout = duration
		}
	}

	if startToCloseTimeout, ok := options["startToCloseTimeout"].(string); ok {
		if duration, err := time.ParseDuration(startToCloseTimeout); err == nil {
			activityOptions.StartToCloseTimeout = duration
		}
	}

	if heartbeatTimeout, ok := options["heartbeatTimeout"].(string); ok {
		if duration, err := time.ParseDuration(heartbeatTimeout); err == nil {
			activityOptions.HeartbeatTimeout = duration
		}
	}

	if waitForCancellation, ok := options["waitForCancellation"].(bool); ok {
		activityOptions.WaitForCancellation = waitForCancellation
	}

	if activityID, ok := options["activityID"].(string); ok {
		activityOptions.ActivityID = activityID
	}

	if disableEagerExecution, ok := options["disableEagerExecution"].(bool); ok {
		activityOptions.DisableEagerExecution = disableEagerExecution
	}

	if versioningIntent, ok := options["versioningIntent"].(int); ok {
		activityOptions.VersioningIntent = versioningIntent
	}

	if summary, ok := options["summary"].(string); ok {
		activityOptions.Summary = summary
	}

	return activityOptions
}

var Module = fx.Options(
	fx.Provide(ProvideActivityOptions),
	fx.Invoke(RegisterStoragePlugin),
	fx.Invoke(RegisterCachedOutputPlugin),
	fx.Invoke(RegisterRayPlugin),
	fx.Invoke(RegisterSparkPlugin),
	fx.Invoke(CreateStarlarkService),
)
