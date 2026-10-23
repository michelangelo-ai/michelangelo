package cadenceclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/cadence"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"go.uber.org/cadence/.gen/go/shared"
	cadenceClient "go.uber.org/cadence/client"
	"go.uber.org/cadence/encoded"
	"go.uber.org/zap"
)

const (
	pauseReason   = "paused by michelangelo"
	unpauseReason = "unpaused by michelangelo"

	// defaultDecisionTaskTimeout is applied to a schedule's action when the caller passes
	// none: the server rejects a zero decision task timeout on a schedule action.
	defaultDecisionTaskTimeout = 10 * time.Second
)

// mapCadenceStatusToInterface maps Cadence workflow status to our interface status
func mapCadenceStatusToInterface(closeStatus *shared.WorkflowExecutionCloseStatus) clientInterface.WorkflowExecutionStatus {
	if closeStatus == nil {
		return clientInterface.WorkflowExecutionStatusRunning
	}

	switch *closeStatus {
	case shared.WorkflowExecutionCloseStatusCompleted:
		return clientInterface.WorkflowExecutionStatusCompleted
	case shared.WorkflowExecutionCloseStatusFailed:
		return clientInterface.WorkflowExecutionStatusFailed
	case shared.WorkflowExecutionCloseStatusCanceled:
		return clientInterface.WorkflowExecutionStatusCanceled
	case shared.WorkflowExecutionCloseStatusTerminated:
		return clientInterface.WorkflowExecutionStatusTerminated
	case shared.WorkflowExecutionCloseStatusContinuedAsNew:
		return clientInterface.WorkflowExecutionStatusContinuedAsNew
	case shared.WorkflowExecutionCloseStatusTimedOut:
		return clientInterface.WorkflowExecutionStatusTimedOut
	default:
		return clientInterface.WorkflowExecutionStatusUnSpecified
	}
}

// mapOverlapPolicy translates the engine-neutral overlap policy to Cadence's. The zero
// value maps to SkipNew explicitly rather than to Cadence's Unspecified, so the
// behaviour does not depend on the server's default.
func mapOverlapPolicy(policy clientInterface.OverlapPolicy) cadenceClient.ScheduleOverlapPolicy {
	switch policy {
	case clientInterface.OverlapPolicyBuffer:
		return cadenceClient.ScheduleOverlapPolicyBuffer
	case clientInterface.OverlapPolicyConcurrent:
		return cadenceClient.ScheduleOverlapPolicyConcurrent
	case clientInterface.OverlapPolicyCancelPrevious:
		return cadenceClient.ScheduleOverlapPolicyCancelPrevious
	case clientInterface.OverlapPolicyTerminatePrevious:
		return cadenceClient.ScheduleOverlapPolicyTerminatePrevious
	default:
		return cadenceClient.ScheduleOverlapPolicySkipNew
	}
}

// mapCatchUpPolicy translates the engine-neutral catch-up policy to Cadence's. The zero
// value maps to Skip explicitly, for the same reason as mapOverlapPolicy.
func mapCatchUpPolicy(policy clientInterface.CatchUpPolicy) cadenceClient.ScheduleCatchUpPolicy {
	switch policy {
	case clientInterface.CatchUpPolicyOne:
		return cadenceClient.ScheduleCatchUpPolicyOne
	case clientInterface.CatchUpPolicyAll:
		return cadenceClient.ScheduleCatchUpPolicyAll
	default:
		return cadenceClient.ScheduleCatchUpPolicySkip
	}
}

type CadenceClient struct {
	Client   cadenceClient.Client
	Provider string
	Domain   string
	// Logger is optional; nil disables logging.
	Logger *zap.Logger
	// DataConverter encodes the input of schedule actions. It must be the converter Client
	// was built with, so a workflow fired by a schedule decodes the same bytes a directly
	// started workflow would. Nil means Cadence's default converter, which is also what
	// cadenceClient.NewClient falls back to when its Options carry none.
	DataConverter encoded.DataConverter
}

// ensure CadenceClient implements clientInterface.WorkflowClient
var _ clientInterface.WorkflowClient = &CadenceClient{}

