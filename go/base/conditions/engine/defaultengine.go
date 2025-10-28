package defaultengine

import (
	"context"
	"fmt"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
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
	maxRetryAttempts                      = 3
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

	for _, actor := range actors {
		previousCondition := conditionUtils.GetCondition(actor.GetType(), plugin.GetConditions(resource))
		newCondition, err := e.runActorWithRetry(ctx, actor, resource, previousCondition)
		if err != nil {
			e.logger.Error("error running actor with retry", zap.Error(err))
			return nil, err
		}
		lastCondition = newCondition
		plugin.PutCondition(resource, newCondition)
	}

	return lastCondition, nil
}

func (e *DefaultEngine[T]) runActorWithRetry(ctx context.Context, actor conditionInterfaces.ConditionActor[T], resource T, previousCondition *api.Condition) (*api.Condition, error) {
	attempts := e.getRetryAttempts(previousCondition)

	for attempt := attempts; attempt < maxRetryAttempts; attempt++ {
		e.logger.Info("running actor", zap.String("actor", actor.GetType()), zap.Int32("attempt", attempt+1), zap.Int32("maxAttempts", maxRetryAttempts))

		newCondition, err := actor.Run(ctx, resource, previousCondition)
		if err != nil {
			e.logger.Error("actor execution failed",
				zap.String("actor", actor.GetType()),
				zap.Int32("attempt", attempt+1),
				zap.Error(err))

			// If this is the last attempt, return the error
			if attempt == maxRetryAttempts-1 {
				return nil, err
			}

			// Create a condition to track the retry attempt
			retryCondition := e.createRetryCondition(actor.GetType(), attempt+1, err.Error())
			previousCondition = retryCondition
			continue
		}

		// Success - check if we need to clear retry metadata
		if newCondition.Status == api.CONDITION_STATUS_TRUE || newCondition.Status == api.CONDITION_STATUS_FALSE {
			// Terminal condition - clear retry metadata if it exists
			newCondition.Metadata = nil
			return newCondition, nil
		}

		// Still running (UNKNOWN status) - preserve retry metadata if needed
		if attempt > 0 {
			newCondition = e.addRetryMetadata(newCondition, attempt+1)
		}
		return newCondition, nil
	}

	// This should never be reached, but just in case
	return nil, fmt.Errorf("max retry attempts exceeded for actor %s", actor.GetType())
}

func (e *DefaultEngine[T]) getRetryAttempts(condition *api.Condition) int32 {
	if condition == nil || condition.Metadata == nil {
		return 0
	}

	var retryMeta pbtypes.Struct
	if err := pbtypes.UnmarshalAny(condition.Metadata, &retryMeta); err != nil {
		e.logger.Warn("failed to unmarshal retry metadata", zap.Error(err))
		return 0
	}

	if attemptsVal, ok := retryMeta.Fields["attempts"]; ok {
		if numberVal := attemptsVal.GetNumberValue(); numberVal != 0 {
			return int32(numberVal)
		}
	}

	return 0
}

func (e *DefaultEngine[T]) createRetryCondition(actorType string, attempts int32, errorMsg string) *api.Condition {
	retryStruct := &pbtypes.Struct{
		Fields: map[string]*pbtypes.Value{
			"attempts": {
				Kind: &pbtypes.Value_NumberValue{
					NumberValue: float64(attempts),
				},
			},
		},
	}
	metadata, _ := pbtypes.MarshalAny(retryStruct)

	return &api.Condition{
		Type:   actorType,
		Status: api.CONDITION_STATUS_UNKNOWN,
		Reason: fmt.Sprintf("retry_attempt_%d", attempts),
		Message: fmt.Sprintf("Actor failed on attempt %d/%d: %s", attempts, maxRetryAttempts, errorMsg),
		Metadata: metadata,
		LastUpdatedTimestamp: time.Now().UnixMilli(),
	}
}

func (e *DefaultEngine[T]) addRetryMetadata(condition *api.Condition, attempts int32) *api.Condition {
	retryStruct := &pbtypes.Struct{
		Fields: map[string]*pbtypes.Value{
			"attempts": {
				Kind: &pbtypes.Value_NumberValue{
					NumberValue: float64(attempts),
				},
			},
		},
	}
	metadata, _ := pbtypes.MarshalAny(retryStruct)
	condition.Metadata = metadata
	return condition
}
