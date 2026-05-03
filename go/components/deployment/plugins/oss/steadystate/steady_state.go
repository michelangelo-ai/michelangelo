package steadystate

import (
	"context"

	"go.uber.org/zap"

	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TODO: this is where we will continuously monitor and update the metadata for the
// deployed model. Since the model registry is currently unimplemented, this is a
// no-op for now.

// SteadyStateActor is the steady-state condition actor for OSS deployments. See the
// package-level TODO above.
type SteadyStateActor struct {
	logger *zap.Logger
}

// GetType returns the condition type identifier for steady state.
func (a *SteadyStateActor) GetType() string {
	return common.ActorTypeSteadyState
}

// Retrieve always reports steady state.
func (a *SteadyStateActor) Retrieve(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	return conditionsutil.GenerateTrueCondition(condition), nil
}

// Run is a no-op.
func (a *SteadyStateActor) Run(ctx context.Context, resource *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	return condition, nil
}