// StartWorkflow starts a workflow. A CronSchedule turns the start into a Cadence Schedule
// (see createSchedule); the schedule-only options (StartPaused, the schedule window,
// catch-up and the policies) have no effect on a one-off start, as on Temporal. A one-off
// start whose workflow ID already has an open execution returns that execution rather
// than an error, which is also what the Temporal client does.
func (c *CadenceClient) StartWorkflow(ctx context.Context, options clientInterface.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterface.WorkflowExecution, error) {
	if options.CronSchedule != "" {
		return c.createSchedule(ctx, options, workflowName, args...)
	}

	cadenceOptions := cadenceClient.StartWorkflowOptions{
		ID:                              options.ID,
		TaskList:                        options.TaskList,
		ExecutionStartToCloseTimeout:    options.ExecutionStartToCloseTimeout,
		DecisionTaskStartToCloseTimeout: options.DecisionTaskStartToCloseTimeout,
		WorkflowIDReusePolicy:           cadenceClient.WorkflowIDReusePolicyAllowDuplicate,
	}
	cadenceWorkflowExecution, err := c.Client.StartWorkflow(ctx, cadenceOptions, workflowName, args...)
	if err != nil {
		var started *shared.WorkflowExecutionAlreadyStartedError
		if errors.As(err, &started) && started.RunId != nil {
			// A start that repeats one the caller never recorded (a lost response, or a
			// reconciliation retried before its status write landed) finds the earlier
			// execution still open. The Temporal client returns that execution instead of
			// an error by default; match it so callers behave the same on both engines.
			return &clientInterface.WorkflowExecution{ID: options.ID, RunID: *started.RunId}, nil
		}
		return nil, err
	}
	workflowExecution := clientInterface.WorkflowExecution{
		ID:    cadenceWorkflowExecution.ID,
		RunID: cadenceWorkflowExecution.RunID,
	}
	return &workflowExecution, nil
}

// createSchedule starts a recurring trigger as a Cadence Schedule.
//
// Schedules (go.uber.org/cadence v1.4.0; Cadence server v1.4.1 or later with the
// worker.enableScheduler dynamic-config key on for the domain) replace the legacy
// CronSchedule start option, which could neither pause nor change its cron without a
// restart. The schedule is named after the trigger's workflow ID, as on Temporal, and
// each fired run's workflow ID is that ID followed by the occurrence time. The trigger's
// own workflow ID therefore never has an open execution, which is what the controller's
// status and kill paths already assume for Temporal; an open execution under that ID
// can only be a legacy cron workflow started before this change.
//
// Create is not idempotent. A retry whose first response was lost, or a reconciliation
// that repeats a start the controller never recorded, both fail with "already exists",
// so a failed Create is followed by Describe to tell an existing schedule from a real
// failure. An existing schedule is adopted as-is, except that StartPaused still pauses
// it, as on Temporal.
//
// Catch-up is a schedule backfill over [CatchUpFrom, min(now, ScheduleEndAt)). It runs
// with the Buffer overlap policy for this request only, so the replayed occurrences
// start one at a time and the trigger's own concurrency bound holds; Cadence buffers up
// to 1000 fires, above the 500-occurrence cap the controller enforces before asking. A
// failed backfill is reported as a CatchUpError alongside the execution: the schedule is
// live and the caller must keep the trigger running.
func (c *CadenceClient) createSchedule(ctx context.Context, options clientInterface.StartWorkflowOptions, workflowName string, args ...interface{}) (*clientInterface.WorkflowExecution, error) {
	scheduleID := scheduleIDForWorkflow(options.ID)
	input, err := c.dataConverter().ToData(args...)
	if err != nil {
		return nil, fmt.Errorf("encode input for Cadence schedule %s: %w", scheduleID, err)
	}
	decisionTaskTimeout := options.DecisionTaskStartToCloseTimeout
	if decisionTaskTimeout <= 0 {
		decisionTaskTimeout = defaultDecisionTaskTimeout
	}
	request := &cadenceClient.CreateScheduleRequest{
		ScheduleID: scheduleID,
		Spec: &cadenceClient.ScheduleSpec{
			CronExpression: options.CronSchedule,
			// Zero values mean an unbounded window.
			StartTime: options.ScheduleStartAt,
			EndTime:   options.ScheduleEndAt,
		},
		Action: &cadenceClient.ScheduleAction{
			StartWorkflow: &cadenceClient.ScheduleStartWorkflowAction{
				WorkflowType:                    workflowName,
				TaskList:                        options.TaskList,
				Input:                           input,
				WorkflowIDPrefix:                options.ID,
				ExecutionStartToCloseTimeout:    options.ExecutionStartToCloseTimeout,
				DecisionTaskStartToCloseTimeout: decisionTaskTimeout,
			},
		},
		Policies: &cadenceClient.SchedulePolicies{
			OverlapPolicy: mapOverlapPolicy(options.OverlapPolicy),
			CatchUpPolicy: mapCatchUpPolicy(options.CatchUpPolicy),
		},
	}
	if options.StartPaused {
		request.State = &cadenceClient.ScheduleState{Paused: true}
	}

	execution := &clientInterface.WorkflowExecution{
		ID:    scheduleID,
		RunID: "", // Schedules don't have runIDs
	}
	scheduleClient := c.Client.ScheduleClient()
	if _, err := scheduleClient.Create(ctx, request); err != nil {
		if _, describeErr := scheduleClient.Describe(ctx, scheduleID); describeErr != nil {
			return nil, fmt.Errorf("failed to create Cadence schedule %s: %w", scheduleID, err)
		}
		c.logger().Info("Cadence schedule already exists, adopting it",
			zap.String("schedule_id", scheduleID),
			zap.NamedError("create_error", err))
		if options.StartPaused {
			if pauseErr := scheduleClient.Pause(ctx, scheduleID, pauseReason); pauseErr != nil {
				return nil, fmt.Errorf("failed to pause existing Cadence schedule %s: %w", scheduleID, pauseErr)
			}
		}
		return execution, nil
	}

	if options.CatchUpFrom.IsZero() || options.StartPaused {
		return execution, nil
	}
	backfill := &cadenceClient.BackfillRequest{
		StartTime:     options.CatchUpFrom,
		EndTime:       catchUpEnd(options.ScheduleEndAt, time.Now()),
		OverlapPolicy: cadenceClient.ScheduleOverlapPolicyBuffer,
	}
	if err := scheduleClient.Backfill(ctx, scheduleID, backfill); err != nil {
		return execution, &clientInterface.CatchUpError{
			ScheduleID:  scheduleID,
			CatchUpFrom: options.CatchUpFrom,
			Err:         err,
		}
	}
	return execution, nil
}

