package main

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/uber-go/tally"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/discovery"
	deploymentOSSPlugin "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/route"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	inferenceserverOSSPlugin "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/components/ingester"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/scheduler"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun"
	"github.com/michelangelo-ai/michelangelo/go/components/ray"
	"github.com/michelangelo-ai/michelangelo/go/components/spark"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/controllermgr"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const serverName = "ma-controllermgr"

// scheme provides a Kubernetes runtime.Scheme object.
//
// This function creates a new Kubernetes runtime scheme and registers both the standard Kubernetes API types
// (via the k8s.io/client-go/kubernetes/scheme package) and custom API types defined in the proto/api/v2 package.
//
// Returns:
//   - *runtime.Scheme: A runtime scheme containing registered Kubernetes API and custom CRD types.
//   - error: An error if there is a failure during scheme registration.
func scheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := kubescheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v2pb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func getTallyScope() (tally.Scope, error) {
	// Create basic tally scope with console output for now
	s, _ := tally.NewRootScopeWithDefaultInterval(tally.ScopeOptions{
		Prefix: serverName,
	})

	// Register Prometheus metrics with controller-runtime
	metrics.RegisterMetrics()

	return s, nil
}

// options provides the FX modules and configurations used by the application.
//
// This function defines the dependencies and lifecycle management for the application by:
//   - Providing the Kubernetes runtime scheme as a dependency.
//   - Including the controllermgr.Module, which defines additional FX modules specific to the application.
//   - Setting up a logger to be used by the controller-runtime package.
//
// Returns:
//   - fx.Option: A collection of FX options defining the application's modules and configurations.
func options() fx.Option {
	return fx.Options(
		env.Module,
		zapfx.Module,
		fx.Provide(zapr.NewLogger),
		baseconfig.Module,
		fx.Provide(scheme),
		fx.Provide(baseconfig.GetK8sConfig),
		fx.Provide(baseconfig.GetMetadataStorageConfig),
		fx.Provide(baseconfig.GetMySQLConfig),
		fx.Provide(baseconfig.GetIngesterConfig),
		fx.Provide(baseconfig.GetWorkflowClientConfig),
		fx.Provide(getTallyScope),
		fx.Provide(provideMetadataStorage),
		fx.Provide(provideIngesterConfig),
		apiHandler.CtrlMgrModule,
		spark.Module,
		ray.Module,
		triggerrun.Module,
		workflowclient.Module,
		ingester.Module,
		pipeline.Module,
		pipelinerun.Module,
		controllermgr.Module,
		route.Module,
		discovery.Module,
		deploymentOSSPlugin.Module,
		deployment.Module,
		backends.Module,
		modelconfig.Module,
		inferenceserverOSSPlugin.Module,
		inferenceserver.Module,
		scheduler.Module,
		cluster.Module,
		client.Module,
		fx.Invoke(func(logger logr.Logger) {
			ctrl.SetLogger(logger)
		}),
	)
}

// main initializes and runs the application.
//
// This function uses the FX framework to bootstrap the application with the provided options
// and starts the application lifecycle. The application's lifecycle will continue to run until
// an interrupt signal is received, at which point it will cleanly shut down all managed components.
func main() {
	fx.New(options()).Run()
}
