package ingester

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/storage"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Module provides the ingester reconcilers
var Module = fx.Options(
	fx.Invoke(register),
)

type registerParams struct {
	fx.In
	Manager         ctrl.Manager
	Scheme          *runtime.Scheme
	MetadataStorage storage.MetadataStorage `optional:"true"`
	Config          Config                  `optional:"true"`
	Logger          *zap.Logger
}

// register sets up ingester reconcilers for all configured CRD types
func register(p registerParams) error {
	// Only set up ingester if metadata storage is configured
	if p.MetadataStorage == nil {
		p.Logger.Info("Metadata storage not configured, skipping ingester controller setup")
		return nil
	}

	p.Logger.Info("Setting up ingester controllers")

	// List of CRD objects to watch
	crdObjects := []runtime.Object{}

	for _, obj := range crdObjects {
		gvks, _, err := p.Scheme.ObjectKinds(obj)
		if err != nil || len(gvks) == 0 {
			return fmt.Errorf("failed to get GVK for %T: %w", obj, err)
		}
		gvk := gvks[0]
		log := p.Logger.With(zap.String("kind", gvk.Kind))

		// Cast runtime.Object to client.Object
		clientObj, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("object %s does not implement client.Object", gvk.Kind)
		}

		// Get controller-specific config (supports per-CRD configuration)
		controllerConfig := p.Config.GetControllerConfig(gvk.Kind)

		reconciler := NewReconciler(
			p.Manager.GetClient(),
			ctrl.Log.WithName("ingester").WithName(gvk.Kind),
			p.Scheme,
			clientObj,
			p.MetadataStorage,
			WithConfig(controllerConfig),
		)

		if err := reconciler.SetupWithManager(p.Manager); err != nil {
			return fmt.Errorf("failed to setup ingester for %s: %w", gvk.Kind, err)
		}

		log.Info("Ingester controller registered successfully",
			zap.Int("concurrentReconciles", controllerConfig.ConcurrentReconciles),
			zap.Duration("requeuePeriod", controllerConfig.RequeuePeriod))
	}

	return nil
}
