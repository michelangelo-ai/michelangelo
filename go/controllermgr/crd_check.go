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

// CRDCheckConfig configuration for CRD check in controller manager
type CRDCheckConfig struct {
	CheckInterval time.Duration `yaml:"checkInterval"`
}

// CRDCheckParams parameters for CRD check
type CRDCheckParams struct {
	fx.In
	Config  *CRDCheckConfig
	Logger  *zap.Logger
	Gateway crd.Gateway
}

// CRDCheckModule provides CRD check functionality for controller manager
var CRDCheckModule = fx.Options(
	fx.Provide(newCRDCheckConfig),
	fx.Provide(crd.NewCRDGateway),
	fx.Invoke(startCRDCheck),
)

func newCRDCheckConfig(provider config.Provider) (*CRDCheckConfig, error) {
	conf := CRDCheckConfig{
		CheckInterval: 5 * time.Minute, // default to 5 minutes
	}
	err := provider.Get("crdCheck").Populate(&conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

func startCRDCheck(p CRDCheckParams) error {
	logger := p.Logger.With(zap.String("module", "crd-check"))
	logger.Info("Starting CRD schema comparison service", zap.Duration("interval", p.Config.CheckInterval))

	// Start periodic schema comparison immediately
	go startPeriodicSchemaComparison(context.Background(), logger, p.Config, p.Gateway)

	return nil
}

func startPeriodicSchemaComparison(ctx context.Context, logger *zap.Logger, config *CRDCheckConfig, gateway crd.Gateway) {
	interval := config.CheckInterval

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

	// Create a map of server CRDs for easy lookup
	serverCRDMap := make(map[string]*apiextv1.CustomResourceDefinition)
	for i := range serverCRDs.Items {
		serverCRDMap[serverCRDs.Items[i].Name] = &serverCRDs.Items[i]
	}

	// Compare each local schema with server schema
	for crdName, localCRD := range localSchemas {
		if serverCRD, exists := serverCRDMap[crdName]; exists {
			// CRD exists on server, compare schemas
			compareAndLogDifferences(logger, crdName, localCRD, serverCRD)
		} else {
			// CRD missing on server
			logger.Warn("CRD missing on server",
				zap.String("crd_name", crdName),
				zap.String("group", localCRD.Spec.Group),
				zap.String("kind", localCRD.Spec.Names.Kind))
		}
	}

	// Check for extra CRDs on server that are not in local schemas
	for crdName, serverCRD := range serverCRDMap {
		if _, exists := localSchemas[crdName]; !exists {
			logger.Warn("Extra CRD on server not in local schemas",
				zap.String("crd_name", crdName),
				zap.String("group", serverCRD.Spec.Group),
				zap.String("kind", serverCRD.Spec.Names.Kind))
		}
	}

	return nil
}

// compareAndLogDifferences performs detailed comparison and logs specific differences
func compareAndLogDifferences(logger *zap.Logger, crdName string, localCRD, serverCRD *apiextv1.CustomResourceDefinition) {
	// Use reflect.DeepEqual for the overall comparison first
	if reflect.DeepEqual(localCRD.Spec, serverCRD.Spec) {
		return // No differences, early return
	}

	// If there are differences, perform detailed field-by-field comparison for logging
	hasDifferences := false

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

	if !reflect.DeepEqual(localCRD.Spec.Conversion, serverCRD.Spec.Conversion) {
		logger.Warn("CRD conversion settings mismatch",
			zap.String("crd_name", crdName),
			zap.String("field", "spec.conversion"))
		hasDifferences = true
	}

	if !reflect.DeepEqual(localCRD.Spec.Versions, serverCRD.Spec.Versions) {
		if compareVersions(logger, crdName, localCRD.Spec.Versions, serverCRD.Spec.Versions) {
			hasDifferences = true
		}
	}

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
		if serverVersion, exists := serverVersionMap[versionName]; exists {
			// Compare version properties
			if localVersion.Served != serverVersion.Served {
				logger.Warn("Version served property mismatch",
					zap.String("crd_name", crdName),
					zap.String("version", versionName),
					zap.String("field", "spec.versions[].served"),
					zap.Bool("local", localVersion.Served),
					zap.Bool("server", serverVersion.Served))
				hasDifferences = true
			}

			if localVersion.Storage != serverVersion.Storage {
				logger.Warn("Version storage property mismatch",
					zap.String("crd_name", crdName),
					zap.String("version", versionName),
					zap.String("field", "spec.versions[].storage"),
					zap.Bool("local", localVersion.Storage),
					zap.Bool("server", serverVersion.Storage))
				hasDifferences = true
			}

			// Compare schemas
			if !reflect.DeepEqual(localVersion.Schema, serverVersion.Schema) {
				logger.Warn("Version schema mismatch",
					zap.String("crd_name", crdName),
					zap.String("version", versionName),
					zap.String("field", "spec.versions[].schema"))
				hasDifferences = true
			}

			// Compare additional printer columns
			if !reflect.DeepEqual(localVersion.AdditionalPrinterColumns, serverVersion.AdditionalPrinterColumns) {
				logger.Warn("Version additional printer columns mismatch",
					zap.String("crd_name", crdName),
					zap.String("version", versionName),
					zap.String("field", "spec.versions[].additionalPrinterColumns"))
				hasDifferences = true
			}

			// Compare subresources
			if !reflect.DeepEqual(localVersion.Subresources, serverVersion.Subresources) {
				logger.Warn("Version subresources mismatch",
					zap.String("crd_name", crdName),
					zap.String("version", versionName),
					zap.String("field", "spec.versions[].subresources"))
				hasDifferences = true
			}
		}
	}

	return hasDifferences
}
