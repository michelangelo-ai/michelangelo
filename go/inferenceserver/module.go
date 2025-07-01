package inferenceserver

import (
	"context"

	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/oss"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
)

// Module provides the inference server controller with all dependencies
var Module = fx.Module("inferenceserver",
	fx.Provide(NewInferenceServerGateway),
	fx.Provide(NewOSSProvider),
	fx.Provide(NewReconciler),
	fx.Invoke(register),
)

// NewInferenceServerGateway creates a new inference server gateway
func NewInferenceServerGateway() inferenceserver.Gateway {
	return inferenceserver.NewGateway()
}

// NewOSSProvider creates a new OSS provider with gateway
func NewOSSProvider(client client.Client, gateway inferenceserver.Gateway) Provider {
	return &ossProviderAdapter{provider: oss.NewProvider(client, gateway)}
}

// ossProviderAdapter adapts the OSS provider to the interface
type ossProviderAdapter struct {
	provider *oss.Provider
}

func (a *ossProviderAdapter) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	ossReq := &oss.CreateRequest{
		InferenceServer: req.InferenceServer,
		Logger:          req.Logger,
	}
	ossResp, err := a.provider.Create(ctx, ossReq)
	if err != nil {
		return nil, err
	}
	return &CreateResponse{
		State:   ossResp.State,
		Message: ossResp.Message,
		Details: ossResp.Details,
	}, nil
}

func (a *ossProviderAdapter) Get(ctx context.Context, req *GetRequest) (*GetResponse, error) {
	ossReq := &oss.GetRequest{
		InferenceServer: req.InferenceServer,
		Logger:          req.Logger,
	}
	ossResp, err := a.provider.Get(ctx, ossReq)
	if err != nil {
		return nil, err
	}
	return &GetResponse{
		State:   ossResp.State,
		Message: ossResp.Message,
		Details: ossResp.Details,
	}, nil
}

func (a *ossProviderAdapter) Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error) {
	ossReq := &oss.DeleteRequest{
		InferenceServer: req.InferenceServer,
		Logger:          req.Logger,
	}
	ossResp, err := a.provider.Delete(ctx, ossReq)
	if err != nil {
		return nil, err
	}
	return &DeleteResponse{
		State:   ossResp.State,
		Message: ossResp.Message,
		Details: ossResp.Details,
	}, nil
}

// NewReconciler creates a new inference server reconciler
func NewReconciler(mgr ctrl.Manager, scheme *runtime.Scheme, provider Provider) *Reconciler {
	return &Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   scheme,
		Recorder: mgr.GetEventRecorderFor(ControllerName),
		Provider: provider,
	}
}

// register sets up the inference server controller with the manager
func register(mgr ctrl.Manager, reconciler *Reconciler) error {
	return reconciler.SetupWithManager(mgr)
}