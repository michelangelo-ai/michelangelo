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
		if hasChange := hasCRDSchemaChanges(localCRD, serverCRD); hasChange {
			metricsLogger.LogSchemaMismatch(name)
		} else {
			metricsLogger.LogSchemaMatch(name)
		}
	}

	return nil
}

// hasCRDSchemaChanges performs comprehensive comparison of CRD schemas
func hasCRDSchemaChanges(localCRD, serverCRD *apiextv1.CustomResourceDefinition) bool {
	// Compare basic spec fields
	if localCRD.Spec.Group != serverCRD.Spec.Group {
		return true
	}
	
	if !reflect.DeepEqual(localCRD.Spec.Names, serverCRD.Spec.Names) {
		return true
	}
	
	if localCRD.Spec.Scope != serverCRD.Spec.Scope {
		return true
	}
	
	if !reflect.DeepEqual(localCRD.Spec.Conversion, serverCRD.Spec.Conversion) {
		return true
	}
	
	if localCRD.Spec.PreserveUnknownFields != serverCRD.Spec.PreserveUnknownFields {
		return true
	}
	
	// Compare versions - this is the most complex part
	if len(localCRD.Spec.Versions) != len(serverCRD.Spec.Versions) {
		return true
	}
	
	// Create maps for version comparison
	localVersions := make(map[string]apiextv1.CustomResourceDefinitionVersion)
	serverVersions := make(map[string]apiextv1.CustomResourceDefinitionVersion)
	
	for _, v := range localCRD.Spec.Versions {
		localVersions[v.Name] = v
	}
	
	for _, v := range serverCRD.Spec.Versions {
		serverVersions[v.Name] = v
	}
	
	// Compare each version
	for versionName, localVersion := range localVersions {
		serverVersion, exists := serverVersions[versionName]
		if !exists {
			return true
		}
		
		if hasVersionSchemaChanges(localVersion, serverVersion) {
			return true
		}
	}
	
	return false
}

// hasVersionSchemaChanges compares individual version schemas recursively
func hasVersionSchemaChanges(localVersion, serverVersion apiextv1.CustomResourceDefinitionVersion) bool {
	// Compare basic version fields
	if localVersion.Name != serverVersion.Name {
		return true
	}
	
	if localVersion.Served != serverVersion.Served {
		return true
	}
	
	if localVersion.Storage != serverVersion.Storage {
		return true
	}
	
	if localVersion.Deprecated != serverVersion.Deprecated {
		return true
	}
	
	if localVersion.DeprecationWarning != nil && serverVersion.DeprecationWarning != nil {
		if *localVersion.DeprecationWarning != *serverVersion.DeprecationWarning {
			return true
		}
	} else if localVersion.DeprecationWarning != serverVersion.DeprecationWarning {
		return true
	}
	
	// Compare schemas recursively
	if localVersion.Schema != nil && serverVersion.Schema != nil {
		return hasJSONSchemaChanges(localVersion.Schema.OpenAPIV3Schema, serverVersion.Schema.OpenAPIV3Schema)
	} else if localVersion.Schema != serverVersion.Schema {
		return true
	}
	
	// Compare subresources
	if !reflect.DeepEqual(localVersion.Subresources, serverVersion.Subresources) {
		return true
	}
	
	// Compare additional printer columns
	if !reflect.DeepEqual(localVersion.AdditionalPrinterColumns, serverVersion.AdditionalPrinterColumns) {
		return true
	}
	
	return false
}

