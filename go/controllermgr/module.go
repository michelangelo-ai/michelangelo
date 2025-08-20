package controllermgr

import (
	"context"
	"fmt"
	"os"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore/minio"
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
		blobstore.Module,
		minio.Module,
		fx.Provide(newConfig),
		fx.Provide(create),
		fx.Invoke(start),
	)
)

type (
	params struct {
		fx.In
		Config Config          // Configuration parameters for the controller manager.
		Scheme *runtime.Scheme // Kubernetes runtime scheme used by the manager.
	}

	result struct {
		fx.Out
		Manager manager.Manager // Initialized Kubernetes controller manager.
		Client  client.Client   // Kubernetes client for interacting with the cluster.
	}
)

// create initializes and configures a new Kubernetes controller manager based on the provided parameters.
// It retrieves the Kubernetes REST configuration, creates a manager instance, and configures it with the specified options.
//
// Params:
//
//	p (params): Struct containing Config and Scheme.
//
// Returns:
//
//	result: Struct containing the initialized Manager and Client.
//	error: Error if the manager creation fails.
func create(p params) (result, error) {

	restConf, err := ctrl.GetConfig()
	if err != nil {
		return result{}, err
	}

	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme: p.Scheme,
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
//
//	lc (fx.Lifecycle): Lifecycle hook to manage application startup and shutdown.
//	mgr (manager.Manager): Initialized Kubernetes controller manager.
//
// Returns:
//
//	error: Error if lifecycle setup fails.
func start(lc fx.Lifecycle, mgr manager.Manager) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go _start(mgr)
			return nil
		},
	})
	return nil
}


// _start starts the Kubernetes controller manager with enhanced schema validation logging.
func _start(mgr manager.Manager) {
	fmt.Printf("Starting controller manager with enhanced schema validation...\n")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("Controller Manager execution failed: %v\n", err)

		// Enhanced logging for schema compatibility errors
		if schemaErrorType := IsSchemaCompatibilityError(err); schemaErrorType != "" {
			fmt.Printf("SCHEMA COMPATIBILITY ERROR DETECTED!\n")
			fmt.Printf("Error Type: %s\n", schemaErrorType)
			fmt.Printf("Error Details: %v\n", err)
			fmt.Printf("This indicates resources with schema compatibility issues exist in the cluster\n")
			fmt.Printf("To identify the problematic resource, run:\n")

			if problemValue := ExtractSchemaErrorValue(err.Error(), schemaErrorType); problemValue != "" {
				fmt.Printf("   kubectl get pipelines.v2.michelangelo.api -A -o yaml | grep -C5 '%s'\n", problemValue)
			} else {
				fmt.Printf("   kubectl get pipelines.v2.michelangelo.api -A -o yaml\n")
			}

			// Provide guidance based on error type
			ProvideStartupSchemaGuidance(schemaErrorType)
		} else {
			fmt.Printf("Non-schema error detected\n")
		}

		os.Exit(1)
	}
}