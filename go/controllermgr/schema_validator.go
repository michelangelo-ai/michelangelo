package controllermgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SchemaErrorType represents different types of schema compatibility errors
type SchemaErrorType string

// ResourceValidator is a function type for custom resource validation
// It takes an unstructured resource and returns whether it's valid and any error encountered
type ResourceValidator func(item *unstructured.Unstructured, logger *zap.Logger) (bool, error)

const (
	SchemaErrorUnknownEnum     SchemaErrorType = "unknown_enum"
	SchemaErrorUnknownField    SchemaErrorType = "unknown_field"
	SchemaErrorUnmarshal       SchemaErrorType = "unmarshal_failure"
	SchemaErrorUnknownType     SchemaErrorType = "unknown_type"
	SchemaErrorDecoding        SchemaErrorType = "decoding_failure"
	SchemaErrorVersionMismatch SchemaErrorType = "version_mismatch"
)

// schemaErrorPatterns maps error patterns to their corresponding error types
var schemaErrorPatterns = map[SchemaErrorType][]string{
	SchemaErrorUnknownEnum: {
		"unknown value",
		"for enum",
	},
	SchemaErrorUnknownField: {
		"unknown field",
		"unrecognized field",
		"proto: unknown field",
	},
	SchemaErrorUnmarshal: {
		"failed to unmarshal",
		"cannot unmarshal",
		"unmarshal error",
	},
	SchemaErrorUnknownType: {
		"no kind is registered",
		"unknown type",
	},
	SchemaErrorDecoding: {
		"strict decoding error",
		"decoding failure",
		"serialization error",
	},
	SchemaErrorVersionMismatch: {
		"version mismatch",
		"unsupported version",
		"api version",
	},
}

// SchemaValidatorInterface defines the interface for schema validation
type SchemaValidatorInterface interface {
	IsSchemaCompatibilityError(err error) SchemaErrorType
	MonitorResourceSchemaHealth(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger)
}

// schemaValidator implements SchemaValidatorInterface
type schemaValidator struct {
	validators map[string]ResourceValidator // Map of GVR string to custom validator
}

var (
	// SchemaValidator provides centralized schema validation for all controllers
	SchemaValidator = &schemaValidator{
		validators: make(map[string]ResourceValidator),
	}
)

// registerValidator registers a custom validator for a specific resource type
func (sv *schemaValidator) registerValidator(gvr schema.GroupVersionResource, validator ResourceValidator) {
	gvrKey := fmt.Sprintf("%s.%s.%s", gvr.Resource, gvr.Version, gvr.Group)
	sv.validators[gvrKey] = validator
}

// IsSchemaCompatibilityError checks if an error is due to schema compatibility issues
func (sv *schemaValidator) IsSchemaCompatibilityError(err error) SchemaErrorType {
	if err == nil {
		return ""
	}

	errorStr := strings.ToLower(err.Error())

	// Check each error type pattern
	for errorType, patterns := range schemaErrorPatterns {
		// Check if all patterns match
		allPatternsMatch := true
		for _, pattern := range patterns {
			if !strings.Contains(errorStr, strings.ToLower(pattern)) {
				allPatternsMatch = false
				break
			}
		}
		if allPatternsMatch {
			return errorType
		}
	}

	return ""
}

// ExtractSchemaErrorValue extracts problematic values from schema error messages
func (sv *schemaValidator) ExtractSchemaErrorValue(errorMsg string, errorType SchemaErrorType) string {
	switch errorType {
	case SchemaErrorUnknownEnum:
		// Look for pattern: unknown value "VALUE" for enum
		if strings.Contains(errorMsg, "unknown value") && strings.Contains(errorMsg, "for enum") {
			start := strings.Index(errorMsg, "unknown value \"")
			if start != -1 {
				start += len("unknown value \"")
				end := strings.Index(errorMsg[start:], "\"")
				if end != -1 {
					return errorMsg[start : start+end]
				}
			}
		}
	case SchemaErrorUnknownField:
		// Look for pattern: unknown field "fieldname"
		if strings.Contains(errorMsg, "unknown field") {
			start := strings.Index(errorMsg, "unknown field \"")
			if start != -1 {
				start += len("unknown field \"")
				end := strings.Index(errorMsg[start:], "\"")
				if end != -1 {
					return errorMsg[start : start+end]
				}
			}
		}
	}
	return ""
}

