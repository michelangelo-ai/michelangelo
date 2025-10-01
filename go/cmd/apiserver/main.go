package main

import (
	"github.com/michelangelo-ai/michelangelo/go/api/crd"
	apihandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/auth"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	pipelinerunapihook "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/apihook"
	projectapihook "github.com/michelangelo-ai/michelangelo/go/components/project/apihook"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
	uberconfig "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

const serverName = "ma-apiserver"

func main() {
	fx.New(
		opts(),
	).Run()
}

func opts() fx.Option {
	return fx.Options(
		env.Module,
		baseconfig.Module,
		zapfx.Module,
		fx.Invoke(printConfig),
		apihandler.APIServerModule,
		auth.DummyAuthModule,
		logging.DummyAuditLogModule,
		fx.Provide(getTallyScope),
		fx.Provide(baseconfig.GetK8sConfig),
		fx.Provide(getYARPCConfig),
		fx.Provide(baseconfig.GetMetadataStorageConfig),
		fx.Provide(provideDispatcher),
		fx.Provide(getScheme),
		fx.Invoke(projectapihook.RegisterProjectAPIHook),
		fx.Invoke(pipelinerunapihook.RegisterPipelineRunAPIHook),
		v2pb.ProjectSvcModule,
		v2pb.PipelineSvcModule,
		v2pb.RayClusterSvcModule,
		v2pb.RayJobSvcModule,
		v2pb.SparkJobSvcModule,
		v2pb.CachedOutputSvcModule,
		v2pb.PipelineRunSvcModule,
		v2pb.ModelFamilySvcModule,
		v2pb.ModelSvcModule,
		v2pb.TriggerRunSvcModule,
		v2pb.DeploymentSvcModule,
		crd.Module,
		crd.SyncCRDs(v2pb.GroupVersion.Group, v2pb.YamlSchemas),
		fx.Invoke(registerProcedures),
		fx.Invoke(startYARPCServer),
	)
}

func getTallyScope() (tally.Scope, error) {
	s, _ := tally.NewRootScopeWithDefaultInterval(tally.ScopeOptions{
		Prefix: serverName,
	})
	return s, nil
}

func getScheme() (*runtime.Scheme, error) {
	s := scheme.Scheme
	if err := v2pb.AddToScheme(s); err != nil {
		return nil, err
	}
	return s, nil
}

func printConfig(logger *zap.Logger, provider uberconfig.Provider) {
	logger.Info("Configuration", zap.Any("config", provider.Get(uberconfig.Root)))
}
