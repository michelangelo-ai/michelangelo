package ray

import (
	"context"
	"fmt"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var Module = fx.Options(
	fx.Provide(scheme),
	fx.Provide(register),
	fx.Invoke(start),
)

func scheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := kubescheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v2pb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func register(workers []worker.Worker, scheme *runtime.Scheme) manager.Manager {

	restConf, err := ctrl.GetConfig()
	if err != nil {
		return nil
	}

	mgr, err := ctrl.NewManager(restConf, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: "localhost:8081",
		LeaderElection:         false,
		LeaderElectionID:       "decaf1259.michelangelo.uber.com",
	})

	if err != nil {
		return nil
	}
	a := &activities{
		k8sClient: mgr.GetClient(),
	}
	for _, w := range workers {
		w.RegisterActivity(a)
	}
	return mgr
}

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
//
//	mgr (manager.Manager): Kubernetes controller manager to be started.
func _start(mgr manager.Manager) {
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		// TODO: handle error properly. Exit app? Propagate to the parent thread?
		fmt.Printf("ERR: Controller Manager execution failed: %v", err)
		os.Exit(1)
	}
}
