package triggerrun

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	gogoproto "github.com/gogo/protobuf/proto"
	pbtypes "github.com/gogo/protobuf/types"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/robfig/cron"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// cronTrigger implements the Runner interface for cron-scheduled recurring workflows.
//
// This implementation manages workflows that execute on a recurring schedule defined by
// cron expressions. The workflow continues running until explicitly killed, spawning
// child workflow executions at each scheduled interval.
//
// The cron schedule is read from TriggerRun.Spec.Trigger.CronSchedule.Cron and passed
// to the workflow engine's CronSchedule option.
type cronTrigger struct {
	Log            logr.Logger                    // Structured logger for trigger operations
	WorkflowClient clientInterface.WorkflowClient // Workflow engine client (Cadence/Temporal)
}

// NewCronTrigger creates a new cron trigger Runner.
//
// The returned Runner manages recurring scheduled workflows using cron expressions.
// It requires a logger for structured logging and a workflow client for interacting
// with the workflow engine.
func NewCronTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient) Runner {
	return &cronTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
	}
}

// Run starts a recurring cron-scheduled workflow.
//
// This method performs the following operations:
//  1. Generate deterministic workflow ID from namespace and name
//  2. Check if workflow is already running (idempotent start)
//  3. Configure workflow options with cron schedule
//  4. Start workflow execution with "trigger.CronTrigger" workflow type
//  5. Return status with workflow URL for monitoring
//
// The workflow uses:
//   - ID: <namespace>.<name> (deterministic for idempotency)
//   - TaskList: "trigger_run"
//   - ExecutionStartToCloseTimeout: 1 year (effectively no timeout)
//   - DecisionTaskStartToCloseTimeout: 30 seconds
//   - CronSchedule: From TriggerRun spec
//
// Returns State=RUNNING when an active schedule starts successfully, State=PAUSED when
// a PAUSE action is applied during creation, or State=FAILED if workflow start fails.
func (r *cronTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateWorkflowID(triggerRun)
	window, err := resolveScheduleWindow(triggerRun, time.Now())
	if err != nil {
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("resolve schedule window for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	opt := clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, practically no timeout
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
		CronSchedule:                    triggerRun.Spec.Trigger.GetCronSchedule().GetCron(),
		StartPaused:                     triggerRun.Spec.Action == v2pb.TRIGGER_RUN_ACTION_PAUSE,
		ScheduleStartAt:                 window.startAt,
		ScheduleEndAt:                   window.endAt,
		CatchUpFrom:                     window.catchUpFrom,
	}
	domain := r.WorkflowClient.GetDomain()
	rid, err := getWorkflowOpenRunID(ctx, wid, r.WorkflowClient, domain)
	if err != nil {
		// log the error and continue
		log.Error(err, "failed to get open workflow execution",
			"operation", "get_workflow_runid",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
	}
	if rid != nil && *rid != "" {
		log.Info("scheduled workflow already running",
			"operation", "run_cron_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid,
			"runId", *rid)
		// Finding an existing workflow proves only that it is running, not that
		// its schedule action contains the current TriggerRun input. Leave the
		// Actual* fields empty so Update verifies and refreshes them on the next
		// reconciliation.
		status := v2pb.TriggerRunStatus{
			State:  v2pb.TRIGGER_RUN_STATE_RUNNING,
			LogUrl: getWorkflowURL(wid, r.WorkflowClient.GetProvider()),
		}
		if opt.StartPaused {
			paused := true
			if updateErr := r.WorkflowClient.UpdateTrigger(ctx, wid, "", &paused, nil); updateErr != nil {
				status.State = v2pb.TRIGGER_RUN_STATE_FAILED
				status.ErrorMessage = updateErr.Error()
				return status, fmt.Errorf("pause existing trigger %s/%s: %w",
					triggerRun.Namespace, triggerRun.Name, updateErr)
			}
			status.State = v2pb.TRIGGER_RUN_STATE_PAUSED
		}
		return status, nil
	}
	inputHash, err := scheduleInputHash(triggerRun)
	if err != nil {
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("hash schedule input for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	log.Info("starting scheduled workflow",
		"operation", "start_workflow",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"workflowId", opt.ID,
		"taskList", opt.TaskList)
	exec, err := r.WorkflowClient.StartWorkflow(
		ctx, opt, "trigger.CronTrigger", CreateTriggerRequest{TriggerRun: scheduleWorkflowInput(triggerRun)})

	// A failed catch-up is not a failed start. The schedule is live and firing forward, so
	// failing here would mark the TriggerRun terminal (see isTerminateState) and leave a
	// running schedule that nothing reconciles. Keep the trigger running and record the
	// missed window instead. It is not replayed automatically yet: until the reconciler
	// derives the pending occurrences from the engine's own run list, re-issuing a
	// backfill whose outcome is unknown risks duplicate runs for the same period.
	var catchUpWarning string
	var catchUpErr *clientInterface.CatchUpError
	if errors.As(err, &catchUpErr) {
		log.Error(err, "cron trigger is running but its catch-up did not replay",
			"operation", "start_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", opt.ID,
			"catchUpFrom", catchUpErr.CatchUpFrom.UTC().Format(time.RFC3339))
		catchUpWarning = err.Error()
		err = nil
	}

	if err != nil {
		log.Error(err, "failed to start scheduled workflow",
			"operation", "start_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", opt.ID)
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("start workflow for trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	r.Log.Info("scheduled workflow created",
		"operation", "workflow_started",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"paused", opt.StartPaused,
		"execution_id", exec.ID,
		"run_id", exec.RunID)
	status := recurringTriggerStatus(triggerRun, wid, opt.CronSchedule, inputHash, r.WorkflowClient.GetProvider())
	if opt.StartPaused {
		status.State = v2pb.TRIGGER_RUN_STATE_PAUSED
	}
	// Carried on a running trigger so the missed window stays visible to an operator
	// rather than only appearing once in the controller log.
	status.ErrorMessage = catchUpWarning
	return status, nil
}

// maxCatchUpOccurrences bounds how many missed occurrences one catch-up may replay.
//
// The bound is on occurrence count rather than wall-clock age so that a daily cron can
// catch up more than a year while a per-minute cron is stopped after a few hours. A
// mistyped year on a frequent cron would otherwise enqueue thousands of pipeline runs,
// so an over-large window is rejected rather than silently clamped.
const maxCatchUpOccurrences = 500

// scheduleWindow is the engine-facing reading of spec.start_timestamp,
// spec.end_timestamp and spec.catchup. Zero times mean "unset".
type scheduleWindow struct {
	startAt     time.Time
	endAt       time.Time
	catchUpFrom time.Time
}

// resolveScheduleWindow validates the window and decides whether a catch-up is due.
//
// Nothing here depends on "now" except the catch-up range itself: the window is
// passed to the engine as-is, and catch-up is requested only when the spec asks for
// it and the window has already opened. A future start simply idles the schedule.
//
// The explicit nil checks are required: types.TimestampFromProto maps a nil timestamp
// to the Unix epoch, which would read as a window starting in 1970.
func resolveScheduleWindow(triggerRun *v2pb.TriggerRun, now time.Time) (scheduleWindow, error) {
	var window scheduleWindow
	spec := &triggerRun.Spec
	if spec.StartTimestamp != nil {
		startAt, err := pbtypes.TimestampFromProto(spec.StartTimestamp)
		if err != nil {
			return window, fmt.Errorf("invalid start_timestamp: %w", err)
		}
		window.startAt = startAt.UTC()
	}
	if spec.EndTimestamp != nil {
		endAt, err := pbtypes.TimestampFromProto(spec.EndTimestamp)
		if err != nil {
			return window, fmt.Errorf("invalid end_timestamp: %w", err)
		}
		window.endAt = endAt.UTC()
	}
	if !window.startAt.IsZero() && !window.endAt.IsZero() && !window.endAt.After(window.startAt) {
		return window, fmt.Errorf("end_timestamp %s must be after start_timestamp %s",
			window.endAt.Format(time.RFC3339), window.startAt.Format(time.RFC3339))
	}
	if !spec.Catchup || window.startAt.IsZero() || !window.startAt.Before(now) {
		return window, nil
	}

	catchUpTo := now
	if !window.endAt.IsZero() && window.endAt.Before(now) {
		catchUpTo = window.endAt
	}
	cronExpr := spec.Trigger.GetCronSchedule().GetCron()
	occurrences, err := countCronOccurrences(cronExpr, window.startAt, catchUpTo, maxCatchUpOccurrences)
	if err != nil {
		return window, err
	}
	if occurrences > maxCatchUpOccurrences {
		return window, fmt.Errorf(
			"catch-up from %s would replay more than %d occurrences of %q; "+
				"narrow the window or split it across several TriggerRuns",
			window.startAt.Format(time.RFC3339), maxCatchUpOccurrences, cronExpr)
	}
	window.catchUpFrom = window.startAt
	return window, nil
}

// countCronOccurrences counts the cron occurrences in [from, to). It stops as soon as
// the count exceeds limit, so an absurd window is rejected without being walked.
func countCronOccurrences(cronExpr string, from, to time.Time, limit int) (int, error) {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return 0, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	count := 0
	// Next is strictly after its argument at second granularity; step back one second
	// so an occurrence that falls exactly on `from` is counted. A zero result means the
	// schedule has no further occurrence within the parser's search horizon.
	for t := schedule.Next(from.Add(-time.Second)); !t.IsZero() && t.Before(to); t = schedule.Next(t) {
		count++
		if count > limit {
			break
		}
	}
	return count, nil
}

func recurringTriggerStatus(
	triggerRun *v2pb.TriggerRun, workflowID string, cron string, inputHash string, provider string,
) v2pb.TriggerRunStatus {
	return v2pb.TriggerRunStatus{
		State:  v2pb.TRIGGER_RUN_STATE_RUNNING,
		LogUrl: getWorkflowURL(workflowID, provider),
		ActualTrigger: &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: cron},
			},
		},
		ActualNotifications:     triggerRun.Spec.Notifications,
		ActualScheduleInputHash: inputHash,
	}
}

// Kill stops the cron trigger by delegating to the shared killWorkflow utility.
//
// killWorkflow handles engine-specific cleanup via DeleteTrigger and then
// terminates any open workflow execution.
//
// Returns State=KILLED on success.
func (r *cronTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient)
}

