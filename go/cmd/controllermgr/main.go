package main

import (
	"fmt"

	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/uber-go/tally"

	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/proxy"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/gateways"
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
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
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

// newProxyProvider creates a new proxy provider
func newProxyProvider(dynamicClient dynamic.Interface, logger *zap.Logger) proxy.ProxyProvider {
	return proxy.NewHTTPRouteManager(dynamicClient, logger)
}

// newDynamicClient creates a Kubernetes dynamic client for working with unstructured resources
func newDynamicClient() (dynamic.Interface, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to get rest config: %w", err))
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %w", err))
	}
	return dynamicClient, nil
}

// newModelConfigMapProvider creates a new model config map provider
func newModelConfigMapProvider(client kubeclient.Client, logger *zap.Logger) configmap.ModelConfigMapProvider {
	return configmap.NewDefaultModelConfigMapProvider(client, logger)
}

// newEventRecorder creates a new event recorder
func newEventRecorder(mgr ctrl.Manager) record.EventRecorder {
	return mgr.GetEventRecorderFor(inferenceserver.ControllerName)
}

// newInferenceServerGateway creates a new inference server gateway with clients
func newInferenceServerGateway(kubeClient kubeclient.Client, modelConfigMapProvider configmap.ModelConfigMapProvider, logger *zap.Logger) gateways.Gateway {
	return gateways.NewGatewayWithClients(gateways.Params{
		Logger:                 logger,
		KubeClient:             kubeClient,
		ModelConfigMapProvider: modelConfigMapProvider,
	})
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
		baseconfig.Module,
		fx.Provide(scheme),
		fx.Provide(baseconfig.GetK8sConfig),
		fx.Provide(baseconfig.GetMetadataStorageConfig),
		fx.Provide(baseconfig.GetWorkflowClientConfig),
		fx.Provide(getTallyScope),
		fx.Provide(newProxyProvider),
		fx.Provide(newDynamicClient),
		fx.Provide(newModelConfigMapProvider),
		fx.Provide(newInferenceServerGateway),
		fx.Provide(newEventRecorder),
		apiHandler.CtrlMgrModule,
		spark.Module,
		ray.Module,
		triggerrun.Module,
		cadenceclient.Module,
		pipeline.Module,
		pipelinerun.Module,
		controllermgr.Module,
		deployment.Module,
		inferenceserver.Module,
		scheduler.Module,
		cluster.Module,
		client.Module,
		fx.Invoke(func(logger *zap.Logger) {
			ctrl.SetLogger(zapr.NewLogger(logger))
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
