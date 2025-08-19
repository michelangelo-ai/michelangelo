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

// Reconcile is the main entrypoint for the controller with enhanced enum error logging.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.With(zap.String("namespace-name", req.NamespacedName.String()))
	logger.Info("🔄 Reconciling pipeline starts with enhanced error detection")

	pipeline := &v2pb.Pipeline{}
	if err := r.Get(ctx, req.Namespace, req.Name, &metav1.GetOptions{}, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Debug("📭 Pipeline not found, ignoring")
			return ctrl.Result{}, nil
		}

		// Enhanced enum error detection
		if isEnumCompatibilityError(err) {
			logger.Error("🚨 ENUM COMPATIBILITY ERROR detected during pipeline get operation!")
			logger.Error("📋 Error details:", zap.Error(err))
			logger.Error("🎯 This indicates the Pipeline resource contains unknown enum values")
			logger.Error("💡 Triggering diagnostic to identify problematic resources...")
			
			// Trigger diagnostic only when enum error is detected
			go r.diagnosePipelineResourcesOnError(req.Namespace, req.Name)
		} else {
			logger.Error("❌ Failed to get pipeline (non-enum error)", zap.Error(err))
		}

		return ctrl.Result{}, err
	}

	originalPipeline := pipeline.DeepCopy()
	state := pipeline.Status.State
	logger.Info("✅ Successfully retrieved pipeline",
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

// isEnumCompatibilityError checks if error is related to enum compatibility
func isEnumCompatibilityError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	enumPatterns := []string{
		"unknown value",
		"for enum",
		"enum value",
	}

	for _, pattern := range enumPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// extractUnknownEnumValue extracts the unknown enum value from error messages
// Example: "unknown value \"PIPELINE_MANIFEST_TYPE_TEST\" for enum michelangelo.api.v2.PipelineManifest_Type"
// Returns: "PIPELINE_MANIFEST_TYPE_TEST"
func extractUnknownEnumValue(errorMsg string) string {
	// Look for pattern: unknown value "VALUE" for enum
	if strings.Contains(errorMsg, "unknown value") && strings.Contains(errorMsg, "for enum") {
		// Find the quoted value after "unknown value"
		start := strings.Index(errorMsg, "unknown value \"")
		if start != -1 {
			start += len("unknown value \"")
			end := strings.Index(errorMsg[start:], "\"")
			if end != -1 {
				return errorMsg[start : start+end]
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

	r.logger.Info("🔧 Registering Pipeline controller with ERROR-BASED DIAGNOSTIC")
	r.logger.Info("💡 Diagnostic will ONLY run when reflector errors are detected")

	// Use standard controller registration
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.Pipeline{}).
		Complete(r)
	
	if err != nil {
		r.logger.Error("❌ Pipeline controller registration failed", zap.Error(err))
		return err
	}
	
	// Monitor for reflector errors after successful registration
	go r.monitorForReflectorErrors(mgr)
	
	r.logger.Info("✅ Pipeline controller registered successfully")
	return nil
}

// monitorForReflectorErrors monitors for actual reflector errors and triggers diagnostic only when needed
func (r *Reconciler) monitorForReflectorErrors(mgr ctrl.Manager) {
	// Wait for controller to start
	time.Sleep(2 * time.Second)
	
	r.logger.Info("🔍 Starting reflector error monitoring...")
	
	// Check if Pipeline informer can sync (indicates reflector health)
	cache := mgr.GetCache()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Try to wait for Pipeline informer to sync
	r.logger.Info("⏱️  Waiting for Pipeline informer to sync...")
	
	// This will return false if reflector fails due to enum errors
	if !cache.WaitForCacheSync(ctx) {
		r.logger.Error("🚨 REFLECTOR ERROR DETECTED: Pipeline informer failed to sync!")
		r.logger.Error("🎯 This indicates enum compatibility issues - running diagnostic...")
		r.runDiagnosticOnError(mgr)
	} else {
		r.logger.Info("✅ Pipeline informer synced successfully - no reflector errors")
		r.logger.Info("💡 Diagnostic will not run since no errors were detected")
	}
}

// runDiagnosticOnError runs diagnostic when errors are actually detected
func (r *Reconciler) runDiagnosticOnError(mgr ctrl.Manager) {
	r.logger.Info("🔧 RUNNING DIAGNOSTIC DUE TO DETECTED REFLECTOR ERROR")
	
	// Get dynamic client
	config := mgr.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		r.logger.Error("❌ Failed to create dynamic client for diagnostic", zap.Error(err))
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
		r.logger.Error("🚨 CONFIRMED: Failed to list Pipeline resources!", zap.Error(err))
		
		// Extract and show the unknown enum value
		if unknownValue := extractUnknownEnumValue(err.Error()); unknownValue != "" {
			r.logger.Error("🎯 PROBLEMATIC ENUM VALUE IDENTIFIED!", zap.String("unknown_enum_value", unknownValue))
			r.logger.Info("💡 To find the problematic resource, run:")
			r.logger.Info(fmt.Sprintf("   kubectl get pipelines.v2.michelangelo.api -A -o yaml | grep -C5 '%s'", unknownValue))
		}
		return
	}
	
	// If we get here, list succeeded, so check individual resources
	r.logger.Info("✅ Successfully listed Pipeline resources", zap.Int("count", len(result.Items)))
	for _, item := range result.Items {
		r.validatePipelineResource(&item)
	}
}

// diagnosePipelineResourcesOnError identifies problematic resources when an enum error occurs
func (r *Reconciler) diagnosePipelineResourcesOnError(problemNamespace, problemName string) {
	r.logger.Info("🔍 Starting on-demand diagnostic for enum error...", 
		zap.String("triggered_by", fmt.Sprintf("%s/%s", problemNamespace, problemName)))
	
	r.logger.Info("💡 For immediate identification, run:")
	r.logger.Info(fmt.Sprintf("   kubectl get pipeline %s -n %s -o yaml | grep -E 'PIPELINE_MANIFEST_TYPE_.*'", problemName, problemNamespace))
}

// diagnosePipelineResources identifies which specific Pipeline has problematic enum values (unused now)
func (r *Reconciler) diagnosePipelineResources(mgr ctrl.Manager) {
	// Wait a bit for manager to be ready
	time.Sleep(2 * time.Second)

	r.logger.Info("🔍 Starting diagnostic scan for problematic Pipeline resources...")

	// Get dynamic client to access raw JSON
	config := mgr.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		r.logger.Error("❌ Failed to create dynamic client for diagnostics", zap.Error(err))
		return
	}

	// Define the GVR for Pipeline
	gvr := schema.GroupVersionResource{
		Group:    "michelangelo.api",
		Version:  "v2",
		Resource: "pipelines",
	}

	// List all Pipeline resources across all namespaces
	r.logger.Info("📋 Listing all Pipeline resources to identify problematic ones...")

	result, err := dynamicClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		r.logger.Error("🚨 Failed to list Pipeline resources - this is likely the enum error!", zap.Error(err))
		r.logger.Info("💡 Attempting individual resource inspection...")

		// Try to get individual resources to identify the problematic one
		r.inspectIndividualPipelines(dynamicClient, gvr)
		return
	}

	r.logger.Info("✅ Successfully listed Pipeline resources", zap.Int("count", len(result.Items)))

	// Check each Pipeline individually for enum issues
	for _, item := range result.Items {
		r.validatePipelineResource(&item)
	}
}