// GetStatus retrieves the execution status of a cron-scheduled workflow.
//
// This method checks for open workflow executions and maps workflow states to
// TriggerRun states. For cron triggers, the workflow should remain running until
// explicitly killed, so an active execution indicates RUNNING state.
//
// The status check is delegated to getRecurringRunWorkflowStatus which handles
// state mapping for recurring workflows:
//   - Open execution exists → RUNNING
//   - Terminated/Canceled → KILLED
//   - Failed/TimedOut → FAILED
//
// Returns the current TriggerRunStatus with state and error information if applicable.
func (r *cronTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	return getRecurringRunWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// Pause suspends the cron trigger schedule to prevent new executions.
//
// This method pauses the Temporal Schedule associated with the cron trigger,
// preventing new workflow executions from being scheduled while keeping the
// trigger run in a recoverable state.
//
// Returns TriggerRunStatus with State=PAUSED on success.
func (c *cronTrigger) Pause(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	return pauseWorkflow(ctx, triggerRun, c.Log, c.WorkflowClient, c.WorkflowClient.GetDomain())
}

// Resume reactivates a paused cron trigger schedule.
//
// This method resumes the Temporal Schedule associated with the cron trigger,
// allowing new workflow executions to be scheduled according to the original
// cron expression.
//
// Returns TriggerRunStatus with State=RUNNING on success.
func (c *cronTrigger) Resume(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	return resumeWorkflow(ctx, triggerRun, c.Log, c.WorkflowClient, c.WorkflowClient.GetDomain())
}

// Update synchronizes cron schedule changes from the TriggerRun spec to the workflow engine.
//
// This method ensures that changes in TriggerRun.Spec.Trigger.CronSchedule.Cron are reflected
// in the workflow engine's schedule. It performs the following operations:
//  1. Get the desired cron schedule from the TriggerRun spec
//  2. Query the actual cron schedule from the workflow engine
//  3. Compare the desired and actual schedules
//  4. If different, update the workflow engine schedule to match the spec
//
// For Temporal, this updates the schedule via ScheduleClient.Update. For Cadence, this is a no-op
// since Cadence doesn't support external schedule updates.
//
// Returns the current TriggerRunStatus (preserving state) on success, or an error status if
// the update fails.
func (c *cronTrigger) Update(ctx context.Context, triggerRun *v2pb.TriggerRun, action v2pb.TriggerRunAction) (v2pb.TriggerRunStatus, bool, error) {
	log := c.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})

	desiredCron := triggerRun.Spec.Trigger.GetCronSchedule().GetCron()
	if desiredCron == "" {
		log.Info("no cron schedule in spec, skipping update")
		return triggerRun.Status, false, nil
	}

	var actualCron string
	if triggerRun.Status.ActualTrigger != nil && triggerRun.Status.ActualTrigger.GetCronSchedule() != nil {
		actualCron = triggerRun.Status.ActualTrigger.GetCronSchedule().GetCron()
	}

	cronDrifted := actualCron != desiredCron
	notifDrifted := !notificationsEqual(triggerRun.Spec.Notifications, triggerRun.Status.ActualNotifications)
	desiredInputHash, err := scheduleInputHash(triggerRun)
	if err != nil {
		errorStatus := triggerRun.Status
		errorStatus.ErrorMessage = err.Error()
		return errorStatus, false, fmt.Errorf("hash schedule input for trigger %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}
	inputDrifted := triggerRun.Status.ActualScheduleInputHash != desiredInputHash

	// Determine if we need to set paused state atomically with the update
	var paused *bool
	actionHandled := false
	if action == v2pb.TRIGGER_RUN_ACTION_PAUSE {
		p := true
		paused = &p
	} else if action == v2pb.TRIGGER_RUN_ACTION_RESUME {
		p := false
		paused = &p
	}

	if !cronDrifted && !notifDrifted && !inputDrifted && paused == nil {
		return triggerRun.Status, false, nil
	}

	wid := generateWorkflowID(triggerRun)

	// Cron is also embedded in the workflow input and is used to calculate
	// LAST_SCHEDULE_TIMESTAMP, so refresh the action args for cron drift even if
	// the persisted input hash is unexpectedly already current.
	var args []interface{}
	if inputDrifted || cronDrifted || notifDrifted {
		args = []interface{}{CreateTriggerRequest{TriggerRun: scheduleWorkflowInput(triggerRun)}}
	}

	// cronToUpdate is empty when only notifications/paused changed — UpdateTrigger
	// skips the cron spec update in that case.
	cronToUpdate := ""
	if cronDrifted {
		cronToUpdate = desiredCron
	}

	log.Info("drift detected, updating workflow engine schedule",
		"cronDrifted", cronDrifted,
		"notifDrifted", notifDrifted,
		"inputDrifted", inputDrifted,
		"desiredCron", desiredCron,
		"actualCron", actualCron,
		"atomicPaused", paused)

	err = c.WorkflowClient.UpdateTrigger(ctx, wid, cronToUpdate, paused, args)
	if err != nil {
		log.Error(err, "failed to update trigger in workflow engine",
			"workflowId", wid,
			"desiredCron", desiredCron)
		// Update ActualNotifications even on failure to prevent infinite retry loops
		// when the workflow is in a bad state (e.g. signal limit exceeded).
		// We still return the error to surface it in status, but mark notifications
		// as synced to avoid repeated doomed update attempts.
		errorStatus := triggerRun.Status
		if notifDrifted {
			errorStatus.ActualNotifications = triggerRun.Spec.Notifications
		}
		errorStatus.ErrorMessage = err.Error()
		return errorStatus, false, fmt.Errorf("update trigger for %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}

	log.Info("successfully updated trigger",
		"workflowId", wid,
		"newCron", desiredCron,
		"atomicPaused", paused)

	newStatus := triggerRun.Status
	if cronDrifted {
		newStatus.ActualTrigger = &v2pb.Trigger{
			TriggerType: &v2pb.Trigger_CronSchedule{
				CronSchedule: &v2pb.CronSchedule{Cron: desiredCron},
			},
		}
	}
	if notifDrifted {
		newStatus.ActualNotifications = triggerRun.Spec.Notifications
	}
	if len(args) > 0 {
		newStatus.ActualScheduleInputHash = desiredInputHash
	}

	if paused != nil {
		actionHandled = true
		if *paused {
			newStatus.State = v2pb.TRIGGER_RUN_STATE_PAUSED
		} else {
			newStatus.State = v2pb.TRIGGER_RUN_STATE_RUNNING
		}
		newStatus.ErrorMessage = ""
	}

	return newStatus, actionHandled, nil
}

// notificationsEqual reports whether two notification slices are identical.
// Order matters: notifications are compared positionally.
func notificationsEqual(a, b []*v2pb.Notification) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !gogoproto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
