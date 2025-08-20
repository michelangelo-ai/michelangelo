package pipeline

import (
	"go.uber.org/fx"
	"go.uber.org/zap"
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
	
	// Enable comprehensive schema monitoring for Pipeline resources
	return controllermgr.SetupSchemaMonitoringForResource(mgr, &v2pb.Pipeline{}, pipelineGVR, logger)
}
