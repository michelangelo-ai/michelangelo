package model

import (
	"context"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var Activities = (*activities)(nil)

// activities struct encapsulates the YARPC clients for Spark cluster and job services.
type activities struct {
	model v2pb.ModelServiceYARPCClient
}

func (r *activities) GetModel(ctx context.Context, request v2pb.GetModelRequest) (*v2pb.GetModelResponse, error) {
	return r.model.GetModel(ctx, &request)
}
