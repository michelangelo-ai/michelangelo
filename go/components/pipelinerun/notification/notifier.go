// Package notification provides PipelineRun-specific notification functionality.
package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/michelangelo-ai/michelangelo/go/worker/workflows/notification"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

// PipelineRunNotifier handles notification logic for pipeline run state changes.
type PipelineRunNotifier struct {
	workflowClient clientInterfaces.WorkflowClient
	logger         *zap.Logger
}

// NewPipelineRunNotifier creates a new pipeline run notifier.
func NewPipelineRunNotifier(
	workflowClient clientInterfaces.WorkflowClient,
	logger *zap.Logger,
) *PipelineRunNotifier {
	return &PipelineRunNotifier{
		workflowClient: workflowClient,
		logger:         logger.With(zap.String("component", "pipeline-run-notifier")),
	}
}

// NotifyOnStateChange detects pipeline run state transitions and sends notifications.
func (n *PipelineRunNotifier) NotifyOnStateChange(
	ctx context.Context,
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
) error {
	if newPipelineRun == nil {
		return nil
	}

	logger := n.logger.With(
		zap.String("pipeline_run", newPipelineRun.Name),
		zap.String("namespace", newPipelineRun.Namespace),
	)

	// Check for state change and determine if we should notify
	if !n.shouldNotify(oldPipelineRun, newPipelineRun, logger) {
		return nil
	}

	logger.Info("State change detected, starting notification workflow")

	// Crop pipeline run to reduce payload size for workflow
	croppedPipelineRun := types.CropPipelineRun(newPipelineRun)

	// Start notification workflow
	workflowID := fmt.Sprintf("%s.%s.notification", newPipelineRun.Namespace, newPipelineRun.Name)
	options := clientInterfaces.StartWorkflowOptions{
		ID:                              workflowID,
		TaskList:                        "pipeline_run",
		ExecutionStartToCloseTimeout:    60 * time.Hour,
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}

	execution, err := n.workflowClient.StartWorkflow(
		ctx,
		options,
		notification.PRNotificationWorkflowName,
		croppedPipelineRun,
	)

	if err != nil {
		logger.Error("Failed to start notification workflow", zap.Error(err))
		// Don't fail reconciliation due to notification issues
		return nil
	}

	logger.Info("Notification workflow started successfully",
		zap.String("workflow_run_id", execution.RunID))

	return nil
}

// shouldNotify determines if a pipeline run state change should trigger notifications.
func (n *PipelineRunNotifier) shouldNotify(
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
	logger *zap.Logger,
) bool {
	if newPipelineRun == nil {
		return false
	}

	// Extract effective states for comparison
	oldState := getEffectiveState(oldPipelineRun)
	newState := getEffectiveState(newPipelineRun)

	logger.Debug("Checking state transition",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()))

	// Only process state changes
	if oldState == newState {
		logger.Debug("No state change detected, skipping notification")
		return false
	}

	// Check if notifications are configured
	if len(newPipelineRun.Spec.Notifications) == 0 {
		logger.Debug("No notifications configured for pipeline run")
		return false
	}

	// Check if any notification is configured for this state
	for _, notif := range newPipelineRun.Spec.Notifications {
		if types.ContainsEventType(notif.EventTypes, newState) {
			return true
		}
	}

	logger.Debug("No notifications configured for this state",
		zap.String("state", newState.String()))
	return false
}

// getEffectiveState returns the effective state of a pipeline run.
func getEffectiveState(pipelineRun *v2pb.PipelineRun) v2pb.PipelineRunState {
	if pipelineRun == nil {
		return v2pb.PIPELINE_RUN_STATE_PENDING
	}
	if pipelineRun.Status.State == v2pb.PIPELINE_RUN_STATE_INVALID {
		return v2pb.PIPELINE_RUN_STATE_PENDING
	}
	return pipelineRun.Status.State
}

// ShouldNotify determines if a pipeline run state change should trigger notifications.
// Public method for testing and debugging.
func (n *PipelineRunNotifier) ShouldNotify(
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
) bool {
	logger := n.logger.With(zap.String("method", "ShouldNotify"))
	return n.shouldNotify(oldPipelineRun, newPipelineRun, logger)
}
