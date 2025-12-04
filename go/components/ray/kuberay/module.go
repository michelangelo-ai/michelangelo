package kuberay

import (
	"go.uber.org/fx"
)

// Module provides Uber FX dependency injection options for KubeRay client support.
//
// This module registers the NewRestClient provider, making a configured REST client
// for KubeRay resources available to other components through dependency injection.
//
// The module is named "kuberay" for organizational purposes within the FX application
// graph, allowing clear identification of KubeRay-related dependencies.
//
// Provided:
//   - rest.Interface: Configured client for ray.io/v1 API operations
var Module = fx.Module("kuberay",
	fx.Provide(NewRestClient),
)
