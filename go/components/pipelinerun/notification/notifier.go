// Package notification provides PipelineRun-specific notification functionality.
//
// This package implements the notification logic for pipeline run state changes,
// integrating with the base notification provider to send alerts when pipeline
// runs complete, fail, or are killed. It handles state transition detection,
// notification filtering, and event generation.
package notification

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/base/notification/provider"
	"github.com/michelangelo-ai/michelangelo/go/base/notification/types"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

// PipelineRunNotifier handles notification logic for pipeline run state changes.
//
// This component integrates with the PipelineRun controller to detect state
// transitions and send appropriate notifications. It filters notifications
// based on configuration and only sends alerts for meaningful state changes
// to avoid spamming users with intermediate updates.
type PipelineRunNotifier struct {
	provider provider.NotificationProvider
	logger   *zap.Logger
}

// NewPipelineRunNotifier creates a new pipeline run notifier.
//
// The notifier requires a notification provider for delivery and a logger
// for observability. The provider abstracts the delivery mechanism, allowing
// for different implementations (Cadence workflows, direct API calls, etc.).
func NewPipelineRunNotifier(
	provider provider.NotificationProvider,
	logger *zap.Logger,
) *PipelineRunNotifier {
	return &PipelineRunNotifier{
		provider: provider,
		logger:   logger.With(zap.String("component", "pipeline-run-notifier")),
	}
}

// NotifyOnStateChange detects pipeline run state transitions and sends notifications.
//
// This method compares the old and new pipeline run states to detect changes,
// then filters and sends notifications based on the configured event types.
// It only triggers notifications for terminal states (SUCCEEDED, FAILED, KILLED, SKIPPED)
// to avoid overwhelming users with intermediate status updates.
//
// The method implements non-blocking error handling - notification failures are logged
// but do not affect the return status, ensuring that notification issues don't impact
// pipeline execution or controller reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - oldPipelineRun: Pipeline run state before reconciliation
//   - newPipelineRun: Pipeline run state after reconciliation
//
// Returns an error only if there are critical issues with notification processing.
// Delivery failures are logged but not returned to maintain controller stability.
func (n *PipelineRunNotifier) NotifyOnStateChange(
	ctx context.Context,
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
) error {
	if newPipelineRun == nil {
		return nil // Nothing to notify about
	}

	logger := n.logger.With(
		zap.String("pipeline_run", newPipelineRun.Name),
		zap.String("namespace", newPipelineRun.Namespace),
	)

	// Extract effective states for comparison
	oldState := types.GetEffectiveState(oldPipelineRun)
	newState := types.GetEffectiveState(newPipelineRun)

	logger.Debug("Checking state transition",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()))

	// Only process state changes to avoid duplicate notifications
	if oldState == newState {
		logger.Debug("No state change detected, skipping notification")
		return nil
	}

	// Map the new state to a notification event type
	eventType := types.MapStateToEventType(newState)
	if eventType == types.EventTypeInvalid {
		logger.Debug("State does not trigger notifications",
			zap.String("state", newState.String()))
		return nil
	}

	// Check if notifications are configured for this pipeline run
	if len(newPipelineRun.Spec.Notifications) == 0 {
		logger.Debug("No notifications configured for pipeline run")
		return nil
	}

	// Filter notifications for this specific event type
	relevantNotifications := types.FilterNotificationsForEvent(
		newPipelineRun.Spec.Notifications,
		eventType,
	)

	if len(relevantNotifications) == 0 {
		logger.Debug("No notifications configured for this event type",
			zap.String("event_type", eventType.String()))
		return nil
	}

	logger.Info("State change detected, sending notifications",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()),
		zap.String("event_type", eventType.String()),
		zap.Int("notification_count", len(relevantNotifications)))

	// Create notification event with cropped pipeline run to reduce payload size
	croppedPipelineRun := types.CropPipelineRun(newPipelineRun)
	event := &types.NotificationEvent{
		EventType:     eventType,
		ResourceType:  types.ResourceTypePipelineRun,
		PipelineRun:   croppedPipelineRun,
		Notifications: relevantNotifications,
	}

	// Send notification via provider (non-blocking for controller stability)
	if err := n.provider.SendPipelineRunNotification(ctx, event); err != nil {
		// Log error but don't fail reconciliation - notifications are best-effort
		logger.Warn("Failed to send notification",
			zap.Error(err),
			zap.String("event_type", eventType.String()))
		// Note: We could add metrics here to track notification failures
		return nil // Don't propagate notification errors to controller
	}

	logger.Info("Notification sent successfully",
		zap.String("event_type", eventType.String()),
		zap.Int("notification_count", len(relevantNotifications)))

	return nil
}

// ShouldNotify determines if a pipeline run state change should trigger notifications.
//
// This is a utility method that can be used to check whether a state transition
// would result in notifications being sent, without actually sending them.
// Useful for testing and debugging notification logic.
func (n *PipelineRunNotifier) ShouldNotify(
	oldPipelineRun, newPipelineRun *v2pb.PipelineRun,
) bool {
	if newPipelineRun == nil {
		return false
	}

	// Check for state change
	oldState := types.GetEffectiveState(oldPipelineRun)
	newState := types.GetEffectiveState(newPipelineRun)
	if oldState == newState {
		return false
	}

	// Check if state triggers notifications
	eventType := types.MapStateToEventType(newState)
	if eventType == types.EventTypeInvalid {
		return false
	}

	// Check if notifications are configured
	if len(newPipelineRun.Spec.Notifications) == 0 {
		return false
	}

	// Check if any notifications are configured for this event type
	relevantNotifications := types.FilterNotificationsForEvent(
		newPipelineRun.Spec.Notifications,
		eventType,
	)

	return len(relevantNotifications) > 0
}