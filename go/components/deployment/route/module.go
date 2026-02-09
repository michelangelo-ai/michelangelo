package route

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

var Module = fx.Options(
	fx.Provide(newRouteProvider),
	fx.Provide(newDynamicClient),
)

// newRouteProvider creates a new route provider
func newRouteProvider(dynamicClient dynamic.Interface, logger *zap.Logger) RouteProvider {
	return NewHTTPRouteManager(dynamicClient, logger)
}

// newDynamicClient creates a Kubernetes dynamic client for working with unstructured resources
// todo: ghosharitra: move this somewhere else
func newDynamicClient() (dynamic.Interface, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to get rest config: %w", err))
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(fmt.Errorf("failed to create dynamic client: %w", err))
	}
	return dynamicClient, nil
}
