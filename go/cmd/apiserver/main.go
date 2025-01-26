package main

import (
	"context"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/logging"
	"github.com/michelangelo-ai/michelangelo/go/storage"
	"github.com/michelangelo-ai/michelangelo/go/storage/blobstorage/minio"
	"github.com/michelangelo-ai/michelangelo/go/storage/mysql"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
	uber_config "go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/http"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Module provides the YARPC dispatcher and server.
var Module = fx.Options(
	zapfx.Module,
	fx.Provide(logging.GetLogrLoggerOrPanic),
	fx.Provide(handler.GetCRDScheme),
	handler.APIServerModule,
	config.Module,
	mysql.Module,
	minio.Module,
	storage.ConfigModule,
	fx.Provide(utils.NewAuditLogEmitter),
	fx.Provide(newRestConfig),
	fx.Provide(newScope),
	fx.Provide(newAuth),
	fx.Provide(v2pb.GetK8sClient),
	v2pb.RayJobSvcModule,
	v2pb.RayClusterSvcModule,
	v2pb.ProjectSvcModule,
	v2pb.SparkJobSvcModule,
	fx.Provide(newConfig),
	fx.Provide(provideDispatcher),
	fx.Invoke(startYARPCServer),
)

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

const configKey = "apiserver"

type (
	// Config defines the server's configuration.
	Config struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
		QPS   float32 `yaml:"k8sQps"`
		Burst int     `yaml:"k8sBurst"`
	}

	In struct {
		fx.In

		Metrics         tally.Scope
		Config Config
		RayCluster v2pb.RayClusterServiceYARPCServer
		RayJob v2pb.RayJobServiceYARPCServer
		Project v2pb.ProjectServiceYARPCServer
		SparkJob v2pb.SparkJobServiceYARPCServer
	}

	Out struct {
		fx.Out
		Dispatcher *yarpc.Dispatcher
	}
)

// provideDispatcher creates and configures a YARPC dispatcher.
func provideDispatcher(in In) (Out, error) {
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "michelangelo-apiserver",
		Outbounds: yarpc.Outbounds{
			"michelangelo-apiserver": {
				Unary: http.NewTransport().NewSingleOutbound(fmt.Sprintf("http://%s:%d", in.Config.Host, in.Config.Port)),
			},
		},
		Inbounds: yarpc.Inbounds{
			http.NewTransport().NewInbound(fmt.Sprintf(":%d", in.Config.Port)),
		},
	})
	dispatcher.Register(v2pb.BuildRayClusterServiceYARPCProcedures(in.RayCluster))
	dispatcher.Register(v2pb.BuildRayJobServiceYARPCProcedures(in.RayJob))
	dispatcher.Register(v2pb.BuildProjectServiceYARPCProcedures(in.Project))
	dispatcher.Register(v2pb.BuildSparkJobServiceYARPCProcedures(in.SparkJob))
	return Out{Dispatcher: dispatcher}, nil
}

// startYARPCServer starts the YARPC dispatcher.
func startYARPCServer(lc fx.Lifecycle, dispatcher *yarpc.Dispatcher) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dispatcher.Start()
		},
		OnStop: func(ctx context.Context) error {
			return dispatcher.Stop()
		},
	})
}

func main() {
	fx.New(
		fx.Options(
			env.Module,
			Module,
		),
	).Run()
}


func newConfig(provider uber_config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

func newAuth() (auth.Auth, error) {
	return auth.DummyAuth{}, nil
}

func newScope() (tally.Scope, error) {
	s, _ := tally.NewRootScope(tally.ScopeOptions{
		Tags:            nil,
		Prefix:          "",
		Reporter:        nil,
		CachedReporter:  nil,
		Separator:       "",
		DefaultBuckets:  nil,
		SanitizeOptions: nil,
		MetricsOption:   0,
	}, 0)
	return s, nil
}

func newRestConfig(conf Config) (*rest.Config, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	restConfig.QPS = conf.QPS
	restConfig.Burst = conf.Burst
	return restConfig, nil
}