package controllermgr

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	// Module provides and starts the Kubernetes Controller Manager as configured by the Config.
	// It uses Fx for dependency injection to initialize configurations, create the manager,
	// and set up the lifecycle hooks for the application.
	Module = fx.Options(
		fx.Provide(newConfig),
		fx.Provide(create),
		fx.Invoke(start),
	)
)

type (
	params struct {
		fx.In
		Config Config			// Configuration parameters for the controller manager.
		Scheme *runtime.Scheme	// Kubernetes runtime scheme used by the manager.
	}

	result struct {
		fx.Out
		Manager manager.Manager	// Initialized Kubernetes controller manager.
		Client  client.Client	// Kubernetes client for interacting with the cluster.
	}
)

// create initializes and configures a new Kubernetes controller manager based on the provided parameters.
// It retrieves the Kubernetes REST configuration, creates a manager instance, and configures it with the specified options.
//
// Params:
//   p (params): Struct containing Config and Scheme.
//
// Returns:
//   result: Struct containing the initialized Manager and Client.
//   error: Error if the manager creation fails.
func create(p params) (result, error) {

	restConf, err := ctrl.GetConfig()
	if err != nil {
		return result{}, err
	}

	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme:                 p.Scheme,
		//MetricsBindAddress:     p.Config.MetricsBindAddress,
		//Port:                   p.Config.Port,
		HealthProbeBindAddress: p.Config.HealthProbeBindAddress,
		LeaderElection:         p.Config.LeaderElection,
		LeaderElectionID:       p.Config.LeaderElectionID,
	})
	if err != nil {
		return result{}, err
	}

	return result{
		Manager: mgr,
		Client:  mgr.GetClient(),
	}, nil
}

// start sets up a lifecycle hook to start the Kubernetes controller manager.
// The manager is started in a separate goroutine and listens for termination signals.
//
// Params:
//   lc (fx.Lifecycle): Lifecycle hook to manage application startup and shutdown.
//   mgr (manager.Manager): Initialized Kubernetes controller manager.
//
// Returns:
//   error: Error if lifecycle setup fails.
func start(lc fx.Lifecycle, mgr manager.Manager) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go _start(mgr)
			return nil
		},
	})
	return nil
}

// _start starts the Kubernetes controller manager and handles runtime errors.
// If the manager fails to start, it logs the error and exits the application.
//
// Params:
//   mgr (manager.Manager): Kubernetes controller manager to be started.
func _start(mgr manager.Manager) {
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		// TODO: handle error properly. Exit app? Propagate to the parent thread?
		fmt.Printf("ERR: Controller Manager execution failed: %v", err)
		os.Exit(1)
	}
}
