package plugins

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// DefaultEngine implements the Engine interface following Uber's proven pattern
type DefaultEngine struct{}

// NewEngine creates a new instance of the default engine
func NewEngine() Engine {
	return &DefaultEngine{}
}

// Run executes the plugin by running through the list of actors from the plugin and executing Retrieve and Run
// for each actor. Only the first failing condition will have its Run method executed per engine execution.
func (e *DefaultEngine) Run(ctx context.Context, logger *zap.Logger, plugin Plugin, resource *v2pb.InferenceServer) (*apipb.Condition, error) {
	actors := plugin.GetActors()
	if len(actors) == 0 {
		logger.Info("No actors found in plugin")
		return nil, nil
	}

	conditions := plugin.GetConditions(resource)
	conditionMap := make(map[string]*apipb.Condition)

	// Create map of existing conditions by type
	for _, condition := range conditions {
		if condition != nil {
			conditionMap[condition.Type] = condition
		}
	}

	logger.Info("Running engine with actors", zap.Int("actorCount", len(actors)), zap.Int("existingConditions", len(conditions)))

	var firstFailingCondition *apipb.Condition

	// Execute Retrieve for each actor
	for _, actor := range actors {
		actorType := actor.GetType()
		actorLogger := logger.With(zap.String("actorType", actorType))

		// Get existing condition or create new one
		existingCondition := &apipb.Condition{
			Type:   actorType,
			Status: apipb.CONDITION_STATUS_UNKNOWN,
		}
		if existing, exists := conditionMap[actorType]; exists {
			existingCondition = existing
		}

		// Execute Retrieve to get current condition state
		retrievedCondition, err := actor.Retrieve(ctx, actorLogger, resource, *existingCondition)
		if err != nil {
			actorLogger.Error("Failed to retrieve condition", zap.Error(err))
			// Create failed condition
			retrievedCondition = apipb.Condition{
				Type:    actorType,
				Status:  apipb.CONDITION_STATUS_FALSE,
				Reason:  "RetrieveError",
				Message: fmt.Sprintf("Failed to retrieve condition: %v", err),
			}
		}

		// Update condition in plugin
		plugin.PutCondition(resource, retrievedCondition)

		// Track first failing condition
		if retrievedCondition.Status == apipb.CONDITION_STATUS_FALSE && firstFailingCondition == nil {
			firstFailingCondition = &retrievedCondition
		}

		actorLogger.Info("Retrieved condition",
			zap.String("status", retrievedCondition.Status.String()),
			zap.String("reason", retrievedCondition.Reason),
			zap.String("message", retrievedCondition.Message))
	}

	// If we found a failing condition, execute Run on its actor
	if firstFailingCondition != nil {
		for _, actor := range actors {
			if actor.GetType() == firstFailingCondition.Type {
				runLogger := logger.With(zap.String("actorType", firstFailingCondition.Type))
				runLogger.Info("Executing Run for first failing condition", zap.String("condition", firstFailingCondition.Type))

				if err := actor.Run(ctx, runLogger, resource, firstFailingCondition); err != nil {
					runLogger.Error("Failed to execute Run", zap.Error(err))
					firstFailingCondition.Status = apipb.CONDITION_STATUS_FALSE
					firstFailingCondition.Reason = "RunError"
					firstFailingCondition.Message = fmt.Sprintf("Failed to execute run: %v", err)
				}

				// Update condition in plugin after Run
				plugin.PutCondition(resource, *firstFailingCondition)

				return firstFailingCondition, nil
			}
		}
	}

	logger.Info("All conditions are healthy, no Run execution needed")
	return nil, nil
}