// inspectIndividualPipelines tries to identify problematic resources when listing fails
func (r *Reconciler) inspectIndividualPipelines(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource) {
	r.logger.Info("🔍 Attempting to inspect individual Pipeline resources...")
	// This is a fallback - in practice, if listing fails due to enum errors,
	// we'd need to use kubectl or other tools to identify individual resources
	r.logger.Info("💡 Since listing failed, use this command to identify problematic resources:")
	r.logger.Info("   kubectl get pipelines.v2.michelangelo.api -A -o yaml | grep -A5 -B5 'unknown.*enum'")
}

// validatePipelineResource checks a single Pipeline resource for enum issues
func (r *Reconciler) validatePipelineResource(item *unstructured.Unstructured) {
	name := item.GetName()
	namespace := item.GetNamespace()

	// Convert to JSON and try to unmarshal as Pipeline
	jsonBytes, err := json.Marshal(item.Object)
	if err != nil {
		r.logger.Error("❌ Failed to marshal resource to JSON",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return
	}

	// Try to unmarshal into Pipeline struct
	var pipeline v2pb.Pipeline
	if err := json.Unmarshal(jsonBytes, &pipeline); err != nil {
		r.logger.Error("🚨 PROBLEMATIC PIPELINE RESOURCE IDENTIFIED!",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		r.logger.Info("💡 This Pipeline resource contains invalid enum values")
		r.logger.Info("🛠️  To fix, run:",
			zap.String("command", fmt.Sprintf("kubectl edit pipeline %s -n %s", name, namespace)))

		// Parse the error to extract unknown enum value
		if unknownValue := extractUnknownEnumValue(err.Error()); unknownValue != "" {
			r.logger.Error("🎯 Found unknown enum value in resource",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.String("unknown_enum_value", unknownValue))
		}
	} else {
		r.logger.Debug("✅ Pipeline resource is valid",
			zap.String("name", name),
			zap.String("namespace", namespace))
	}
}