// catchUpEnd bounds a catch-up replay. Occurrences are replayed up to now, or up to the
// schedule's own end when that came first, so a window that already closed is not
// replayed past its end.
func catchUpEnd(scheduleEndAt, now time.Time) time.Time {
	if !scheduleEndAt.IsZero() && scheduleEndAt.Before(now) {
		return scheduleEndAt
	}
	return now
}

// scheduleIDForWorkflow names the Cadence schedule that backs a recurring trigger. It is
// the same derivation the Temporal adapter uses, so operators find a trigger's schedule
// under one name on either engine.
func scheduleIDForWorkflow(workflowID string) string {
	return workflowID + "-schedule"
}

func (c *CadenceClient) dataConverter() encoded.DataConverter {
	if c.DataConverter == nil {
		return encoded.GetDefaultDataConverter()
	}
	return c.DataConverter
}

func (c *CadenceClient) logger() *zap.Logger {
	if c.Logger == nil {
		return zap.NewNop()
	}
	return c.Logger
}

func (c *CadenceClient) GetWorkflowExecutionInfo(ctx context.Context, workflowID string, runID string) (*clientInterface.WorkflowExecutionInfo, error) {
	describeWorkflowExecutionResponse, err := c.Client.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	cadenceWorkflowExecutionInfo := describeWorkflowExecutionResponse.WorkflowExecutionInfo
	if cadenceWorkflowExecutionInfo == nil {
		return &clientInterface.WorkflowExecutionInfo{
			Status: clientInterface.WorkflowExecutionStatusUnSpecified,
		}, nil
	}

	var closeStatus *shared.WorkflowExecutionCloseStatus
	if cadenceWorkflowExecutionInfo.IsSetCloseStatus() {
		status := cadenceWorkflowExecutionInfo.GetCloseStatus()
		closeStatus = &status
	}

	status := mapCadenceStatusToInterface(closeStatus)

	return &clientInterface.WorkflowExecutionInfo{
		Status: status,
		Execution: &clientInterface.WorkflowExecution{
			ID:    cadenceWorkflowExecutionInfo.GetExecution().GetWorkflowId(),
			RunID: cadenceWorkflowExecutionInfo.GetExecution().GetRunId(),
		},
		FailureMessage: c.getFailureMessage(ctx, workflowID, runID, status),
	}, nil
}