// hasJSONSchemaChanges recursively compares JSON schema structures
func hasJSONSchemaChanges(localSchema, serverSchema *apiextv1.JSONSchemaProps) bool {
	if localSchema == nil && serverSchema == nil {
		return false
	}
	
	if localSchema == nil || serverSchema == nil {
		return true
	}
	
	// Compare basic schema properties
	if localSchema.Type != serverSchema.Type {
		return true
	}
	
	if localSchema.Format != serverSchema.Format {
		return true
	}
	
	if localSchema.Title != serverSchema.Title {
		return true
	}
	
	if localSchema.Description != serverSchema.Description {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.Default, serverSchema.Default) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.Example, serverSchema.Example) {
		return true
	}
	
	// Compare numeric constraints
	if !reflect.DeepEqual(localSchema.Maximum, serverSchema.Maximum) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.Minimum, serverSchema.Minimum) {
		return true
	}
	
	if localSchema.ExclusiveMaximum != serverSchema.ExclusiveMaximum {
		return true
	}
	
	if localSchema.ExclusiveMinimum != serverSchema.ExclusiveMinimum {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MaxLength, serverSchema.MaxLength) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MinLength, serverSchema.MinLength) {
		return true
	}
	
	if localSchema.Pattern != serverSchema.Pattern {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MaxItems, serverSchema.MaxItems) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MinItems, serverSchema.MinItems) {
		return true
	}
	
	if localSchema.UniqueItems != serverSchema.UniqueItems {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MultipleOf, serverSchema.MultipleOf) {
		return true
	}
	
	// Compare array constraints
	if !reflect.DeepEqual(localSchema.MaxProperties, serverSchema.MaxProperties) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.MinProperties, serverSchema.MinProperties) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.Required, serverSchema.Required) {
		return true
	}
	
	if !reflect.DeepEqual(localSchema.Enum, serverSchema.Enum) {
		return true
	}
	
	// Compare items schema (for arrays)
	if localSchema.Items != nil && serverSchema.Items != nil {
		if localSchema.Items.Schema != nil && serverSchema.Items.Schema != nil {
			if hasJSONSchemaChanges(localSchema.Items.Schema, serverSchema.Items.Schema) {
				return true
			}
		} else if !reflect.DeepEqual(localSchema.Items, serverSchema.Items) {
			return true
		}
	} else if localSchema.Items != serverSchema.Items {
		return true
	}
	
	// Compare properties (for objects) - this is the key recursive part
	if len(localSchema.Properties) != len(serverSchema.Properties) {
		return true
	}
	
	for propName, localProp := range localSchema.Properties {
		serverProp, exists := serverSchema.Properties[propName]
		if !exists {
			return true
		}
		
		if hasJSONSchemaChanges(&localProp, &serverProp) {
			return true
		}
	}
	
	// Compare additional properties
	if localSchema.AdditionalProperties != nil && serverSchema.AdditionalProperties != nil {
		if localSchema.AdditionalProperties.Schema != nil && serverSchema.AdditionalProperties.Schema != nil {
			if hasJSONSchemaChanges(localSchema.AdditionalProperties.Schema, serverSchema.AdditionalProperties.Schema) {
				return true
			}
		} else if !reflect.DeepEqual(localSchema.AdditionalProperties, serverSchema.AdditionalProperties) {
			return true
		}
	} else if localSchema.AdditionalProperties != serverSchema.AdditionalProperties {
		return true
	}
	
	// Compare pattern properties
	if len(localSchema.PatternProperties) != len(serverSchema.PatternProperties) {
		return true
	}
	
	for pattern, localProp := range localSchema.PatternProperties {
		serverProp, exists := serverSchema.PatternProperties[pattern]
		if !exists {
			return true
		}
		
		if hasJSONSchemaChanges(&localProp, &serverProp) {
			return true
		}
	}
	
	// Compare dependencies
	if !reflect.DeepEqual(localSchema.Dependencies, serverSchema.Dependencies) {
		return true
	}
	
	// Compare allOf, anyOf, oneOf, not schemas
	if len(localSchema.AllOf) != len(serverSchema.AllOf) {
		return true
	}
	
	for i, localAllOf := range localSchema.AllOf {
		if i >= len(serverSchema.AllOf) || hasJSONSchemaChanges(&localAllOf, &serverSchema.AllOf[i]) {
			return true
		}
	}
	
	if len(localSchema.AnyOf) != len(serverSchema.AnyOf) {
		return true
	}
	
	for i, localAnyOf := range localSchema.AnyOf {
		if i >= len(serverSchema.AnyOf) || hasJSONSchemaChanges(&localAnyOf, &serverSchema.AnyOf[i]) {
			return true
		}
	}
	
	if len(localSchema.OneOf) != len(serverSchema.OneOf) {
		return true
	}
	
	for i, localOneOf := range localSchema.OneOf {
		if i >= len(serverSchema.OneOf) || hasJSONSchemaChanges(&localOneOf, &serverSchema.OneOf[i]) {
			return true
		}
	}
	
	if localSchema.Not != nil && serverSchema.Not != nil {
		if hasJSONSchemaChanges(localSchema.Not, serverSchema.Not) {
			return true
		}
	} else if localSchema.Not != serverSchema.Not {
		return true
	}
	
	// Compare external documentation
	if !reflect.DeepEqual(localSchema.ExternalDocs, serverSchema.ExternalDocs) {
		return true
	}
	
	return false
}
