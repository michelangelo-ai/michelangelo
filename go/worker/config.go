package worker

import (
	"crypto/tls"
	"fmt"
	"strings"

	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/peer"
	"go.uber.org/yarpc/peer/hostport"
	"go.uber.org/yarpc/transport/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/kubernetes"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/worker/runnertoken"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const configKey = "worker"

// Config represents the worker YARPC configuration.
type Config struct {
	MaAPIServiceName string `yaml:"maApiServiceName"`
	Address          string `yaml:"address"`
	UseTLS           bool   `yaml:"useTLS"`
	// RunnerToken, when enabled, attaches per-project michelangelo-runner
	// bearer tokens to outbound RPCs; disabled (the default) the worker's
	// traffic is unchanged.
	RunnerToken runnertoken.Config `yaml:"runnerToken"`
}

// Params provides dependencies for YARPC dispatcher.
type Params struct {
	fx.In

	Config   Config
	Provider config.Provider
}

// ClientParams provides dependencies for creating YARPC clients.
type ClientParams struct {
	fx.In

	Dispatcher *yarpc.Dispatcher
	Config     Config
}

// NewConfig creates a new Config from a provider.
func NewConfig(provider config.Provider) (Config, error) {
	var conf Config
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

// NewYARPCDispatcher creates and starts a new YARPC dispatcher.
func NewYARPCDispatcher(p Params) (*yarpc.Dispatcher, error) {
	var tran transport.UnaryOutbound

	// Check config to determine if we should use TLS
	if p.Config.UseTLS {
		// Configure TLS for secure connections (e.g., ingress endpoints)
		tlsConfig := &tls.Config{
			ServerName: extractServerName(p.Config.Address),
		}
		creds := credentials.NewTLS(tlsConfig)

		// Create a dialer with TLS credentials
		dialer := grpc.NewTransport().NewDialer(grpc.DialerCredentials(creds))

		// Create a peer chooser with the TLS-enabled dialer
		chooser := peer.NewSingle(
			hostport.Identify(p.Config.Address),
			dialer,
		)

		// Create outbound with the chooser
		tran = grpc.NewTransport().NewOutbound(chooser)
	} else {
		// Use insecure connection for local development
		tran = grpc.NewTransport().NewSingleOutbound(p.Config.Address)
	}

	yarpcConfig := yarpc.Config{
		Name:      p.Config.MaAPIServiceName,
		Outbounds: yarpc.Outbounds{p.Config.MaAPIServiceName: {Unary: tran}},
	}
	if p.Config.RunnerToken.Enabled {
		outboundMiddleware, err := newRunnerTokenMiddleware(p)
		if err != nil {
			return nil, err
		}
		yarpcConfig.OutboundMiddleware = yarpc.OutboundMiddleware{Unary: outboundMiddleware}
	}

	dispatcher := yarpc.NewDispatcher(yarpcConfig)

	if err := dispatcher.Start(); err != nil {
		return nil, err
	}

	return dispatcher, nil
}

// newRunnerTokenMiddleware builds the bearer-token middleware; the
// Kubernetes client is constructed here, only when the feature is on, so a
// disabled worker needs no cluster credentials.
func newRunnerTokenMiddleware(p Params) (runnertoken.OutboundMiddleware, error) {
	k8sConfig, err := baseconfig.GetK8sConfig(p.Provider)
	if err != nil {
		return runnertoken.OutboundMiddleware{}, fmt.Errorf("worker: runner-token middleware needs a kubernetes client: %w", err)
	}
	client, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return runnertoken.OutboundMiddleware{}, fmt.Errorf("worker: runner-token middleware needs a kubernetes client: %w", err)
	}
	return runnertoken.NewOutboundMiddleware(runnertoken.NewMinter(client, p.Config.RunnerToken)), nil
}

// NewRayClusterServiceClient creates a RayClusterService YARPC client.
func NewRayClusterServiceClient(p ClientParams) v2pb.RayClusterServiceYARPCClient {
	return v2pb.NewRayClusterServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewRayJobServiceClient creates a RayJobService YARPC client.
func NewRayJobServiceClient(p ClientParams) v2pb.RayJobServiceYARPCClient {
	return v2pb.NewRayJobServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewSparkJobServiceClient creates a SparkJobService YARPC client.
func NewSparkJobServiceClient(p ClientParams) v2pb.SparkJobServiceYARPCClient {
	return v2pb.NewSparkJobServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewCachedOutputServiceClient creates a CachedOutputService YARPC client.
func NewCachedOutputServiceClient(p ClientParams) v2pb.CachedOutputServiceYARPCClient {
	return v2pb.NewCachedOutputServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewPipelineRunServiceClient creates a PipelineRunService YARPC client.
func NewPipelineRunServiceClient(p ClientParams) v2pb.PipelineRunServiceYARPCClient {
	return v2pb.NewPipelineRunServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewModelServiceClient creates a ModelService YARPC client.
func NewModelServiceClient(p ClientParams) v2pb.ModelServiceYARPCClient {
	return v2pb.NewModelServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// NewDeploymentServiceClient creates a DeploymentService YARPC client.
func NewDeploymentServiceClient(p ClientParams) v2pb.DeploymentServiceYARPCClient {
	return v2pb.NewDeploymentServiceYARPCClient(p.Dispatcher.ClientConfig(p.Config.MaAPIServiceName))
}

// extractServerName extracts the server name from an address for TLS SNI
func extractServerName(address string) string {
	// Extract hostname from "hostname:port" format
	if idx := strings.LastIndex(address, ":"); idx >= 0 {
		return address[:idx]
	}
	return address
}