// checkCacheSync performs cache sync check with timeout and optional schema error handling
func (sv *schemaValidator) checkCacheSync(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger, checkType string) bool {
	mgrCache := mgr.GetCache()

	if checkType == "initial" {
		// For initial check, use a timeout
		// Timeout recommendations based on entity volume:
		// - Small deployments (<1k entities): 10 minutes
		// - Medium deployments (1k-5k entities): 20 minutes
		// - Large deployments (5k-10k entities): 30 minutes
		// - Very large deployments (>10k entities): 60 minutes
		// Default: 30 minutes for most production scenarios
		syncTimeout := 10 * time.Second
		logger.Info("Cache sync timeout configured", zap.Duration("timeout", syncTimeout))
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()

		logger.Info("Waiting for resource informer to sync...", zap.String("resource", gvr.Resource))
		return mgrCache.WaitForCacheSync(ctx)
	} else {
		// For runtime checks, use background context (no timeout)
		ctx := context.Background()
		return mgrCache.WaitForCacheSync(ctx)
	}
}

// MonitorResourceSchemaHealth monitors schema health for a specific resource type
func (sv *schemaValidator) MonitorResourceSchemaHealth(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger) {
	// Wait for controller to start
	time.Sleep(2 * time.Second)

	logger.Info("Starting schema compatibility monitoring for resource",
		zap.String("resource", gvr.Resource),
		zap.String("group", gvr.Group),
		zap.String("version", gvr.Version))

	// Perform initial sync check
	if !sv.checkCacheSync(mgr, gvr, logger, "initial") {
		logger.Warn("SCHEMA ERROR DETECTED: Resource informer failed to sync!",
			zap.String("resource", gvr.Resource))
		logger.Warn("This indicates schema compatibility issues - running diagnostic...")
		sv.runSchemaValidationDiagnostic(mgr, gvr, logger)
	} else {
		logger.Info("Resource informer synced successfully - no schema errors",
			zap.String("resource", gvr.Resource))
		logger.Info("Diagnostic will not run since no errors were detected")
	}

	// Start continuous runtime monitoring to detect schema errors that occur after initialization
	go sv.startRuntimeSchemaMonitoring(mgr, gvr, logger)
}

// startRuntimeSchemaMonitoring continuously monitors for schema errors during runtime
func (sv *schemaValidator) startRuntimeSchemaMonitoring(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger) {
	// Monitor every 10 seconds for schema health
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logger.Info("Starting continuous runtime schema monitoring",
				zap.String("resource", gvr.Resource))

			// Use the shared cache sync check function
			if !sv.checkCacheSync(mgr, gvr, logger, "runtime") {
				logger.Error("RUNTIME SCHEMA ERROR DETECTED!",
					zap.String("resource", gvr.Resource))
				logger.Error("A problematic resource was likely added during runtime operation")
				logger.Error("Running diagnostic to identify the problematic resource...")

				// Run diagnostic to identify the specific problematic resource
				sv.runSchemaValidationDiagnostic(mgr, gvr, logger)
			}
		}
	}
}

