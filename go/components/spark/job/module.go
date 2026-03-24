package job

import (
	"go.uber.org/fx"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
)

var (
	// Module provides Uber FX dependency injection options for the SparkJob controller.
	//
	// This module registers the SparkJob controller with the Kubernetes controller manager.
	// The controller manages Spark jobs that execute on Kubernetes via the Spark Operator,
	// handling job submission, status monitoring, and condition updates.
	//
	// Dependencies:
	//   - SparkClient: For creating and monitoring Spark applications
	//   - Manager: Kubernetes controller manager for resource watching
	//   - Environment: Context for configuration
	Module = fx.Options(
		fx.Invoke(register),
	)
)

// register initializes and registers the SparkJob controller with the manager.
//
// This function is invoked by Uber FX during application startup. It constructs
// the Reconciler with all required dependencies and registers it with the
// Kubernetes controller manager.
//
// The controller will begin watching SparkJob resources and processing them
// through their lifecycle once the manager starts.
func register(
	env env.Context,
	mgr manager.Manager,
	sparkClient Client,
) error {
	return NewReconciler(mgr.GetClient(), sparkClient, env).Register(mgr)
}
