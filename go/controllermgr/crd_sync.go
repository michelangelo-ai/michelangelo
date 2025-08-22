package controllermgr

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/api/crd"
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
	Config  *CRDSyncConfig
	Logger  *zap.Logger
	Gateway crd.Gateway
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

	// Start periodic schema comparison immediately
	go startPeriodicSchemaComparison(context.Background(), logger, p.Config, p.Gateway)
	
	return nil
}

func startPeriodicSchemaComparison(ctx context.Context, logger *zap.Logger, config *CRDSyncConfig, gateway crd.Gateway) {
	interval := config.SyncInterval

	logger.Info("Starting periodic schema comparison", zap.Duration("interval", interval))

	// Perform initial comparison immediately
	performSchemaComparison(ctx, logger, gateway)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping periodic schema comparison")
			return
		case <-ticker.C:
			performSchemaComparison(ctx, logger, gateway)
		}
	}
}

func performSchemaComparison(ctx context.Context, logger *zap.Logger, gateway crd.Gateway) {
	logger.Debug("Performing schema comparison")

	// Get local schemas from YAML
	localSchemas := make(map[string]*apiextv1.CustomResourceDefinition)
	for name, yamlStr := range v2pb.YamlSchemas {
		crd := apiextv1.CustomResourceDefinition{}
		err := yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlStr)).Decode(&crd)
		if err != nil {
			logger.Error("Failed to deserialize CRD from yaml for comparison",
				zap.String("name", name), zap.Error(err))
			continue
		}
		localSchemas[crd.Name] = &crd
	}

	// Get all CRDs from API Server
	serverCRDs, err := gateway.List(ctx)
	if err != nil {
		logger.Error("Failed to list CRDs from API Server", zap.Error(err))
		return
	}

	// Perform schema comparison
	if err := compareSchemasWithServerList(ctx, logger, localSchemas, serverCRDs); err != nil {
		logger.Error("Failed to compare schemas with API Server", zap.Error(err))
	}
}

// compareSchemasWithServerList compares local schemas with API Server schemas without performing any updates
func compareSchemasWithServerList(ctx context.Context, logger *zap.Logger,
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
			continue
		}

		// Perform detailed comparison and log specific differences
		compareAndLogDifferences(logger, name, localCRD, serverCRD)
	}

	return nil
}

// compareAndLogDifferences performs detailed comparison and logs specific differences
func compareAndLogDifferences(logger *zap.Logger, crdName string, localCRD, serverCRD *apiextv1.CustomResourceDefinition) {
	hasDifferences := false
	
	// Compare basic CRD spec fields
	if localCRD.Spec.Group != serverCRD.Spec.Group {
		logger.Warn("CRD group mismatch",
			zap.String("crd_name", crdName),
			zap.String("field", "spec.group"),
			zap.String("local", localCRD.Spec.Group),
			zap.String("server", serverCRD.Spec.Group))
		hasDifferences = true
	}
	
	if localCRD.Spec.Scope != serverCRD.Spec.Scope {
		logger.Warn("CRD scope mismatch",
			zap.String("crd_name", crdName),
			zap.String("field", "spec.scope"),
			zap.String("local", string(localCRD.Spec.Scope)),
			zap.String("server", string(serverCRD.Spec.Scope)))
		hasDifferences = true
	}
	
	// Compare Names
	if !reflect.DeepEqual(localCRD.Spec.Names, serverCRD.Spec.Names) {
		if localCRD.Spec.Names.Kind != serverCRD.Spec.Names.Kind {
			logger.Warn("CRD kind mismatch",
				zap.String("crd_name", crdName),
				zap.String("field", "spec.names.kind"),
				zap.String("local", localCRD.Spec.Names.Kind),
				zap.String("server", serverCRD.Spec.Names.Kind))
		}
		if localCRD.Spec.Names.Plural != serverCRD.Spec.Names.Plural {
			logger.Warn("CRD plural name mismatch",
				zap.String("crd_name", crdName),
				zap.String("field", "spec.names.plural"),
				zap.String("local", localCRD.Spec.Names.Plural),
				zap.String("server", serverCRD.Spec.Names.Plural))
		}
		if localCRD.Spec.Names.Singular != serverCRD.Spec.Names.Singular {
			logger.Warn("CRD singular name mismatch",
				zap.String("crd_name", crdName),
				zap.String("field", "spec.names.singular"),
				zap.String("local", localCRD.Spec.Names.Singular),
				zap.String("server", serverCRD.Spec.Names.Singular))
		}
		hasDifferences = true
	}
	
	// Compare Conversion settings
	if !reflect.DeepEqual(localCRD.Spec.Conversion, serverCRD.Spec.Conversion) {
		logger.Warn("CRD conversion settings mismatch",
			zap.String("crd_name", crdName),
			zap.String("field", "spec.conversion"))
		hasDifferences = true
	}
	
	// Compare versions in detail
	if compareVersions(logger, crdName, localCRD.Spec.Versions, serverCRD.Spec.Versions) {
		hasDifferences = true
	}
	
	// Log overall status if any differences found
	if hasDifferences {
		logger.Warn("CRD schema differences detected", zap.String("crd_name", crdName))
	}
}

