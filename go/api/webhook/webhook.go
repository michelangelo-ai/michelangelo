package webhook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

const (
	moduleName         = "webhook"
	webhookConfigKey   = "webhook"
	serverStartTimeout = 2 * time.Second
)

// Module is the fx module for version conversion webhook server.
// K8s API server will call the webhook server to convert the CR object to the desired version.
// This is needed for supporting multiple CRD versions.
// The webhook server is started when the fx app starts.
// The webhook server is stopped when the fx app stops.
var Module = fx.Options(
	fx.Provide(
		parseConfig,
		getWebhookClientConfig,
	),
	fx.Invoke(StartWebhookServer),
)

// Configuration is the webhook server configuration.
type Configuration struct {
	// the host that the webhook server listens on
	Host string `yaml:"host"`
	// the port that the webhook server listens on
	Port int `yaml:"port"`
	// the directory that contains the https cert files, including:
	// - ca.crt (the CA certificate for client certificate verification)
	// - tls.crt (the server certificate)
	// - tls.key (the server key)
	CertDir string `yaml:"certDir"`
	// the url that the client will connect to
	URL string `yaml:"url"`
}

// Params is the fx parameters for the webhook server module.
type Params struct {
	fx.In

	Config  *Configuration
	Scheme  *runtime.Scheme
	Logger  *zap.Logger
	Metrics tally.Scope
}

// parseConfig parses the webhook server configuration.
func parseConfig(provider config.Provider) (*Configuration, error) {
	conf := Configuration{}
	err := provider.Get(webhookConfigKey).Populate(&conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

// getWebhookClientConfig returns the k8s WebhookClientConfig for the webhook server.
func getWebhookClientConfig(params Params) (*apiextv1.WebhookClientConfig, error) {
	url := params.Config.URL + "/convert"
	// read the ca.crt file from the cert dir
	caCertPath := filepath.Join(params.Config.CertDir, "ca.crt")
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate from %s: %w", caCertPath, err)
	}
	return &apiextv1.WebhookClientConfig{
		URL:      &url,
		CABundle: caCert,
	}, nil
}

// StartWebhookServer registers fx lifecycle hooks for the webhook server.
// When the fx app starts, it will start the webhook server.
// When the fx app stops, it will stop the webhook server.
func StartWebhookServer(lc fx.Lifecycle, params Params) {
	logger := params.Logger.With(zap.String("module", moduleName))
	logger.Info("webhook config", zap.Any("config", params.Config))
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			var err error
			logger.Info("starting webhook server...")
			err, cancel = startWebhookServer(params)
			return err
		},
		OnStop: func(ctx context.Context) error {
			if cancel != nil {
				logger.Info("stopping webhook server...")
				cancel()
			}
			return nil
		},
	})
}

func startWebhookServer(params Params) (error, context.CancelFunc) {
	server := &webhook.Server{
		Host:    params.Config.Host,
		Port:    params.Config.Port,
		CertDir: params.Config.CertDir,
	}

	conversionWebhook := &conversion.Webhook{}
	if err := conversionWebhook.InjectScheme(params.Scheme); err != nil {
		return err, nil
	}

	server.Register("/convert", conversionWebhook)

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- server.Start(cancelCtx)
	}()

	select {
	case err := <-errCh:
		// The server failed to start and returned an error immediately.
		if cancelFunc != nil {
			cancelFunc()
		}
		return fmt.Errorf("webhook server failed to start: %w", err), nil
	case <-time.After(serverStartTimeout):
		// Check if the server is running
		startedChecker := server.StartedChecker()
		if e := startedChecker(nil); e != nil {
			if cancelFunc != nil {
				cancelFunc()
			}
			return fmt.Errorf("webhook server failed to start in %v: %w", serverStartTimeout, e), nil
		}
		return nil, cancelFunc
	}
}
