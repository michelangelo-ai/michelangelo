package main

import (
	v2pb "v2"

	"github.com/go-logr/logr"

	"github.com/michelangelo-ai/michelangelo/go/controllermgr"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

// scheme FX provider for kubernetes runtime.Scheme object. Defines kubernetes API types used by the app.
//
// Usually, the returned Scheme struct includes kubernetes standard scheme defined by the
// k8s.io/client-go/kubernetes/scheme, as well as a number of custom schemes containing CRDs,
// e.g. api/v2
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

// options FX modules used by the app
func options() fx.Option {
	return fx.Options(
		fx.Provide(scheme),
		controllermgr.Module,
		fx.Invoke(func(logger logr.Logger) {
			ctrl.SetLogger(logger)
		}),
	)
}

func main() {

	fx.New(options()).Run()
}
