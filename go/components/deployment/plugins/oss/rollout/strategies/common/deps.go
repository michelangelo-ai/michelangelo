package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/components/common/routing"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	modelconfig "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ClusterActorDeps bundles the collaborators every per-cluster rollout actor uses.
type ClusterActorDeps struct {
	ClientFactory clientfactory.ClientFactory
	// APIHandler reads the Model, which lives in the control plane rather than in the
	// target cluster, and may be served from metadata storage.
	APIHandler          goapi.Handler
	BackendRegistry     *backends.Registry
	ModelConfigProvider modelconfig.ModelConfigProvider
	RouteManager        routing.Manager
	Logger              *zap.Logger
	Settings            osscommon.RolloutSettings
	// Now is the clock used for timeouts and soaks. Nil means time.Now.
	Now func() time.Time
}

func (d ClusterActorDeps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// modelStatus returns the model's per-replica load status in the target cluster.
func (d ClusterActorDeps) modelStatus(ctx context.Context, target *v2pb.ClusterTarget, deployment *v2pb.Deployment, modelName string) (*backends.ModelStatus, *osscommon.ProbeFailure) {
	return osscommon.ProbeModelStatus(ctx, d.Logger, d.ClientFactory, d.BackendRegistry, osscommon.BackendTypeOf(deployment), target,
		deployment.Namespace, deployment.Spec.GetInferenceServer().GetName(), modelName)
}

// describeFailedReplicas renders the failed replicas of a status for a condition message.
func describeFailedReplicas(failed []backends.ReplicaModelStatus) string {
	parts := make([]string, 0, len(failed))
	for _, replica := range failed {
		parts = append(parts, fmt.Sprintf("%s: %s", replica.Replica, replica.Reason))
	}
	return strings.Join(parts, "; ")
}