// getFailureMessage returns a human-readable failure reason for a terminal, unsuccessful
// workflow status. For Failed/Terminated it fetches just the closing history event (a single
// event, not the full history) to extract the underlying failure text. A failure to fetch
// history is swallowed - the workflow status itself is still valid without it.
func (c *CadenceClient) getFailureMessage(ctx context.Context, workflowID string, runID string, status clientInterface.WorkflowExecutionStatus) string {
	switch status {
	case clientInterface.WorkflowExecutionStatusTimedOut:
		return "Workflow execution timed out"
	case clientInterface.WorkflowExecutionStatusFailed, clientInterface.WorkflowExecutionStatusTerminated:
		message, err := c.getCloseEventFailureMessage(ctx, workflowID, runID)
		if err != nil {
			return ""
		}
		return message
	default:
		return ""
	}
}

// getCloseEventFailureMessage fetches the closing history event for a workflow execution and
// extracts a failure message from it, if any.
func (c *CadenceClient) getCloseEventFailureMessage(ctx context.Context, workflowID string, runID string) (string, error) {
	iter := c.Client.GetWorkflowHistory(ctx, workflowID, runID, false, shared.HistoryEventFilterTypeCloseEvent)
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			return "", fmt.Errorf("failed to get close event: %w", err)
		}

		switch event.GetEventType() {
		case shared.EventTypeWorkflowExecutionFailed:
			if attr := event.WorkflowExecutionFailedEventAttributes; attr != nil {
				return extractCadenceFailureMessage(attr.GetReason(), attr.GetDetails()), nil
			}
		case shared.EventTypeWorkflowExecutionTerminated:
			if attr := event.WorkflowExecutionTerminatedEventAttributes; attr != nil {
				return attr.GetReason(), nil
			}
		}
	}
	return "", nil
}

// extractCadenceFailureMessage returns the human-readable text of a Cadence workflow
// failure. The starlark-worker library (used by our pipeline workflows) reports errors via
// workflow.NewCustomError(ctx, code, err.Error()), which Cadence stores as: Reason = code
// (e.g. "invalid-argument") and Details = the actual descriptive error text, encoded with
// the same custom DataConverter the workflow used. Prefer decoding Details over the generic
// code string in Reason, falling back to Reason when there are no decodable details.
func extractCadenceFailureMessage(reason string, details []byte) string {
	if len(details) > 0 {
		converter := cadence.DataConverter{Logger: zap.NewNop()}
		var detail string
		if err := converter.FromData(details, &detail); err == nil && detail != "" {
			return detail
		}
	}
	return reason
}

func (c *CadenceClient) CancelWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	cancelWorkflowOptions := cadenceClient.WithCancelReason(reason)
	return c.Client.CancelWorkflow(ctx, workflowID, runID, cancelWorkflowOptions)
}

func (c *CadenceClient) QueryWorkflow(ctx context.Context, workflowID string, runID string, queryHandlerKey string, queryResult any) error {
	queryWorkflowWithOptionRequest := cadenceClient.QueryWorkflowWithOptionsRequest{
		WorkflowID:            workflowID,
		RunID:                 runID,
		QueryType:             queryHandlerKey,
		QueryConsistencyLevel: shared.QueryConsistencyLevelStrong.Ptr(),
	}
	queryWorkflowWithOptionResponse, err := c.Client.QueryWorkflowWithOptions(ctx, &queryWorkflowWithOptionRequest)
	if err != nil || queryWorkflowWithOptionResponse == nil {
		return fmt.Errorf("error querying workflow workflowID %s, runID %s for queryHandlerKey %s with Error: %w", workflowID, runID, queryHandlerKey, err)
	}
	if queryWorkflowWithOptionResponse.QueryResult.HasValue() {
		if err := queryWorkflowWithOptionResponse.QueryResult.Get(&queryResult); err != nil {
			return fmt.Errorf("error getting query result: %w", err)
		}
	}
	return nil
}

