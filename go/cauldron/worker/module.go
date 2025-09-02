package worker

import (
	"github.com/cadence-workflow/starlark-worker/service"
	activitiesrayhttp "github.com/michelangelo-ai/michelangelo/go/cauldron/worker/activities/rayhttp"
	activitiessparkyhttp "github.com/michelangelo-ai/michelangelo/go/cauldron/worker/activities/sparkhttp"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/http"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/plugins/rayhttp"
	"github.com/michelangelo-ai/michelangelo/go/cauldron/worker/plugins/sparkhttp"
	"go.uber.org/fx"
)

// RegisterRayHTTPPlugin adds the rayhttp plugin to the plugin registry.
func RegisterRayHTTPPlugin(registry map[string]service.IPlugin) {
	registry[rayhttp.Plugin.ID()] = rayhttp.Plugin
}

// RegisterSparkHTTPPlugin adds the sparkhttp plugin to the plugin registry.
func RegisterSparkHTTPPlugin(registry map[string]service.IPlugin) {
	registry[sparkhttp.Plugin.ID()] = sparkhttp.Plugin
}

var Module = fx.Options(
	fx.Invoke(RegisterRayHTTPPlugin),
	fx.Invoke(RegisterSparkHTTPPlugin),
	activitiesrayhttp.Module,
	activitiessparkyhttp.Module,
	http.Module,
)
