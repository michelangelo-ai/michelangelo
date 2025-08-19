package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	reconcileInterval = 10 * time.Second
)

// Reconciler is the output of NewReconciler.
type Reconciler struct {
	api.Handler
	env               env.Context
	logger            *zap.Logger
	apiHandlerFactory apiHandler.Factory
}

// Reconcile is the main entrypoint for the controller with enhanced schema error logging.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("Reconciling pipeline starts with enhanced schema validation")

	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Debug("Pipeline not found, ignoring")
			return ctrl.Result{}, nil
		}

		// Enhanced schema compatibility error detection
		if schemaErrorType := isSchemaCompatibilityError(err); schemaErrorType != "" {
			logger.Error("SCHEMA COMPATIBILITY ERROR detected during pipeline get operation!")
			logger.Error("Error details:", zap.Error(err))
			logger.Error("Schema error type:", zap.String("error_type", string(schemaErrorType)))
			logger.Error("This indicates the Pipeline resource has schema compatibility issues")
			logger.Error("Triggering diagnostic to identify problematic resources...")
			
			// Trigger diagnostic when schema error is detected
			go r.diagnosePipelineResourcesOnError(req.Namespace, req.Name, schemaErrorType)
		} else {
			logger.Error("Failed to get pipeline (non-schema error)", zap.Error(err))
		}

		return ctrl.Result{}, err
	}

	originalPipeline := pipeline.DeepCopy()
	state := pipeline.Status.State
	logger.Info("Successfully retrieved pipeline",
		zap.Any("PipelineStatusState", state.String()),
		zap.String("pipeline", pipeline.Name))

	pipeline.Status.LatestRevision = &apipb.ResourceIdentifier{
		Name:      formatRevisionName(pipeline),
		Namespace: pipeline.Namespace,
	}
	pipeline.Status.State = v2pb.PIPELINE_STATE_READY
	return r.updatePipelineStatus(ctx, pipeline, originalPipeline, logger)
}

func (r *Reconciler) updatePipelineStatus(ctx context.Context, pipeline *v2pb.Pipeline, originalPipeline *v2pb.Pipeline, logger *zap.Logger) (ctrl.Result, error) {
	result := ctrl.Result{}
	if !isTerminatedState(pipeline.Status.State) {
		result = ctrl.Result{RequeueAfter: reconcileInterval}
	}
	if !reflect.DeepEqual(originalPipeline.Status, pipeline.Status) {
		logger.Info("Pipeline status updated", zap.Any("PipelineStatusState", pipeline.Status.State.String()))
		err := r.UpdateStatus(ctx, pipeline, &metav1.UpdateOptions{})
		if err != nil {
			logger.Error("Failed to update pipeline status", zap.Error(err))
			return result, err
		}
	}

	return result, nil
}

func formatRevisionName(pipeline *v2pb.Pipeline) string {
	return fmt.Sprintf("%s-%s-%s", "pipeline", strings.ToLower(pipeline.Name), pipeline.Spec.Commit.GitRef[:min(len(pipeline.Spec.Commit.GitRef), 12)])
}

func isTerminatedState(state v2pb.PipelineState) bool {
	return state == v2pb.PIPELINE_STATE_READY ||
		state == v2pb.PIPELINE_STATE_ERROR
}

// SchemaErrorType represents different types of schema compatibility errors
type SchemaErrorType string

