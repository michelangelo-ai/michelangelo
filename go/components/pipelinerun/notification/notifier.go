// Package notification provides PipelineRun-specific notification functionality.
package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.uber.org/zap"
)

// Config holds operator-supplied configuration for PipelineRunNotifier.
type Config struct {
	// TaskList is the Cadence/Temporal task list on which the notification
	// workflow runs. It must match the task list registered by the worker.
	// To customize, update this value and register the notification workflow
	// and activities on the same task list in your cmd/worker/main.go.
	// Set to "" to disable notifications.
	TaskList string `yaml:"taskList"`
	// StudioBaseURL is the base URL of the platform UI, used to build deep
	// links in notification message bodies.
	// Example: "https://ml.mycompany.com/studio/"
	// If empty, no deep link is included in notification messages.
	StudioBaseURL string `yaml:"studioBaseURL"`
	// SenderEmail is the From address for outgoing email notifications.
	// Required when a real email transport activity is wired. The built-in
	// no-op activity ignores this field — emails will not be delivered until
	// the activity is replaced with a real implementation (SMTP, SendGrid, etc.).
	SenderEmail string `yaml:"senderEmail"`
}

// PipelineRunNotifier starts the notification workflow when a pipeline run
// transitions to a state that has matching notification configuration.
type PipelineRunNotifier struct {
	cfg            Config
	workflowClient clientInterfaces.WorkflowClient
	logger         *zap.Logger
}

// NewPipelineRunNotifier creates a new PipelineRunNotifier.
//
// Returns (nil, nil) when cfg.TaskList is empty — notifications are disabled
// and the caller's nil guard skips dispatch. This allows the controller manager
// to start with notifications disabled.
func NewPipelineRunNotifier(
	cfg Config,
	workflowClient clientInterfaces.WorkflowClient,
	logger *zap.Logger,
) (*PipelineRunNotifier, error) {
	if cfg.TaskList == "" {
		logger.Info("notification.taskList is empty — PipelineRun notifications disabled")
		return nil, nil
	}
	logger.Info("notification task list configured; ensure the worker registers this task list",
		zap.String("taskList", cfg.TaskList))
	return &PipelineRunNotifier{
		cfg:            cfg,
		workflowClient: workflowClient,
		logger:         logger.With(zap.String("component", "pipeline-run-notifier")),
	}, nil
}

// NotifyOnStateChange detects pipeline run state transitions and starts the
// notification workflow when the new state matches a configured event type.
//
// The error returned is from StartWorkflow. Callers (typically a reconciler)
// should log it at Warn level — notification failures must not block pipeline
// run reconciliation.
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

	if !n.shouldNotify(oldPipelineRun, newPipelineRun, logger) {
		return nil
	}

	logger.Info("State change detected, starting notification workflow")

	req := &types.PipelineRunNotificationRequest{
		PipelineRun:   types.CropPipelineRun(newPipelineRun),
		StudioBaseURL: n.cfg.StudioBaseURL,
		SenderEmail:   n.cfg.SenderEmail,
	}

	// Include the new state in the workflow ID so that STARTED and terminal-state
	// workflows for the same run never collide. Cadence/Temporal rejects a
	// StartWorkflow call when a workflow with the same ID is still in flight
	// (AllowDuplicate only permits reuse after the previous execution closes);
	// a constant ID would silently drop the terminal notification if the STARTED
	// workflow is still running.
	workflowID := fmt.Sprintf("%s.%s.notification.%s",
		newPipelineRun.Namespace,
		newPipelineRun.Name,
		strings.ToLower(getEffectiveState(newPipelineRun).String()),
	)
	options := clientInterfaces.StartWorkflowOptions{
		ID:                              workflowID,
		TaskList:                        n.cfg.TaskList,
		ExecutionStartToCloseTimeout:    60 * time.Hour,
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}

	execution, err := n.workflowClient.StartWorkflow(
		ctx,
		options,
		types.PipelineRunNotificationWorkflowName,
		req,
	)
	if err != nil {
		logger.Warn("Failed to start notification workflow", zap.Error(err))
		return err
	}

	logger.Info("Notification workflow started",
		zap.String("workflow_run_id", execution.RunID))
	return nil
}

// shouldNotify reports whether a state change on newPipelineRun should trigger
// a notification workflow.
func (n *PipelineRunNotifier) shouldNotify(
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
	logger *zap.Logger,
) bool {
	oldState := getEffectiveState(oldPipelineRun)
	newState := getEffectiveState(newPipelineRun)

	logger.Debug("Checking state transition",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()))

	if oldState == newState {
		logger.Debug("No state change, skipping notification")
		return false
	}

	if len(newPipelineRun.Spec.Notifications) == 0 {
		logger.Debug("No notifications configured")
		return false
	}

	for _, notif := range newPipelineRun.Spec.Notifications {
		if types.ContainsEventType(notif.EventTypes, newState) {
			return true
		}
	}

	logger.Debug("No notification configured for this state",
		zap.String("state", newState.String()))
	return false
}

// getEffectiveState returns the effective state of a pipeline run, treating nil
// and INVALID as PENDING.
func getEffectiveState(pipelineRun *v2pb.PipelineRun) v2pb.PipelineRunState {
	if pipelineRun == nil {
		return v2pb.PIPELINE_RUN_STATE_PENDING
	}
	if pipelineRun.Status.State == v2pb.PIPELINE_RUN_STATE_INVALID {
		return v2pb.PIPELINE_RUN_STATE_PENDING
	}
	return pipelineRun.Status.State
}
