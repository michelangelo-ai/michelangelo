package controllermgr

import (
	"context"
	"fmt"
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

// VersionInfo represents information about an API version
type VersionInfo struct {
	Group   string
	Version string
	CRDs    []*apiextv1.CustomResourceDefinition
}

func performSchemaComparison(ctx context.Context, logger *zap.Logger, gateway crd.Gateway) {
	logger.Debug("Performing schema comparison")

	// Step 1: Get all CRDs from API Server FIRST
	serverCRDs, err := gateway.List(ctx)
	if err != nil {
		logger.Error("Failed to list CRDs from API Server", zap.Error(err))
		return
	}

	// Step 2: Detect versions from server CRDs
	detectedVersions := detectVersionsFromServer(serverCRDs)
	logger.Info("Detected versions from server",
		zap.Int("version_count", len(detectedVersions)),
		zap.Any("versions", getVersionKeys(detectedVersions)))

	// Step 3: Load local schemas for detected versions
	localSchemasByVersion := loadLocalSchemasForVersions(detectedVersions, logger)

	// Step 4: Compare schemas for each detected version
	for versionKey, versionInfo := range detectedVersions {
		logger.Debug("Comparing schemas for version", zap.String("version", versionKey))

		localSchemas := localSchemasByVersion[versionKey]
		if len(localSchemas) == 0 {
			logger.Warn("No local schemas found for version", zap.String("version", versionKey))
			continue
		}

		// Compare schemas for this version
		compareSchemasForVersion(ctx, logger, localSchemas, versionInfo.CRDs, versionKey)
	}
}

// detectVersionsFromServer extracts version information from server CRDs
func detectVersionsFromServer(serverCRDs *apiextv1.CustomResourceDefinitionList) map[string]VersionInfo {
	versionMap := make(map[string]VersionInfo)

	for _, crd := range serverCRDs.Items {
		group := crd.Spec.Group
		for _, version := range crd.Spec.Versions {
			versionKey := fmt.Sprintf("%s/%s", group, version.Name)

			if _, exists := versionMap[versionKey]; !exists {
				versionMap[versionKey] = VersionInfo{
					Group:   group,
					Version: version.Name,
					CRDs:    make([]*apiextv1.CustomResourceDefinition, 0),
				}
			}

			// Add this CRD to the version's CRD list
			versionInfo := versionMap[versionKey]
			versionInfo.CRDs = append(versionInfo.CRDs, &crd)
			versionMap[versionKey] = versionInfo
		}
	}

	return versionMap
}

// getVersionKeys returns a slice of version keys for logging
func getVersionKeys(versionMap map[string]VersionInfo) []string {
	keys := make([]string, 0, len(versionMap))
	for key := range versionMap {
		keys = append(keys, key)
	}
	return keys
}

// loadLocalSchemasForVersions loads local YAML schemas based on detected versions
func loadLocalSchemasForVersions(detectedVersions map[string]VersionInfo, logger *zap.Logger) map[string]map[string]*apiextv1.CustomResourceDefinition {
	localSchemas := make(map[string]map[string]*apiextv1.CustomResourceDefinition)

	for versionKey, versionInfo := range detectedVersions {
		localSchemas[versionKey] = make(map[string]*apiextv1.CustomResourceDefinition)

		// Load schemas based on detected version
		switch versionInfo.Version {
		case "v2":
			loadV2Schemas(localSchemas[versionKey], logger)
		case "v3":
			// Future: Load v3 schemas when v3pb is available
			logger.Debug("v3 schemas not yet implemented", zap.String("version", versionKey))
		default:
			logger.Warn("Unknown version detected", zap.String("version", versionInfo.Version))
		}
	}

	return localSchemas
}

// loadV2Schemas loads v2 schemas from YAML
func loadV2Schemas(schemas map[string]*apiextv1.CustomResourceDefinition, logger *zap.Logger) {
	for name, yamlStr := range v2pb.YamlSchemas {
		crd := apiextv1.CustomResourceDefinition{}
		err := yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlStr)).Decode(&crd)
		if err != nil {
			logger.Error("Failed to deserialize CRD from yaml",
				zap.String("name", name), zap.Error(err))
			continue
		}
		schemas[crd.Name] = &crd
	}
}

// compareSchemasForVersion compares local and server schemas for a specific version
func compareSchemasForVersion(ctx context.Context, logger *zap.Logger,
	localSchemas map[string]*apiextv1.CustomResourceDefinition,
	serverCRDs []*apiextv1.CustomResourceDefinition,
	versionKey string) {

	// Create a map of server CRDs for easy lookup
	serverCRDMap := make(map[string]*apiextv1.CustomResourceDefinition)
	for _, crd := range serverCRDs {
		serverCRDMap[crd.Name] = crd
	}

	// Compare each local schema with server schema
	for crdName, localCRD := range localSchemas {
		if serverCRD, exists := serverCRDMap[crdName]; exists {
			// CRD exists on server, compare schemas
			compareAndLogDifferences(logger, crdName, localCRD, serverCRD, versionKey)
		} else {
			// CRD missing on server
			logger.Warn("CRD missing on server",
				zap.String("crd_name", crdName),
				zap.String("version", versionKey),
				zap.String("group", localCRD.Spec.Group),
				zap.String("kind", localCRD.Spec.Names.Kind))
		}
	}

	// Check for extra CRDs on server that are not in local schemas
	for crdName, serverCRD := range serverCRDMap {
		if _, exists := localSchemas[crdName]; !exists {
			logger.Warn("Extra CRD on server not in local schemas",
				zap.String("crd_name", crdName),
				zap.String("version", versionKey),
				zap.String("group", serverCRD.Spec.Group),
				zap.String("kind", serverCRD.Spec.Names.Kind))
		}
	}
}

// compareAndLogDifferences performs comparison using reflect.DeepEqual and logs differences
func compareAndLogDifferences(logger *zap.Logger, crdName string, localCRD, serverCRD *apiextv1.CustomResourceDefinition, versionKey string) {
	// Use reflect.DeepEqual for the overall comparison
	if reflect.DeepEqual(localCRD.Spec, serverCRD.Spec) {
		return // No differences, early return
	}

	// Log the difference with version information
	logger.Warn("CRD schema differences detected",
		zap.String("crd_name", crdName),
		zap.String("version", versionKey),
		zap.String("group", localCRD.Spec.Group),
		zap.String("kind", localCRD.Spec.Names.Kind))
}