const (
	SchemaErrorUnknownEnum   SchemaErrorType = "unknown_enum"
	SchemaErrorUnknownField  SchemaErrorType = "unknown_field"
	SchemaErrorUnmarshal     SchemaErrorType = "unmarshal_failure"
	SchemaErrorUnknownType   SchemaErrorType = "unknown_type"
	SchemaErrorDecoding      SchemaErrorType = "decoding_failure"
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

// isSchemaCompatibilityError checks if an error is due to schema compatibility issues
// and returns the specific error type if found, or empty string if not a schema error.
func isSchemaCompatibilityError(err error) SchemaErrorType {
	if err == nil {
		return ""
	}
	
	errorStr := strings.ToLower(err.Error())
	
	// Check each error type pattern
	for errorType, patterns := range schemaErrorPatterns {
		// First check if all patterns match (most specific)
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
		
		// Also check if any single pattern matches (for backwards compatibility)
		for _, pattern := range patterns {
			if strings.Contains(errorStr, strings.ToLower(pattern)) {
				return errorType
			}
		}
	}
	
	return ""
}

// extractSchemaErrorValue extracts problematic values from schema error messages
func extractSchemaErrorValue(errorMsg string, errorType SchemaErrorType) string {
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

// Register is used to register the controller with enhanced error logging.
func (r *Reconciler) Register(mgr ctrl.Manager) error {
	handler, err := r.apiHandlerFactory.GetAPIHandler(mgr.GetClient())
	if err != nil {
		return err
	}
	r.Handler = handler

	r.logger.Info("Registering Pipeline controller with SCHEMA-BASED DIAGNOSTIC")
	r.logger.Info("Diagnostic will ONLY run when schema compatibility errors are detected")

	// Use standard controller registration
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Pipeline{}).
		Complete(r)
	
	if err != nil {
		r.logger.Error("Pipeline controller registration failed", zap.Error(err))
		return err
	}
	
	// Monitor for schema compatibility errors after successful registration
	go r.monitorForSchemaErrors(mgr)
	
	r.logger.Info("Pipeline controller registered successfully")
	return nil
}

// monitorForSchemaErrors monitors for actual schema errors and triggers diagnostic only when needed
func (r *Reconciler) monitorForSchemaErrors(mgr ctrl.Manager) {
	// Wait for controller to start
	time.Sleep(2 * time.Second)
	
	r.logger.Info("Starting schema compatibility error monitoring...")
	
	// Check if Pipeline informer can sync (indicates schema health)
	cache := mgr.GetCache()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Try to wait for Pipeline informer to sync
	r.logger.Info("Waiting for Pipeline informer to sync...")
	
	// This will return false if informer fails due to schema errors
	if !cache.WaitForCacheSync(ctx) {
		r.logger.Error("SCHEMA ERROR DETECTED: Pipeline informer failed to sync!")
		r.logger.Error("This indicates schema compatibility issues - running diagnostic...")
		r.runSchemaValidationDiagnostic(mgr)
	} else {
		r.logger.Info("Pipeline informer synced successfully - no schema errors")
		r.logger.Info("Diagnostic will not run since no errors were detected")
	}
}

// runSchemaValidationDiagnostic runs diagnostic when schema errors are actually detected
func (r *Reconciler) runSchemaValidationDiagnostic(mgr ctrl.Manager) {
	r.logger.Info("RUNNING SCHEMA VALIDATION DIAGNOSTIC DUE TO DETECTED SCHEMA ERROR")
	
	// Get dynamic client
	config := mgr.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		r.logger.Error("Failed to create dynamic client for diagnostic", zap.Error(err))
		return
	}
	
	// Define the GVR for Pipeline
	gvr := schema.GroupVersionResource{
		Group:    "michelangelo.api",
		Version:  "v2",
		Resource: "pipelines",
	}
	
	// Try to list Pipeline resources
	result, err := dynamicClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		r.logger.Error("CONFIRMED: Failed to list Pipeline resources!", zap.Error(err))
		
		// Analyze schema error type and extract problematic values
		if schemaErrorType := isSchemaCompatibilityError(err); schemaErrorType != "" {
			r.logger.Error("SCHEMA COMPATIBILITY ISSUE IDENTIFIED!", 
				zap.String("schema_error_type", string(schemaErrorType)))
			
			if problemValue := extractSchemaErrorValue(err.Error(), schemaErrorType); problemValue != "" {
				r.logger.Error("PROBLEMATIC VALUE IDENTIFIED!", zap.String("problematic_value", problemValue))
				r.logger.Info("To find the problematic resource, run:")
				r.logger.Info(fmt.Sprintf("   kubectl get pipelines.v2.michelangelo.api -A -o yaml | grep -C5 '%s'", problemValue))
			} else {
				r.logger.Info("To find the problematic resource, run:")
				r.logger.Info("   kubectl get pipelines.v2.michelangelo.api -A -o yaml")
			}
			
			// Provide specific guidance based on error type
			r.provideSchemaErrorGuidance(schemaErrorType)
		}
		return
	}
	
	// If we get here, list succeeded, so check individual resources
	r.logger.Info("Successfully listed Pipeline resources", zap.Int("count", len(result.Items)))
	for _, item := range result.Items {
		r.validatePipelineResourceSchema(&item)
	}
}

// diagnosePipelineResourcesOnError identifies problematic resources when a schema error occurs
func (r *Reconciler) diagnosePipelineResourcesOnError(problemNamespace, problemName string, schemaErrorType SchemaErrorType) {
	r.logger.Info("Starting on-demand diagnostic for schema error...", 
		zap.String("triggered_by", fmt.Sprintf("%s/%s", problemNamespace, problemName)),
		zap.String("schema_error_type", string(schemaErrorType)))
	
	r.logger.Info("For immediate identification, run:")
	switch schemaErrorType {
	case SchemaErrorUnknownEnum:
		r.logger.Info(fmt.Sprintf("   kubectl get pipeline %s -n %s -o yaml | grep -E '.*_TYPE_.*'", problemName, problemNamespace))
	case SchemaErrorUnknownField:
		r.logger.Info(fmt.Sprintf("   kubectl get pipeline %s -n %s -o yaml", problemName, problemNamespace))
	default:
		r.logger.Info(fmt.Sprintf("   kubectl get pipeline %s -n %s -o yaml", problemName, problemNamespace))
	}
}

