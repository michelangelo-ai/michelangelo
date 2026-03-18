package main

import (
	"github.com/michelangelo-ai/michelangelo/go/api/crd"
	apihandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/auth"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore/minio"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	projectapihook "github.com/michelangelo-ai/michelangelo/go/components/project/apihook"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/blobstorage"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
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
		blobstore.Module,
		minio.Module,
		fx.Invoke(printConfig),
		apihandler.APIServerModule,
		auth.DummyAuthModule,
		logging.DummyAuditLogModule,
		fx.Provide(getTallyScope),
		fx.Provide(baseconfig.GetK8sConfig),
		fx.Provide(getYARPCConfig),
		fx.Provide(baseconfig.GetMetadataStorageConfig),
		fx.Provide(baseconfig.GetBlobStorageConfig),
		fx.Provide(provideBlobStorage),
		fx.Provide(provideDispatcher),
		fx.Provide(getScheme),
		fx.Invoke(projectapihook.RegisterProjectAPIHook),
		v2pb.CachedOutputSvcModule,
		v2pb.ClusterSvcModule,
		v2pb.DeploymentSvcModule,
		v2pb.ModelFamilySvcModule,
		v2pb.ModelSvcModule,
		v2pb.PipelineRunSvcModule,
		v2pb.PipelineSvcModule,
		v2pb.ProjectSvcModule,
		v2pb.RayClusterSvcModule,
		v2pb.RayJobSvcModule,
		v2pb.RevisionSvcModule,
		v2pb.SparkJobSvcModule,
		v2pb.TriggerRunSvcModule,
		crd.Module,
		crd.SyncCRDs(v2pb.GroupVersion.Group,
			[]string{},
			v2pb.YamlSchemas),
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

// provideBlobStorage returns a BlobStorage implementation backed by the given BlobStore.
// Returns nil when blob storage is disabled in config, making it optional for the API server.
func provideBlobStorage(store *blobstore.BlobStore, config baseconfig.BlobStorageConfig) storage.BlobStorage {
	if !config.Enabled {
		return nil
	}
	return blobstorage.New(store, config.ToBlobStorageConfig())
}

