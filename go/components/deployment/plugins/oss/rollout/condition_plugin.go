package rollout

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/rollout/strategies"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

// conditionPlugin orchestrates rollout actors in sequence: validation, preparation, placement, routing, completion.
type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies injected for rollout plugin initialization.
type Params struct {
	Client          client.Client
	HTTPClient      *http.Client
	DynamicClient   dynamic.Interface
	ClientFactory   clientfactory.ClientFactory
	RouteManager    routing.Manager
	BackendRegistry *backends.Registry
	Logger          *zap.Logger
}

// NewRolloutPlugin creates a rollout workflow plugin with deployment-specific strategy actors.
// It looks up the InferenceServer referenced by the Deployment to resolve the backend type,
// so actor chains are built correctly for Triton vs KServe.
func NewRolloutPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	logger := p.Logger.With(zap.String("deployment", fmt.Sprintf("%s/%s", deployment.GetNamespace(), deployment.GetName())))

	// Resolve the backend type from the referenced InferenceServer.
	backendType, err := resolveBackendType(ctx, p.Client, deployment)
	if err != nil {
		logger.Warn("failed to resolve backend type, defaulting to Triton", zap.Error(err))
		backendType = v2pb.BACKEND_TYPE_TRITON
	}

	prePlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&ValidationActor{logger: logger},
		&AssetPreparationActor{logger: logger},
		&PlacementPrepActor{kubeClient: p.Client, logger: logger},
	}

	placementActors, err := strategies.GetActorsForStrategy(ctx, strategies.Params{
		ClientFactory:   p.ClientFactory,
		Client:          p.Client,
		HTTPClient:      p.HTTPClient,
		DynamicClient:   p.DynamicClient,
		RouteManager:    p.RouteManager,
		BackendRegistry: p.BackendRegistry,
		BackendType:     backendType,
		Logger:          p.Logger,
	}, deployment)
	if err != nil {
		return nil, err
	}

	postPlacementActors := []conditionInterfaces.ConditionActor[*v2pb.Deployment]{
		&RolloutCompletionActor{
			backendRegistry: p.BackendRegistry,
			logger:          p.Logger,
		},
	}

	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0,
		len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)

	return &conditionPlugin{actors: actors}, nil
}

// resolveBackendType looks up the InferenceServer referenced by the Deployment
// and returns its configured backend type.
func resolveBackendType(ctx context.Context, kubeClient client.Client, deployment *v2pb.Deployment) (v2pb.BackendType, error) {
	isRef := deployment.Spec.GetInferenceServer()
	if isRef == nil || isRef.GetName() == "" {
		return v2pb.BACKEND_TYPE_TRITON, nil
	}

	ns := isRef.GetNamespace()
	if ns == "" {
		ns = deployment.GetNamespace()
	}

	var is v2pb.InferenceServer
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: isRef.GetName(), Namespace: ns}, &is); err != nil {
		return v2pb.BACKEND_TYPE_TRITON, fmt.Errorf("get InferenceServer %s/%s: %w", ns, isRef.GetName(), err)
	}

	return is.Spec.BackendType, nil
}

// GetActors returns the ordered sequence of rollout actors.
func (p *conditionPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.Deployment] {
	return p.actors
}

// GetConditions retrieves the current conditions from the deployment status.
func (p *conditionPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

// PutCondition updates or adds a condition to the deployment status.
func (p *conditionPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}