// ValidateResourceSchema checks a single resource for schema issues
func (sv *schemaValidator) ValidateResourceSchema(item *unstructured.Unstructured, logger *zap.Logger) bool {
	name := item.GetName()
	namespace := item.GetNamespace()
	gvk := item.GroupVersionKind()

	logger.Info("Validating individual resource",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("gvk", gvk.String()))

	// Convert GVK to GVR for validator lookup (assuming Kind == Resource for most cases)
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind) + "s", // Basic pluralization
	}
	gvrKey := fmt.Sprintf("%s.%s.%s", gvr.Resource, gvr.Version, gvr.Group)

	// Check if we have a custom validator for this resource type
	if validator, exists := sv.validators[gvrKey]; exists {
		valid, err := validator(item, logger)
		if err != nil {
			// Analyze schema error type
			if schemaErrorType := sv.IsSchemaCompatibilityError(err); schemaErrorType != "" {
				logger.Error("PROBLEMATIC RESOURCE IDENTIFIED!",
					zap.String("name", name),
					zap.String("namespace", namespace),
					zap.String("gvk", gvk.String()),
					zap.String("schema_error_type", string(schemaErrorType)),
					zap.Error(err))
			} else {
				logger.Error("PROBLEMATIC RESOURCE IDENTIFIED!",
					zap.String("name", name),
					zap.String("namespace", namespace),
					zap.String("gvk", gvk.String()),
					zap.Error(err))
				logger.Info("This resource contains validation errors")
			}
		}
		return valid
	}

	// For other resource types, do basic validation
	jsonBytes, err := json.Marshal(item.Object)
	if err != nil {
		logger.Error("Failed to marshal resource to JSON",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.String("gvk", gvk.String()),
			zap.Error(err))
		return false
	}

	// Try basic JSON unmarshal to detect schema issues
	var testObj map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &testObj); err != nil {
		// Analyze schema error type
		if schemaErrorType := sv.IsSchemaCompatibilityError(err); schemaErrorType != "" {
			logger.Error("PROBLEMATIC RESOURCE IDENTIFIED!",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.String("gvk", gvk.String()),
				zap.String("schema_error_type", string(schemaErrorType)),
				zap.Error(err))

			return false
		} else {
			logger.Error("PROBLEMATIC RESOURCE IDENTIFIED!",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.String("gvk", gvk.String()),
				zap.Error(err))
			logger.Info("This resource contains validation errors")
			return false
		}
	}

	logger.Debug("Resource is valid",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("gvk", gvk.String()))
	return true
}

// runSchemaValidationDiagnostic runs diagnostic when schema errors are detected
func (sv *schemaValidator) runSchemaValidationDiagnostic(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger) {
	logger.Info("RUNNING SCHEMA VALIDATION DIAGNOSTIC DUE TO DETECTED SCHEMA ERROR",
		zap.String("resource", gvr.Resource))

	// Get dynamic client
	config := mgr.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create dynamic client for diagnostic", zap.Error(err))
		return
	}

	// Try to list resources
	result, err := dynamicClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logger.Error("CONFIRMED: Failed to list resources!",
			zap.String("resource", gvr.Resource),
			zap.Error(err))

		// Analyze schema error type and extract problematic values
		if schemaErrorType := sv.IsSchemaCompatibilityError(err); schemaErrorType != "" {
			logger.Error("SCHEMA COMPATIBILITY ISSUE IDENTIFIED!",
				zap.String("schema_error_type", string(schemaErrorType)),
				zap.String("resource", gvr.Resource))

			if problemValue := sv.ExtractSchemaErrorValue(err.Error(), schemaErrorType); problemValue != "" {
				logger.Error("PROBLEMATIC VALUE IDENTIFIED!", zap.String("problematic_value", problemValue))
				logger.Info("To find the problematic resource, run:")
				logger.Info(fmt.Sprintf("   kubectl get %s.%s.%s -A -o yaml | grep -C5 '%s'",
					gvr.Resource, gvr.Version, gvr.Group, problemValue))
			} else {
				logger.Info("To find the problematic resource, run:")
				logger.Info(fmt.Sprintf("   kubectl get %s.%s.%s -A -o yaml",
					gvr.Resource, gvr.Version, gvr.Group))
			}

		}
		return
	}

	// If we get here, list succeeded, so check individual resources
	logger.Info("Successfully listed resources",
		zap.String("resource", gvr.Resource),
		zap.Int("count", len(result.Items)))
	for _, item := range result.Items {
		sv.ValidateResourceSchema(&item, logger)
	}
}

