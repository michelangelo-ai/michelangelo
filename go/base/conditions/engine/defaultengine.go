package defaultengine

import (
	"context"
	"time"

	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionUtils "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
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

	criticalCondition, criticalError := e.runPlugin(ctx, plugin, resource)
	if criticalError != nil {
		// if there is an error, we are in a terminal state and we don't need to requeue
		return conditionInterfaces.Result{
			Result: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			AreSatisfied: false,
			IsTerminal:   true,
		}, nil
	}

	if criticalCondition == nil {
		// if the critical condition is nil, all conditions are satisfied and the condition is terminal.
		return conditionInterfaces.Result{
			Result: ctrl.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			AreSatisfied: true,
			IsTerminal:   true,
		}, nil
	}

	switch criticalCondition.Status {
	case api.CONDITION_STATUS_FALSE:
		if criticalCondition.Reason == KillReason {
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

// runPlugin executes the plugin's actors against the given resource and returns:
//  1. The "critical condition", specifically the condition yielded by the actor's Run() method
//     for the first non-satisfied actor encountered. If the method returns nil, then Run() was never
//     called and all actions succeeded. If the returned condition's status is false, then the condition
//     is terminal and no further requeue is needed.
//  2. An error, if any occurred during processing. The error is returned to indicate that the plugin has failed and the condition is terminal.
func (e *DefaultEngine[T]) runPlugin(ctx context.Context, plugin conditionInterfaces.Plugin[T], resource T) (*api.Condition, error) {
	actors := plugin.GetActors()

	var lastCondition *api.Condition
	var actionHasRun bool
	var actorType string

	var criticalCondition *api.Condition
	var criticalError error

	// Retrieve all conditions
	for _, actor := range actors {
		actorType = actor.GetType()
		previousCondition := conditionUtils.GetCondition(actorType, plugin.GetConditions(resource))
		if previousCondition == nil {
			previousCondition = &api.Condition{
				Type:   actorType,
				Status: api.CONDITION_STATUS_UNKNOWN,
			}
		}
		retrievedCondition, err := actor.Retrieve(ctx, resource, previousCondition)
		if err != nil {
			e.logger.Error("error retrieving actor condition", zap.Error(err))
			criticalError = err
			return nil, criticalError
		}

		lastCondition = retrievedCondition

		// Run only the first non-satisfied condition
		if retrievedCondition.Status != api.CONDITION_STATUS_TRUE && !actionHasRun {

			e.logger.Info("running action for first non-satisfied condition",
				zap.String("actor", actorType),
				zap.String("status", retrievedCondition.Status.String()))

			runCondition, err := actor.Run(ctx, resource, retrievedCondition)
			if err != nil {
				e.logger.Error("error running actor", zap.Error(err))
				criticalError = err
				return nil, criticalError
			}

			lastCondition = runCondition
			criticalCondition = lastCondition
			actionHasRun = true
		}
		plugin.PutCondition(resource, lastCondition)
	}

	return criticalCondition, criticalError
}