// provideSchemaErrorGuidance provides specific guidance based on schema error type
func (r *Reconciler) provideSchemaErrorGuidance(errorType SchemaErrorType) {
	switch errorType {
	case SchemaErrorUnknownEnum:
		r.logger.Info("GUIDANCE: Unknown enum value detected")
		r.logger.Info("- Check if the enum value is supported in your API version")
		r.logger.Info("- Consider updating the controller or API version")
		r.logger.Info("- Validate the resource definition against the schema")
	case SchemaErrorUnknownField:
		r.logger.Info("GUIDANCE: Unknown field detected")
		r.logger.Info("- Check if the field is supported in your API version")
		r.logger.Info("- Consider updating the controller or API version")
		r.logger.Info("- Remove unsupported fields from the resource")
	case SchemaErrorUnmarshal:
		r.logger.Info("GUIDANCE: Unmarshal error detected")
		r.logger.Info("- Check resource format and structure")
		r.logger.Info("- Validate JSON/YAML syntax")
		r.logger.Info("- Ensure data types match schema expectations")
	case SchemaErrorVersionMismatch:
		r.logger.Info("GUIDANCE: API version mismatch detected")
		r.logger.Info("- Check if resource API version is supported")
		r.logger.Info("- Consider updating the controller version")
		r.logger.Info("- Migrate resources to supported API version")
	default:
		r.logger.Info("GUIDANCE: General schema compatibility issue")
		r.logger.Info("- Validate resource against current schema")
		r.logger.Info("- Check controller and API versions compatibility")
		r.logger.Info("- Review recent schema changes")
	}
}

// diagnosePipelineResources identifies which specific Pipeline has problematic enum values (unused now)
func (r *Reconciler) diagnosePipelineResources(mgr ctrl.Manager) {
	// Wait a bit for manager to be ready
	time.Sleep(2 * time.Second)

	r.logger.Info("Starting diagnostic scan for problematic Pipeline resources...")

	// Get dynamic client to access raw JSON
	config := mgr.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		r.logger.Error("Failed to create dynamic client for diagnostics", zap.Error(err))
		return
	}

	// Define the GVR for Pipeline
	gvr := schema.GroupVersionResource{
		Group:    "michelangelo.api",
		Version:  "v2",
		Resource: "pipelines",
	}

	// List all Pipeline resources across all namespaces
	r.logger.Info("Listing all Pipeline resources to identify problematic ones...")

	result, err := dynamicClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		r.logger.Error("Failed to list Pipeline resources - this is likely the enum error!", zap.Error(err))
		r.logger.Info("Attempting individual resource inspection...")

		// Try to get individual resources to identify the problematic one
		r.inspectIndividualPipelines(dynamicClient, gvr)
		return
	}

	r.logger.Info("Successfully listed Pipeline resources", zap.Int("count", len(result.Items)))

	// Check each Pipeline individually for schema issues
	for _, item := range result.Items {
		r.validatePipelineResourceSchema(&item)
	}
}

// inspectIndividualPipelines tries to identify problematic resources when listing fails
func (r *Reconciler) inspectIndividualPipelines(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource) {
	r.logger.Info("Attempting to inspect individual Pipeline resources...")
	// This is a fallback - in practice, if listing fails due to enum errors,
	// we'd need to use kubectl or other tools to identify individual resources
	r.logger.Info("Since listing failed, use this command to identify problematic resources:")
	r.logger.Info("   kubectl get pipelines.v2.michelangelo.api -A -o yaml | grep -A5 -B5 'unknown.*enum'")
}

// validatePipelineResourceSchema checks a single Pipeline resource for schema issues
func (r *Reconciler) validatePipelineResourceSchema(item *unstructured.Unstructured) {
	name := item.GetName()
	namespace := item.GetNamespace()

	// Convert to JSON and try to unmarshal as Pipeline
	jsonBytes, err := json.Marshal(item.Object)
	if err != nil {
		r.logger.Error("Failed to marshal resource to JSON",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return
	}

	// Try to unmarshal into Pipeline struct
	var pipeline v2pb.Pipeline
	if err := json.Unmarshal(jsonBytes, &pipeline); err != nil {
		// Analyze schema error type
		if schemaErrorType := isSchemaCompatibilityError(err); schemaErrorType != "" {
			r.logger.Error("PROBLEMATIC PIPELINE RESOURCE IDENTIFIED!",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.String("schema_error_type", string(schemaErrorType)),
				zap.Error(err))
			
			r.logger.Info("This Pipeline resource contains schema compatibility issues")
			r.logger.Info("To fix, run:",
				zap.String("command", fmt.Sprintf("kubectl edit pipeline %s -n %s", name, namespace)))

			// Extract and report problematic value if possible
			if problemValue := extractSchemaErrorValue(err.Error(), schemaErrorType); problemValue != "" {
				r.logger.Error("Found problematic value in resource",
					zap.String("name", name),
					zap.String("namespace", namespace),
					zap.String("schema_error_type", string(schemaErrorType)),
					zap.String("problematic_value", problemValue))
			}
			
			// Provide specific guidance
			r.provideSchemaErrorGuidance(schemaErrorType)
		} else {
			r.logger.Error("PROBLEMATIC PIPELINE RESOURCE IDENTIFIED!",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.Error(err))
			r.logger.Info("This Pipeline resource contains validation errors")
		}
	} else {
		r.logger.Debug("Pipeline resource is valid",
			zap.String("name", name),
			zap.String("namespace", namespace))
	}
}