// compareVersions compares version arrays and logs specific version differences
func compareVersions(logger *zap.Logger, crdName string, localVersions, serverVersions []apiextv1.CustomResourceDefinitionVersion) bool {
	hasDifferences := false
	
	// Create maps for easy lookup
	localVersionMap := make(map[string]apiextv1.CustomResourceDefinitionVersion)
	serverVersionMap := make(map[string]apiextv1.CustomResourceDefinitionVersion)
	
	for _, v := range localVersions {
		localVersionMap[v.Name] = v
	}
	for _, v := range serverVersions {
		serverVersionMap[v.Name] = v
	}
	
	// Check for missing versions on server
	for versionName := range localVersionMap {
		if _, exists := serverVersionMap[versionName]; !exists {
			logger.Warn("Version missing on server",
				zap.String("crd_name", crdName),
				zap.String("version", versionName),
				zap.String("field", "spec.versions"))
			hasDifferences = true
		}
	}
	
	// Check for extra versions on server
	for versionName := range serverVersionMap {
		if _, exists := localVersionMap[versionName]; !exists {
			logger.Warn("Extra version on server",
				zap.String("crd_name", crdName),
				zap.String("version", versionName),
				zap.String("field", "spec.versions"))
			hasDifferences = true
		}
	}
	
	// Compare each version that exists in both
	for versionName, localVersion := range localVersionMap {
		serverVersion, exists := serverVersionMap[versionName]
		if !exists {
			continue // Already logged as missing
		}
		
		if compareVersionDetails(logger, crdName, versionName, localVersion, serverVersion) {
			hasDifferences = true
		}
	}
	
	return hasDifferences
}

// compareVersionDetails compares individual version properties
func compareVersionDetails(logger *zap.Logger, crdName, versionName string, localVersion, serverVersion apiextv1.CustomResourceDefinitionVersion) bool {
	hasDifferences := false
	
	if localVersion.Served != serverVersion.Served {
		logger.Warn("Version served status mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "served"),
			zap.Bool("local", localVersion.Served),
			zap.Bool("server", serverVersion.Served))
		hasDifferences = true
	}
	
	if localVersion.Storage != serverVersion.Storage {
		logger.Warn("Version storage status mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "storage"),
			zap.Bool("local", localVersion.Storage),
			zap.Bool("server", serverVersion.Storage))
		hasDifferences = true
	}
	
	if localVersion.Deprecated != serverVersion.Deprecated {
		logger.Warn("Version deprecated status mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "deprecated"),
			zap.Bool("local", localVersion.Deprecated),
			zap.Bool("server", serverVersion.Deprecated))
		hasDifferences = true
	}
	
	// Compare deprecation warnings
	localDepWarning := ""
	serverDepWarning := ""
	if localVersion.DeprecationWarning != nil {
		localDepWarning = *localVersion.DeprecationWarning
	}
	if serverVersion.DeprecationWarning != nil {
		serverDepWarning = *serverVersion.DeprecationWarning
	}
	if localDepWarning != serverDepWarning {
		logger.Warn("Version deprecation warning mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "deprecationWarning"),
			zap.String("local", localDepWarning),
			zap.String("server", serverDepWarning))
		hasDifferences = true
	}
	
	// Compare schemas
	if !reflect.DeepEqual(localVersion.Schema, serverVersion.Schema) {
		logger.Warn("Version schema mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema"),
			zap.String("details", "OpenAPI v3 schema differs"))
		hasDifferences = true
		
		// Could add more detailed schema comparison here if needed
		compareSchemaDetails(logger, crdName, versionName, localVersion.Schema, serverVersion.Schema)
	}
	
	// Compare subresources
	if !reflect.DeepEqual(localVersion.Subresources, serverVersion.Subresources) {
		logger.Warn("Version subresources mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "subresources"))
		hasDifferences = true
	}
	
	// Compare additional printer columns
	if !reflect.DeepEqual(localVersion.AdditionalPrinterColumns, serverVersion.AdditionalPrinterColumns) {
		logger.Warn("Version additional printer columns mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "additionalPrinterColumns"))
		hasDifferences = true
	}
	
	return hasDifferences
}

// compareSchemaDetails provides additional schema comparison details
func compareSchemaDetails(logger *zap.Logger, crdName, versionName string, localSchema, serverSchema *apiextv1.CustomResourceValidation) {
	if localSchema == nil && serverSchema == nil {
		return
	}
	
	if localSchema == nil {
		logger.Warn("Local schema is nil but server has schema",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.openAPIV3Schema"))
		return
	}
	
	if serverSchema == nil {
		logger.Warn("Server schema is nil but local has schema",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.openAPIV3Schema"))
		return
	}
	
	if localSchema.OpenAPIV3Schema == nil && serverSchema.OpenAPIV3Schema == nil {
		return
	}
	
	if localSchema.OpenAPIV3Schema == nil {
		logger.Warn("Local OpenAPI schema is nil but server has schema",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.openAPIV3Schema"))
		return
	}
	
	if serverSchema.OpenAPIV3Schema == nil {
		logger.Warn("Server OpenAPI schema is nil but local has schema",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.openAPIV3Schema"))
		return
	}
	
	// Compare basic schema properties
	localS := localSchema.OpenAPIV3Schema
	serverS := serverSchema.OpenAPIV3Schema
	
	if localS.Type != serverS.Type {
		logger.Warn("Schema type mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.type"),
			zap.String("local", localS.Type),
			zap.String("server", serverS.Type))
	}
	
	if len(localS.Properties) != len(serverS.Properties) {
		logger.Warn("Schema properties count mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.properties"),
			zap.Int("local_count", len(localS.Properties)),
			zap.Int("server_count", len(serverS.Properties)))
	}
	
	// Compare required fields
	if !reflect.DeepEqual(localS.Required, serverS.Required) {
		logger.Warn("Schema required fields mismatch",
			zap.String("crd_name", crdName),
			zap.String("version", versionName),
			zap.String("field", "schema.required"),
			zap.Strings("local", localS.Required),
			zap.Strings("server", serverS.Required))
	}
}



