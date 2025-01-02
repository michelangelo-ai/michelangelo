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
	// Module Provides and starts kubernetes controller manager as configured by the Config
	Module = fx.Options(
		fx.Provide(newConfig),
		fx.Provide(create),
		fx.Invoke(start),
	)
)

type (
	params struct {
		fx.In
		Config Config
		Scheme *runtime.Scheme
	}

	result struct {
		fx.Out
		Manager manager.Manager
		Client  client.Client
	}
)

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

func start(lc fx.Lifecycle, mgr manager.Manager) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go _start(mgr)
			return nil
		},
	})
	return nil
}

func _start(mgr manager.Manager) {
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		// TODO: handle error properly. Exit app? Propagate to the parent thread?
		fmt.Printf("ERR: Controller Manager execution failed: %v", err)
		os.Exit(1)
	}
}
