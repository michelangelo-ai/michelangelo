package cachedoutput

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

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