// customWatchErrorHandler handles watch errors from the reflector with schema validation
func customWatchErrorHandler(r *cache.Reflector, err error, logger *zap.Logger, mgr manager.Manager, gvr schema.GroupVersionResource) {
	resourceName := fmt.Sprintf("%s.%s.%s", gvr.Resource, gvr.Version, gvr.Group)

	// Check if this is a schema compatibility error
	if schemaErrorType := SchemaValidator.IsSchemaCompatibilityError(err); schemaErrorType != "" {
		logger.Error("REFLECTOR SCHEMA ERROR DETECTED!",
			zap.String("resource", resourceName),
			zap.String("reflector_name", r.LastSyncResourceVersion()),
			zap.String("schema_error_type", string(schemaErrorType)),
			zap.Error(err))

		// Run diagnostic to identify the specific problematic resource
		logger.Error("RUNNING DIAGNOSTIC TO IDENTIFY PROBLEMATIC RESOURCE...",
			zap.String("resource", resourceName))
		SchemaValidator.runSchemaValidationDiagnostic(mgr, gvr, logger)

	} else {
		// For non-schema errors, log at debug level
		logger.Debug("Non-schema error in reflector",
			zap.String("resource", resourceName), zap.Error(err))
	}

	// Call default handler for standard error handling
	cache.DefaultWatchErrorHandler(r, err)
}

// SetupWatchErrorHandler configures custom error handling for any resource type informers
func SetupWatchErrorHandler(mgr manager.Manager, obj client.Object, gvr schema.GroupVersionResource, logger *zap.Logger) error {
	resourceName := fmt.Sprintf("%s.%s.%s", gvr.Resource, gvr.Version, gvr.Group)
	logger.Info("Starting watch error handler setup", zap.String("resource", resourceName))

	// Get the cache from the manager
	cacheInterface := mgr.GetCache()

	// Get informer for the resource
	ctx := context.Background()
	informer, err := cacheInterface.GetInformer(ctx, obj)
	if err != nil {
		logger.Error("Failed to get informer for watch error handler setup",
			zap.String("resource", resourceName), zap.Error(err))
		return err
	}

	// Try to cast to SharedIndexInformer to access SetWatchErrorHandler
	if sharedInformer, ok := informer.(cache.SharedIndexInformer); ok {
		// Set custom watch error handler
		err = sharedInformer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
			customWatchErrorHandler(r, err, logger, mgr, gvr)
		})
		if err != nil {
			logger.Error("Failed to set watch error handler for informer",
				zap.String("resource", resourceName), zap.Error(err))
			return err
		}
		logger.Info("Successfully configured custom watch error handler",
			zap.String("resource", resourceName))
	} else {
		logger.Warn("Informer does not support SetWatchErrorHandler - unable to configure custom error handling",
			zap.String("resource", resourceName))
		logger.Info("Watch error handler functionality will not be available",
			zap.String("resource", resourceName))
	}

	return nil
}

// SetupSchemaMonitoringForResource sets up both watch error handler and schema monitoring for a resource type
// This is a convenience function that controllers can call to enable comprehensive schema monitoring
func SetupSchemaMonitoringForResource(mgr manager.Manager, obj client.Object, gvr schema.GroupVersionResource, validator ResourceValidator, logger *zap.Logger) error {
	resourceName := fmt.Sprintf("%s.%s.%s", gvr.Resource, gvr.Version, gvr.Group)
	logger.Info("Setting up comprehensive schema monitoring", zap.String("resource", resourceName))

	// Register the custom validator for this resource type
	SchemaValidator.registerValidator(gvr, validator)

	// Set up watch error handler
	if err := SetupWatchErrorHandler(mgr, obj, gvr, logger); err != nil {
		logger.Error("Failed to setup watch error handler",
			zap.String("resource", resourceName), zap.Error(err))
		// Don't return error to avoid blocking controller startup - just log the issue
	}

	// Start schema health monitoring in background
	go MonitorResourceSchemaHealth(mgr, gvr, logger)

	logger.Info("Schema monitoring setup completed", zap.String("resource", resourceName))
	return nil
}

// MonitorResourceSchemaHealth monitors schema health for a specific resource type
func MonitorResourceSchemaHealth(mgr manager.Manager, gvr schema.GroupVersionResource, logger *zap.Logger) {
	SchemaValidator.MonitorResourceSchemaHealth(mgr, gvr, logger)
}