func (c *CadenceClient) GetProvider() string {
	return c.Provider
}

func (c *CadenceClient) GetDomain() string {
	return c.Domain
}

func (c *CadenceClient) ListOpenWorkflow(ctx context.Context, request clientInterface.ListOpenWorkflowExecutionsRequest) (*clientInterface.ListOpenWorkflowExecutionsResponse, error) {
	cadenceRequest := &shared.ListOpenWorkflowExecutionsRequest{
		Domain:          &request.Domain,
		MaximumPageSize: request.MaximumPageSize,
		NextPageToken:   request.NextPageToken,
	}

	// Set start time filter if provided
	if request.StartTimeFilter != nil {
		cadenceRequest.StartTimeFilter = &shared.StartTimeFilter{
			EarliestTime: request.StartTimeFilter.EarliestTime,
			LatestTime:   request.StartTimeFilter.LatestTime,
		}
	}

	// Set execution filter if provided
	if request.ExecutionFilter != nil {
		cadenceRequest.ExecutionFilter = &shared.WorkflowExecutionFilter{
			WorkflowId: &request.ExecutionFilter.WorkflowID,
			RunId:      &request.ExecutionFilter.RunID,
		}
	}

	response, err := c.Client.ListOpenWorkflow(ctx, cadenceRequest)
	if err != nil {
		return nil, err
	}

	// Convert response to our interface format
	executionsInfo := make([]clientInterface.WorkflowExecutionInfo, 0, len(response.Executions))
	for _, exec := range response.Executions {
		var executionTime time.Time
		if exec.ExecutionTime != nil {
			executionTime = time.Unix(0, *exec.ExecutionTime)
		}
		executionsInfo = append(executionsInfo, clientInterface.WorkflowExecutionInfo{
			Execution: &clientInterface.WorkflowExecution{
				ID:    exec.Execution.GetWorkflowId(),
				RunID: exec.Execution.GetRunId(),
			},
			ExecutionTime: executionTime,
			Status:        mapCadenceStatusToInterface(exec.CloseStatus),
		})
	}

	return &clientInterface.ListOpenWorkflowExecutionsResponse{
		Executions:    executionsInfo,
		NextPageToken: response.NextPageToken,
	}, nil
}

func (c *CadenceClient) TerminateWorkflow(ctx context.Context, workflowID string, runID string, reason string) error {
	return c.Client.TerminateWorkflow(ctx, workflowID, runID, reason, nil)
}

func (c *CadenceClient) ResetWorkflow(ctx context.Context, options clientInterface.ResetWorkflowOptions) (*clientInterface.WorkflowExecution, error) {
	resetRequest := &shared.ResetWorkflowExecutionRequest{
		Domain: &c.Domain,
		WorkflowExecution: &shared.WorkflowExecution{
			WorkflowId: &options.WorkflowID,
			RunId:      &options.RunID,
		},
		Reason:                &options.Reason,
		DecisionFinishEventId: &options.EventID,
	}

	if options.RequestID != "" {
		resetRequest.RequestId = &options.RequestID
	}

	response, err := c.Client.ResetWorkflow(ctx, resetRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to reset workflow: %w", err)
	}

	return &clientInterface.WorkflowExecution{
		ID:    options.WorkflowID,
		RunID: response.GetRunId(),
	}, nil
}

