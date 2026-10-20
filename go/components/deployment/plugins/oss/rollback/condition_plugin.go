package rollback

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.Plugin[*v2pb.Deployment] = &conditionPlugin{}

// conditionPlugin orchestrates rollback actors to restore the previous stable revision, one
// cluster at a time, in the same cluster order the rollout used.
type conditionPlugin struct {
	actors []conditionInterfaces.ConditionActor[*v2pb.Deployment]
}

// Params contains dependencies injected for rollback plugin initialization.
type Params struct {
	ClientFactory clientfactory.ClientFactory
	// APIHandler reads the previous revision's Model when its config entry has to be recreated.
	APIHandler          goapi.Handler
	BackendRegistry     *backends.Registry
	ModelConfigProvider modelconfig.ModelConfigProvider
	RouteManager        routing.Manager
	// DynamicClient is the control-plane dynamic client, used for the discovery route.
	DynamicClient dynamic.Interface
	Logger        *zap.Logger
	Settings      osscommon.RolloutSettings
	// Now is the clock used for the rollback timeout. Nil means time.Now.
	Now func() time.Time
}

// NewRollbackPlugin creates a rollback workflow plugin for the deployment. The per-cluster
// actors are derived from the cluster snapshot the rollout recorded on the deployment, so a
// rollback undoes exactly the clusters the rollout touched.
func NewRollbackPlugin(ctx context.Context, p Params, deployment *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	targets, err := osscommon.ReadTargetClustersAnnotation(deployment)
	if err != nil {
		return nil, fmt.Errorf("read target clusters annotation: %w", err)
	}

	actors := make([]conditionInterfaces.ConditionActor[*v2pb.Deployment], 0, len(targets)+1)
	for _, target := range targets {
		actors = append(actors, NewClusterRollbackActor(p, target))
	}
	actors = append(actors, NewRollbackCompletionActor(p))

	return &conditionPlugin{actors: actors}, nil
}

// GetActors returns the rollback actors.
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
