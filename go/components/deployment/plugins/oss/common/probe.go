package common

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Condition reasons shared by the actors that probe model state.
const (
	ReasonBackendUnavailable     = "BackendUnavailable"
	ReasonClientUnavailable      = "ClientUnavailable"
	ReasonHTTPClientUnavailable  = "HTTPClientUnavailable"
	ReasonModelStatusCheckFailed = "ModelStatusCheckFailed"
)

// ProbeFailure is why a model status probe could not be completed, in the form the
// actor's condition reports it: a stable reason token and a human-readable message.
type ProbeFailure struct {
	Reason  string
	Message string
}

// ProbeModelStatus resolves the inference server's backend and the cluster's clients, then
// returns the model's per-replica load status in that cluster.
func ProbeModelStatus(
	ctx context.Context,
	logger *zap.Logger,
	clientFactory clientfactory.ClientFactory,
	backendRegistry *backends.Registry,
	backendType v2pb.BackendType,
	target *v2pb.ClusterTarget,
	namespace string,
	inferenceServerName string,
	modelName string,
) (*backends.ModelStatus, *ProbeFailure) {
	backend, err := backendRegistry.GetBackend(backendType)
	if err != nil {
		return nil, &ProbeFailure{Reason: ReasonBackendUnavailable, Message: err.Error()}
	}
	kubeClient, err := clientFactory.GetClient(ctx, target)
	if err != nil {
		return nil, &ProbeFailure{Reason: ReasonClientUnavailable, Message: err.Error()}
	}
	httpClient, err := clientFactory.GetHTTPClient(ctx, target)
	if err != nil {
		return nil, &ProbeFailure{Reason: ReasonHTTPClientUnavailable, Message: err.Error()}
	}
	status, err := backend.GetModelStatus(ctx, logger, kubeClient, httpClient, APIServerURLFromTarget(target), inferenceServerName, namespace, modelName)
	if err != nil {
		return nil, &ProbeFailure{Reason: ReasonModelStatusCheckFailed, Message: err.Error()}
	}
	return status, nil
}

// IsTerminalFailure reports whether the condition Retrieve handed to Run already carries
// one of the given terminal reasons. Run returns such a condition untouched so the failure
// stays FALSE and the engine ends the rollout, instead of overwriting it with a fresh
// UNKNOWN and waiting forever.
func IsTerminalFailure(condition *apipb.Condition, reasons ...string) bool {
	if condition == nil || condition.Status != apipb.CONDITION_STATUS_FALSE {
		return false
	}
	for _, reason := range reasons {
		if condition.Message == reason {
			return true
		}
	}
	return false
}
