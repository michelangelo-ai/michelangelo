// Package apihook registers the Model API hook used to default and inherit
// the environment label on Models at create/update time. See
// go/components/pipelinerun/apihook and go/components/triggerrun/apihook for
// the sibling packages this one follows the shape of.
//
// Phase 0 (this file, initial commit): wiring-only skeleton. BeforeCreate and
// BeforeUpdate pass through to the embedded NoopModelAPIHook with zero
// behavior change. Label-defaulting/inheritance logic is added in a
// follow-up commit (see specs/001-environment-label/phase1_plan.md in the
// migration-planning harness for that commit's design).
package apihook

import (
	"context"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// RegisterModelAPIHook registers the API hook responsible for Model
// create/update-time behavior (currently a no-op skeleton; see Phase 1 for
// the environment-label defaulting/inheritance logic added on top of this).
func RegisterModelAPIHook(logger *zap.Logger, apiHandler api.Handler) {
	v2.RegisterModelAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
	})
}

type apiHook struct {
	v2.NoopModelAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
}

// BeforeCreate currently defers entirely to the embedded NoopModelAPIHook.
// This method is overridden with real logic in a follow-up commit (Phase 1).
func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateModelRequest) error {
	return a.NoopModelAPIHook.BeforeCreate(ctx, request)
}

// BeforeUpdate currently defers entirely to the embedded NoopModelAPIHook.
// This method is overridden with real logic in a follow-up commit (Phase 1).
func (a apiHook) BeforeUpdate(ctx context.Context, request *v2.UpdateModelRequest) error {
	return a.NoopModelAPIHook.BeforeUpdate(ctx, request)
}
