package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// ActorEngineImpl implements the ActorEngine interface
type ActorEngineImpl struct{}

// NewActorEngine creates a new actor engine
func NewActorEngine() ActorEngine {
	return &ActorEngineImpl{}
}

// ExecuteActors runs actors sequentially and updates conditions
func (e *ActorEngineImpl) ExecuteActors(ctx context.Context, logger logr.Logger, inferenceServer *v2pb.InferenceServer, actors []ConditionActor) error {
	logger.Info("Executing actors", "count", len(actors))
	
	for _, actor := range actors {
		actorLogger := logger.WithValues("actor", actor.GetType())
		actorLogger.Info("Executing actor")
		
		// Execute the actor
		if err := actor.Execute(ctx, actorLogger, inferenceServer); err != nil {
			actorLogger.Error(err, "Actor execution failed")
			
			// Set failed condition
			condition := &apipb.Condition{
				Type:                 actor.GetType(),
				Status:               apipb.CONDITION_STATUS_FALSE,
				LastUpdatedTimestamp: time.Now().UnixMilli(),
				Reason:               "ExecutionFailed",
				Message:              fmt.Sprintf("Actor execution failed: %v", err),
			}
			
			e.updateCondition(inferenceServer, condition)
			return fmt.Errorf("actor %s failed: %w", actor.GetType(), err)
		}
		
		// Evaluate condition after execution
		condition, err := actor.EvaluateCondition(ctx, actorLogger, inferenceServer)
		if err != nil {
			actorLogger.Error(err, "Failed to evaluate condition")
			continue
		}
		
		if condition != nil {
			e.updateCondition(inferenceServer, condition)
			actorLogger.Info("Updated condition", "status", condition.Status, "reason", condition.Reason)
		}
	}
	
	logger.Info("All actors executed successfully")
	return nil
}

// updateCondition updates or adds a condition to the inference server
func (e *ActorEngineImpl) updateCondition(inferenceServer *v2pb.InferenceServer, newCondition *apipb.Condition) {
	if inferenceServer.Status.Conditions == nil {
		inferenceServer.Status.Conditions = []*apipb.Condition{}
	}
	
	// Find existing condition and update it
	for i, condition := range inferenceServer.Status.Conditions {
		if condition.Type == newCondition.Type {
			inferenceServer.Status.Conditions[i] = newCondition
			return
		}
	}
	
	// Add new condition if not found
	inferenceServer.Status.Conditions = append(inferenceServer.Status.Conditions, newCondition)
}