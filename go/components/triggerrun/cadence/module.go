package cadence

import (
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
)

// Module provides fx Options for cadence trigger activities.
var Module = fx.Options(
	fx.Provide(NewService),
)

// ServiceParams are the parameters for creating a cadence Service.
type ServiceParams struct {
	fx.In
	PipelineRunService v2pb.PipelineRunServiceYARPCClient
}

// NewService creates a new cadence Service for trigger activities.
func NewService(p ServiceParams) *Service {
	return &Service{
		PipelineRunService: p.PipelineRunService,
	}
}
