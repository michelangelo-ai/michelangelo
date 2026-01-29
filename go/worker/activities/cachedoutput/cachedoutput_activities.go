package cachedoutput

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Activities = (*activities)(nil)

// TerminateClusterRequest defines the request parameters for terminating a Spark cluster.
type TerminateClusterRequest struct {
	Name      string `json:"name,omitempty"`      // name of the spark job
	Namespace string `json:"namespace,omitempty"` // namespace of the spark job
	Type      string `json:"type,omitempty"`      // termination code
	Reason    string `json:"reason,omitempty"`    // termination reason
}

// TerminateSparkJobRequest defines the request parameters for terminating a Spark job.
type TerminateSparkJobRequest struct {
	Name      string               `json:"name,omitempty"`      // name of the spark job
	Namespace string               `json:"namespace,omitempty"` // namespace of the spark job
	Type      v2pb.TerminationType `json:"type,omitempty"`      // termination code
	Reason    string
}

// activities struct encapsulates the YARPC clients for Spark cluster and job services.
type activities struct {
	cachedOutput v2pb.CachedOutputServiceYARPCClient
}

func (r *activities) GetCachedOutput(ctx context.Context, request v2pb.GetCachedOutputRequest) (*v2pb.GetCachedOutputResponse, error) {
	return r.cachedOutput.GetCachedOutput(ctx, &request)
}

func (r *activities) ListCachedOutput(ctx context.Context, request v2pb.ListCachedOutputRequest) (*v2pb.ListCachedOutputResponse, error) {
	return r.cachedOutput.ListCachedOutput(ctx, &request)
}

func (r *activities) CreateCachedOutput(ctx context.Context, request v2pb.CreateCachedOutputRequest) (*v2pb.CreateCachedOutputResponse, error) {
	return r.cachedOutput.CreateCachedOutput(ctx, &request)
}