func (c *CadenceClient) GetWorkflowExecutionHistory(ctx context.Context, workflowID string, runID string, pageToken []byte, pageSize int32) (*clientInterface.WorkflowHistory, error) {
	// Use iterator-based API for getting history
	iter := c.Client.GetWorkflowHistory(ctx, workflowID, runID, false, shared.HistoryEventFilterTypeAllEvent)

	// Convert Cadence history events to our interface format
	events := make([]clientInterface.HistoryEvent, 0)
	var collectedEvents int32 = 0

	for iter.HasNext() && (pageSize == 0 || collectedEvents < pageSize) {
		event, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to get next history event: %w", err)
		}

		historyEvent := clientInterface.HistoryEvent{
			EventID:   event.GetEventId(),
			EventType: event.GetEventType().String(),
			EventTime: time.Unix(0, event.GetTimestamp()),
			Details:   make(map[string]interface{}),
		}

		// Add relevant event details based on event type
		// Currently handles: DecisionTaskCompleted, ActivityTaskScheduled,
		// ActivityTaskCompleted, ActivityTaskFailed, WorkflowExecutionStarted
		switch event.GetEventType() {
		case shared.EventTypeDecisionTaskCompleted:
			if attr := event.DecisionTaskCompletedEventAttributes; attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
			}
		case shared.EventTypeActivityTaskScheduled:
			if attr := event.ActivityTaskScheduledEventAttributes; attr != nil {
				historyEvent.Details["activity_id"] = attr.GetActivityId()
				historyEvent.Details["activity_type"] = attr.GetActivityType().GetName()
			}
		case shared.EventTypeActivityTaskCompleted:
			if attr := event.ActivityTaskCompletedEventAttributes; attr != nil {
				historyEvent.Details["identity"] = attr.GetIdentity()
				historyEvent.Details["scheduled_event_id"] = attr.GetScheduledEventId()
				// Note: Cadence ActivityTaskCompletedEventAttributes doesn't directly contain activity_id
				// The activity_id is typically found in the corresponding ActivityTaskScheduled event
			}
		case shared.EventTypeActivityTaskFailed:
			if attr := event.ActivityTaskFailedEventAttributes; attr != nil {
				historyEvent.Details["reason"] = attr.GetReason()
				historyEvent.Details["details"] = attr.GetDetails()
				historyEvent.Details["identity"] = attr.GetIdentity()
			}
		case shared.EventTypeWorkflowExecutionStarted:
			if attr := event.WorkflowExecutionStartedEventAttributes; attr != nil {
				historyEvent.Details["workflow_type"] = attr.GetWorkflowType().GetName()
				historyEvent.Details["task_list"] = attr.GetTaskList().GetName()
			}
		}

		events = append(events, historyEvent)
		collectedEvents++
	}

	// Note: Iterator-based API doesn't provide page tokens, so we return empty token
	return &clientInterface.WorkflowHistory{
		Events:        events,
		NextPageToken: pageToken,
	}, nil
}

// Event type abstraction methods for Cadence
func (c *CadenceClient) GetActivityTaskScheduledEventType() string {
	return shared.EventTypeActivityTaskScheduled.String()
}

func (c *CadenceClient) GetActivityTaskCompletedEventType() string {
	return shared.EventTypeActivityTaskCompleted.String()
}

func (c *CadenceClient) GetDecisionTaskCompletedEventType() string {
	return shared.EventTypeDecisionTaskCompleted.String()
}

// PauseTrigger pauses the schedule that backs the trigger. A trigger started before
// schedule support runs as a legacy cron workflow, has no schedule, and cannot be
// paused; the error says so.
func (c *CadenceClient) PauseTrigger(ctx context.Context, workflowID string) error {
	scheduleID := scheduleIDForWorkflow(workflowID)
	if err := c.Client.ScheduleClient().Pause(ctx, scheduleID, pauseReason); err != nil {
		return scheduleError("pause", workflowID, scheduleID, err)
	}
	return nil
}

// UnpauseTrigger resumes the schedule that backs the trigger. Missed occurrences are
// handled by the catch-up policy the schedule was created with (Skip unless the start
// options said otherwise), which Unspecified defers to here.
func (c *CadenceClient) UnpauseTrigger(ctx context.Context, workflowID string) error {
	scheduleID := scheduleIDForWorkflow(workflowID)
	err := c.Client.ScheduleClient().Unpause(ctx, scheduleID, unpauseReason, cadenceClient.ScheduleCatchUpPolicyUnspecified)
	if err != nil {
		return scheduleError("unpause", workflowID, scheduleID, err)
	}
	return nil
}

