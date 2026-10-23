package apihook

import (
	"strings"
	"time"

	"github.com/robfig/cron"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// unwiredTriggerTypesIssue tracks the trigger types the controller has no
// runner registered for yet.
const unwiredTriggerTypesIssue = "https://github.com/michelangelo-ai/michelangelo/issues/548"

// isKillRequest reports whether spec asks the controller to stop the trigger,
// through spec.action or the deprecated spec.kill flag.
func isKillRequest(spec *v2.TriggerRunSpec) bool {
	return spec.Action == v2.TRIGGER_RUN_ACTION_KILL || spec.Kill
}

// defaultSpec resolves a pipeline or revision reference without a namespace in
// the TriggerRun's own namespace. The worker copies both references verbatim
// onto each PipelineRun, with no fallback of its own.
func defaultSpec(run *v2.TriggerRun) {
	if pipeline := run.Spec.Pipeline; pipeline != nil && pipeline.Namespace == "" {
		pipeline.Namespace = run.Namespace
	}
	if revision := run.Spec.Revision; revision != nil && revision.Namespace == "" {
		revision.Namespace = run.Namespace
	}
}

// validateSpec applies the defaults and the clock-independent rules that hold
// for the whole life of a TriggerRun, on create and on every update that is
// not a kill.
func validateSpec(run *v2.TriggerRun) error {
	defaultSpec(run)
	if err := validateScheduleWindow(&run.Spec); err != nil {
		return err
	}
	return validateTrigger(run.Spec.Trigger)
}

// validateTrigger accepts only a trigger the controller has a runner for and
// whose schedule the controller and worker can parse. interval_schedule and
// batch_rerun have no runner registered yet (see unwiredTriggerTypesIssue);
// dispatching them would dereference a nil Runner on the first reconcile.
func validateTrigger(trigger *v2.Trigger) error {
	switch {
	case trigger.GetBatchRerun() != nil:
		return status.Errorf(codes.InvalidArgument,
			"spec.trigger.batch_rerun is not supported yet: no runner is registered for it (%s)", unwiredTriggerTypesIssue)
	case trigger.GetIntervalSchedule() != nil:
		return status.Errorf(codes.InvalidArgument,
			"spec.trigger.interval_schedule is not supported yet: no runner is registered for it (%s)", unwiredTriggerTypesIssue)
	}
	cronSchedule := trigger.GetCronSchedule()
	if cronSchedule == nil {
		return status.Error(codes.InvalidArgument, "spec.trigger.cron_schedule is required")
	}
	if _, err := cron.ParseStandard(cronSchedule.GetCron()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid spec.trigger.cron_schedule.cron %q: %v", cronSchedule.GetCron(), err)
	}
	return nil
}

// validateOnCreate applies the rules that are only checked when the object is
// created. They are not re-checked on update, so an object created before a
// rule existed, or whose window has since moved into the past, can still be
// updated and killed.
func validateOnCreate(run *v2.TriggerRun, now time.Time) error {
	if err := validateBackfillWindowIsPast(&run.Spec, now); err != nil {
		return err
	}
	if errs := validation.IsValidLabelValue(run.Name); len(errs) > 0 {
		return status.Errorf(codes.InvalidArgument,
			"metadata.name %q is stamped as the %s label on every PipelineRun this trigger starts and must be a valid label value: %s",
			run.Name, triggerrun.TriggeredByLabel, strings.Join(errs, "; "))
	}
	if triggerrun.GetTriggerType(run) != triggerrun.TriggerTypeBackfill {
		return nil
	}
	return validateBackfillWindow(&run.Spec)
}

// validateBackfillWindow checks a one-shot backfill (both timestamps, no
// catchup). It runs once, at the revision it was created with, so auto_flip
// has nothing to flip; and its window must contain at least one occurrence,
// counted the way the backfill worker enumerates them, or it would start
// nothing and end SUCCEEDED.
func validateBackfillWindow(spec *v2.TriggerRunSpec) error {
	if spec.AutoFlip {
		return status.Error(codes.InvalidArgument,
			"auto_flip cannot be set on a backfill window: a backfill runs once, at the revision it was created with")
	}
	startAt, endAt, err := scheduleWindowOf(spec)
	if err != nil {
		return err
	}
	cronExpr := spec.Trigger.GetCronSchedule().GetCron()
	occurrences, err := triggerrun.BackfillRunTimestamps(startAt, endAt, cronExpr)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if len(occurrences) == 0 {
		return status.Errorf(codes.InvalidArgument,
			"the backfill window [%s, %s] contains no occurrence of cron %q; widen the window or change the schedule",
			startAt.UTC().Format(time.RFC3339), endAt.UTC().Format(time.RFC3339), cronExpr)
	}
	return nil
}
