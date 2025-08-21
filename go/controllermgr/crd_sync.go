package controllermgr

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/api/crd"
	"github.com/uber-go/tally"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// CRDSyncConfig configuration for CRD sync in controller manager
type CRDSyncConfig struct {
	SyncInterval time.Duration `yaml:"syncInterval"`
}

// CRDSyncParams parameters for CRD sync
type CRDSyncParams struct {
	fx.In
	Lifecycle fx.Lifecycle
	Config    *CRDSyncConfig
	Logger    *zap.Logger
	Gateway   crd.Gateway
	Scope     tally.Scope `optional:"true"`
}

// CRDSyncModule provides CRD sync functionality for controller manager
var CRDSyncModule = fx.Options(
	fx.Provide(newCRDSyncConfig),
	fx.Provide(crd.NewCRDGateway),
	fx.Invoke(startCRDSync),
)

func newCRDSyncConfig(provider config.Provider) (*CRDSyncConfig, error) {
	conf := CRDSyncConfig{
		SyncInterval: 5 * time.Minute, // default to 5 minutes
	}
	err := provider.Get("crdSync").Populate(&conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

func startCRDSync(p CRDSyncParams) error {
	logger := p.Logger.With(zap.String("module", "crd-sync"))
	logger.Info("Starting CRD schema comparison service", zap.Duration("interval", p.Config.SyncInterval))

	// Create metrics if scope is available
	var metrics *Metrics
	if p.Scope != nil {
		metrics = NewMetrics(p.Scope)
	}

	metricsLogger := NewMetricsLogger(logger, metrics)

	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Start periodic schema comparison
			go startPeriodicSchemaComparison(ctx, metricsLogger, p.Config, p.Gateway)
			return nil
		},
		OnStop: nil,
	})

	return nil
}

func startPeriodicSchemaComparison(ctx context.Context, metricsLogger *MetricsLogger, config *CRDSyncConfig, gateway crd.Gateway) {
	interval := config.SyncInterval

	metricsLogger.LogInfo("Starting periodic schema comparison", zap.Duration("interval", interval))

	// Perform initial comparison immediately
	performSchemaComparison(ctx, metricsLogger, gateway)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			metricsLogger.LogInfo("Stopping periodic schema comparison")
			return
		case <-ticker.C:
			performSchemaComparison(ctx, metricsLogger, gateway)
		}
	}
}

func performSchemaComparison(ctx context.Context, metricsLogger *MetricsLogger, gateway crd.Gateway) {
	startTime := time.Now()
	defer func() {
		if metricsLogger.metrics != nil {
			duration := time.Since(startTime)
			metricsLogger.metrics.RecordComparisonDuration(duration)
		}
	}()

	metricsLogger.LogDebug("Performing schema comparison")

	// Get local schemas from YAML
	localSchemas := make(map[string]*apiextv1.CustomResourceDefinition)
	for name, yamlStr := range v2pb.YamlSchemas {
		crd := apiextv1.CustomResourceDefinition{}
		err := yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlStr)).Decode(&crd)
		if err != nil {
			metricsLogger.LogError("Failed to deserialize CRD from yaml for comparison",
				zap.String("name", name), zap.Error(err))
			continue
		}
		localSchemas[crd.Name] = &crd
	}

	// Get all CRDs from API Server
	serverCRDs, err := gateway.List(ctx)
	if err != nil {
		metricsLogger.LogError("Failed to list CRDs from API Server", zap.Error(err))
		return
	}

	// Perform schema comparison
	if err := compareSchemasWithServerList(ctx, metricsLogger, localSchemas, serverCRDs); err != nil {
		metricsLogger.LogError("Failed to compare schemas with API Server", zap.Error(err))
	}
}

// compareSchemasWithServerList compares local schemas with API Server schemas without performing any updates
func compareSchemasWithServerList(ctx context.Context, metricsLogger *MetricsLogger,
	localSchemas map[string]*apiextv1.CustomResourceDefinition,
	serverCRDs *apiextv1.CustomResourceDefinitionList) error {

	// Create a map of server CRDs for quick lookup
	serverSchemas := make(map[string]*apiextv1.CustomResourceDefinition)
	for _, serverCRD := range serverCRDs.Items {
		serverSchemas[serverCRD.Name] = &serverCRD
	}

	// Compare each local schema with corresponding server schema
	for name, localCRD := range localSchemas {
		serverCRD, exists := serverSchemas[name]
		if !exists {
			metricsLogger.LogCRDNotFound(name)
			continue
		}

		// Compare schemas and emit metrics/logs for any mismatches
		if hasChange := !reflect.DeepEqual(serverCRD.Spec.Versions, localCRD.Spec.Versions); hasChange {
			metricsLogger.LogSchemaMismatch(name)
		} else {
			metricsLogger.LogSchemaMatch(name)
		}
	}

	return nil
}
