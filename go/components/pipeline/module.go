package pipeline

import (
	"encoding/json"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/controllermgr"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var (
	// Module FX
	Module = fx.Options(
		fx.Invoke(register),
	)
)

func register(
	mgr manager.Manager,
	env env.Context,
	apiHandlerFactory apiHandler.Factory,
	logger *zap.Logger,
) error {
	// Register the controller
	err := (&Reconciler{
		env:               env,
		apiHandlerFactory: apiHandlerFactory,
		logger:            logger,
	}).Register(mgr)
	if err != nil {
		return err
	}
	
	// Set up schema monitoring for Pipeline resources
	pipelineGVR := schema.GroupVersionResource{
		Group:    "michelangelo.api",
		Version:  "v2",
		Resource: "pipelines",
	}
	
	// Create Pipeline-specific validator function
	validator := func(item *unstructured.Unstructured, logger *zap.Logger) (bool, error) {
		name := item.GetName()
		namespace := item.GetNamespace()

		logger.Info("Validating Pipeline resource with protobuf unmarshaling",
			zap.String("name", name),
			zap.String("namespace", namespace))

		// Convert unstructured to JSON first
		jsonBytes, err := json.Marshal(item.Object)
		if err != nil {
			logger.Error("Failed to marshal Pipeline resource to JSON",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.Error(err))
			return false, err
		}

		// Try to unmarshal into v2pb.Pipeline - this will catch enum validation errors
		var pipeline v2pb.Pipeline
		if err := json.Unmarshal(jsonBytes, &pipeline); err != nil {
			// This is where enum errors will be caught
			logger.Error("PROBLEMATIC RESOURCE IDENTIFIED!",
				zap.String("name", name),
				zap.String("namespace", namespace),
				zap.Error(err))
			return false, err
		}

		logger.Debug("Pipeline resource is valid",
			zap.String("name", name),
			zap.String("namespace", namespace))
		return true, nil
	}

	// Enable comprehensive schema monitoring for Pipeline resources
	return controllermgr.SetupSchemaMonitoringForResource(mgr, &v2pb.Pipeline{}, pipelineGVR, validator, logger)
}