// DeleteTrigger deletes the schedule that backs the trigger, then terminates the open
// execution the controller found under the trigger's workflow ID, if any.
//
// Deleting the schedule stops future occurrences. Like Temporal, it leaves a fired run
// that is still in flight to finish on its own; the controller's drain finalizer waits
// for the pipeline runs it created. An open execution under the trigger's own workflow
// ID exists only for a legacy cron workflow, so the termination step is what retires a
// trigger started before schedule support. Both steps are idempotent: a schedule or
// execution that is already gone is not an error.
func (c *CadenceClient) DeleteTrigger(ctx context.Context, workflowID string, runID string) error {
	scheduleID := scheduleIDForWorkflow(workflowID)
	if err := c.Client.ScheduleClient().Delete(ctx, scheduleID); err != nil {
		if !isGone(err) {
			return fmt.Errorf("delete Cadence schedule %s: %w", scheduleID, err)
		}
		c.logger().Info("Cadence schedule already absent during trigger deletion",
			zap.String("schedule_id", scheduleID))
	}
	if runID == "" {
		return nil
	}
	if err := c.Client.TerminateWorkflow(ctx, workflowID, runID, "trigger killed", nil); err != nil {
		if !isGone(err) {
			return err
		}
		c.logger().Info("Cadence workflow already absent during trigger deletion",
			zap.String("workflow_id", workflowID),
			zap.String("run_id", runID))
	}
	return nil
}

// UpdateTrigger changes the cron expression and the action input of the schedule that
// backs the trigger, and sets its paused state.
//
// Cadence's schedule update cannot change the paused state, so the pause or unpause is a
// separate call. The order keeps the schedule from firing between the two when the caller
// is pausing: pause first, then change the spec; when resuming, change the spec first,
// then unpause. A failure between the calls leaves the spec updated but the state
// unchanged, which the controller's next reconciliation retries.
func (c *CadenceClient) UpdateTrigger(ctx context.Context, workflowID string, newCronSchedule string, paused *bool, args []interface{}) error {
	scheduleID := scheduleIDForWorkflow(workflowID)
	scheduleClient := c.Client.ScheduleClient()

	if paused != nil && *paused {
		if err := scheduleClient.Pause(ctx, scheduleID, pauseReason); err != nil {
			return scheduleError("pause", workflowID, scheduleID, err)
		}
	}

	if newCronSchedule != "" || args != nil {
		var input []byte
		if args != nil {
			var err error
			if input, err = c.dataConverter().ToData(args...); err != nil {
				return fmt.Errorf("encode input for Cadence schedule %s: %w", scheduleID, err)
			}
		}
		err := scheduleClient.Update(ctx, scheduleID, func(update *cadenceClient.ScheduleUpdate) error {
			if newCronSchedule != "" {
				if update.Spec == nil {
					update.Spec = &cadenceClient.ScheduleSpec{}
				}
				update.Spec.CronExpression = newCronSchedule
			}
			if args != nil {
				if update.Action == nil || update.Action.StartWorkflow == nil {
					return fmt.Errorf("Cadence schedule %s has no start-workflow action to update", scheduleID)
				}
				update.Action.StartWorkflow.Input = input
			}
			return nil
		})
		if err != nil {
			return scheduleError("update", workflowID, scheduleID, err)
		}
	}

	if paused != nil && !*paused {
		err := scheduleClient.Unpause(ctx, scheduleID, unpauseReason, cadenceClient.ScheduleCatchUpPolicyUnspecified)
		if err != nil {
			return scheduleError("unpause", workflowID, scheduleID, err)
		}
	}
	return nil
}

// scheduleError explains a failed schedule operation. A missing schedule most often means
// the trigger predates schedule support and runs as a legacy cron workflow: it keeps
// firing, but it cannot be paused, resumed or updated in place and has to be recreated.
func scheduleError(operation, workflowID, scheduleID string, err error) error {
	if isGone(err) {
		return fmt.Errorf("%s trigger %s: Cadence schedule %s not found; a trigger started before "+
			"schedule support runs as a legacy cron workflow and must be recreated to be paused, "+
			"resumed or updated: %w", operation, workflowID, scheduleID, err)
	}
	return fmt.Errorf("%s trigger %s: %w", operation, workflowID, err)
}

// isGone reports whether an error means the schedule or workflow execution no longer
// exists. Cadence reports a missing schedule as EntityNotExistsError and a closed
// workflow execution as either EntityNotExistsError or
// WorkflowExecutionAlreadyCompletedError.
func isGone(err error) bool {
	var notExists *shared.EntityNotExistsError
	var completed *shared.WorkflowExecutionAlreadyCompletedError
	return errors.As(err, &notExists) || errors.As(err, &completed)
}
