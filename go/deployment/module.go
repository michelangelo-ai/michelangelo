package deployment

import (
	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	"k8s.io/client-go/dynamic"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins/oss"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways"
)

// Module provides the deployment controller with all dependencies
var Module = fx.Module("deployment",
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewReconciler creates a new deployment reconciler with injected dependencies
func NewReconciler(client ctrl.Manager, logger *zap.Logger, gateway gateways.Gateway, blobstore *blobstore.BlobStore) *Reconciler {
	log := zapr.NewLogger(logger).WithName("deployment")

	// Create dynamic client from manager's REST config
	restConfig := client.GetConfig()
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		log.Error(err, "Failed to create dynamic client")
		// Continue without dynamic client for backward compatibility
		dynamicClient = nil
	}

	plugin := oss.NewPluginWithDynamicClient(client.GetClient(), gateway, blobstore, dynamicClient, log)

	return &Reconciler{
		Client: client.GetClient(),
		Log:    log,
		Plugin: plugin,
	}
}

// register sets up the deployment controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}
