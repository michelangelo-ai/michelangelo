package defaultengine

import (
	"context"
	"time"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// TODO: Make this configurable
	defaultInactiveRequeuePeriodInSeconds = 10
	KillReason                            = "killed due to workflow termination"
)

var _ conditionInterfaces.Engine[client.Object] = &DefaultEngine[client.Object]{}

type DefaultEngine[T client.Object] struct {
	logger *zap.Logger
}

var _ conditionInterfaces.Engine[client.Object] = &DefaultEngine[client.Object]{}

func NewDefaultEngine[T client.Object](logger *zap.Logger) *DefaultEngine[T] {
	return &DefaultEngine[T]{
		logger: logger,
	}
}

// Run runs a plugin against a resource.
func (e *DefaultEngine[T]) Run(ctx context.Context, plugin conditionInterfaces.Plugin[T], resource T) (conditionInterfaces.Result, error) {

	defaultResult := conditionInterfaces.Result{
		Result: ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Duration(defaultInactiveRequeuePeriodInSeconds) * time.Second,
		},
		AreSatisfied: false,
		IsTerminal:   false,
	}

	lastCondition, err := e.runPlugin(ctx, plugin, resource)
	if err != nil || lastCondition == nil {
		return defaultResult, err
	}

	switch lastCondition.Status {
	case api.CONDITION_STATUS_TRUE:
		// If the condition is true, we are satisfied and the condition is terminal.
		return conditionInterfaces.Result{
			Result: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			AreSatisfied: true,
			IsTerminal:   true,
		}, nil
	case api.CONDITION_STATUS_FALSE:
		if lastCondition.Reason == KillReason {
			return conditionInterfaces.Result{
				Result: ctrl.Result{
					Requeue:      false,
					RequeueAfter: 0,
				},
				AreSatisfied: false,
				IsTerminal:   true,
				IsKilled:     true,
			}, nil
		} else {
			return conditionInterfaces.Result{
				Result: ctrl.Result{
					Requeue:      false,
					RequeueAfter: 0,
				},
				AreSatisfied: false,
				IsTerminal:   true,
			}, nil
		}
	}
	return defaultResult, nil
}

func (e *DefaultEngine[T]) runPlugin(ctx context.Context, plugin conditionInterfaces.Plugin[T], resource T) (*api.Condition, error) {
	actors := plugin.GetActors()

	var lastCondition *api.Condition
	actionRun := false

	// Phase 1: Retrieve status from all actors
	for _, actor := range actors {
		previousCondition := conditionUtils.GetCondition(actor.GetType(), plugin.GetConditions(resource))
		retrievedCondition, err := actor.Retrieve(ctx, resource, previousCondition)
		if err != nil {
			e.logger.Error("error retrieving actor condition", zap.Error(err))
			return nil, err
		}

		lastCondition = retrievedCondition
		plugin.PutCondition(resource, retrievedCondition)

		// Phase 2: Run only the first non-satisfied condition (matching Uber's pattern)
		if retrievedCondition.Status != api.CONDITION_STATUS_TRUE && !actionRun {
			e.logger.Info("running action for first non-satisfied condition",
				zap.String("actor", actor.GetType()),
				zap.String("status", retrievedCondition.Status.String()))

			runCondition, err := actor.Run(ctx, resource, retrievedCondition)
			if err != nil {
				e.logger.Error("error running actor", zap.Error(err))
				return nil, err
			}

			lastCondition = runCondition
			plugin.PutCondition(resource, runCondition)
			actionRun = true
		}
	}

	return lastCondition, nil
}
