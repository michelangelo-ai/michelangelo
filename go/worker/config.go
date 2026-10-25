package worker

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/peer"
	"go.uber.org/yarpc/peer/hostport"
	"go.uber.org/yarpc/transport/grpc"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/kubernetes"

	"github.com/michelangelo-ai/michelangelo/go/auth"
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

	Config Config
	// OutboundMiddleware is an optional unary outbound middleware applied to
	// the Michelangelo API client (e.g. to attach auth headers, see
	// auth.NewBearerOutboundMiddleware). It is injected by deployment-specific
	// modules; when absent, calls go out with no additional middleware.
	OutboundMiddleware middleware.UnaryOutbound `optional:"true"`
	// RunnerTokenInstalled is provided by RunnerTokenModule (see
	// NewRunnerTokenOutboundMiddleware) and is how NewYARPCDispatcher knows
	// that worker.runnerToken.* is actually wired, as opposed to some other
	// deployment-provided OutboundMiddleware occupying the slot.
	RunnerTokenInstalled *RunnerTokenInstalled `optional:"true"`
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
	if err := checkRunnerTokenWiring(p.Config, p.RunnerTokenInstalled != nil); err != nil {
		return nil, err
	}

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

	yarpcCfg := yarpc.Config{
		Name:      p.Config.MaAPIServiceName,
		Outbounds: yarpc.Outbounds{p.Config.MaAPIServiceName: {Unary: tran}},
	}

	// Apply the optional injected outbound middleware (e.g. auth).
	if p.OutboundMiddleware != nil {
		yarpcCfg.OutboundMiddleware = yarpc.OutboundMiddleware{Unary: p.OutboundMiddleware}
	}

	dispatcher := yarpc.NewDispatcher(yarpcCfg)

	if err := dispatcher.Start(); err != nil {
		return nil, err
	}

	return dispatcher, nil
}

// RunnerTokenMiddlewareParams collects NewRunnerTokenOutboundMiddleware's
// dependencies.
type RunnerTokenMiddlewareParams struct {
	fx.In

	Provider config.Provider
}

// RunnerTokenMiddlewareResult carries the optional outbound middleware for
// NewYARPCDispatcher's Params.OutboundMiddleware slot. Left nil when the
// runnerToken feature is off (the default): fx accepts a nil value for an
// optional interface output, and NewYARPCDispatcher's own
// `if p.OutboundMiddleware != nil` check then adds nothing, so a disabled
// worker's traffic -- and its need for cluster credentials -- is unchanged.
type RunnerTokenMiddlewareResult struct {
	fx.Out

	OutboundMiddleware middleware.UnaryOutbound `optional:"true"`
	// Installed is always provided alongside the middleware, so its presence
	// proves RunnerTokenModule is in the graph (see checkRunnerTokenWiring).
	Installed *RunnerTokenInstalled
}

// RunnerTokenInstalled marks that RunnerTokenModule is part of the fx graph.
// It carries no data; only its presence matters.
type RunnerTokenInstalled struct{}

// NewRunnerTokenOutboundMiddleware builds the per-project runner-token
// outbound middleware when worker.runnerToken.enabled, adapting
// runnertoken's Minter into auth.TokenSource for use with
// auth.NewBearerOutboundMiddleware -- the same middleware slot
// cmd/apiserver wires its inbound counterpart through (see go/auth).
func NewRunnerTokenOutboundMiddleware(p RunnerTokenMiddlewareParams) (RunnerTokenMiddlewareResult, error) {
	conf, err := NewConfig(p.Provider)
	if err != nil {
		return RunnerTokenMiddlewareResult{}, fmt.Errorf("worker: cannot parse %s configuration: %w", configKey, err)
	}
	if !conf.RunnerToken.Enabled {
		return RunnerTokenMiddlewareResult{Installed: &RunnerTokenInstalled{}}, nil
	}

	client, err := newKubernetesClient(p.Provider)
	if err != nil {
		return RunnerTokenMiddlewareResult{}, fmt.Errorf("worker: runner-token middleware needs a kubernetes client: %w", err)
	}
	minter := runnertoken.NewMinter(client, conf.RunnerToken)
	return RunnerTokenMiddlewareResult{
		OutboundMiddleware: auth.NewBearerOutboundMiddleware(runnertoken.NewTokenSource(minter)),
		Installed:          &RunnerTokenInstalled{},
	}, nil
}

// newKubernetesClient builds the client runner tokens are minted through. It
// is a variable so tests can substitute a fake clientset.
var newKubernetesClient = func(provider config.Provider) (kubernetes.Interface, error) {
	k8sConfig, err := baseconfig.GetK8sConfig(provider)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(k8sConfig)
}

// checkRunnerTokenWiring fails when worker.runnerToken.enabled is set but
// RunnerTokenModule is not in the graph. It checks provenance (the marker
// RunnerTokenModule provides) rather than whether the middleware slot is
// filled: a deployment that includes Module, supplies its own outbound
// middleware and leaves the module out would otherwise start fine and
// silently never use runner tokens while the config says it does. It runs in
// NewYARPCDispatcher, before the dispatcher is started.
func checkRunnerTokenWiring(conf Config, installed bool) error {
	if conf.RunnerToken.Enabled && !installed {
		return errors.New("worker: worker.runnerToken.enabled is true but worker.RunnerTokenModule is not installed; add it to the fx graph, or unset worker.runnerToken.enabled if you provide your own outbound middleware")
	}
	return nil
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
