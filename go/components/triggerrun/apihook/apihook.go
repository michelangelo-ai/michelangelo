package apihook

import (
	"context"
	"strings"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/robfig/cron"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

// unwiredTriggerTypesIssue tracks the trigger types the controller has no
// runner registered for yet.
const unwiredTriggerTypesIssue = "https://github.com/michelangelo-ai/michelangelo/issues/548"

// RegisterTriggerRunAPIHook registers the API hook that validates and defaults
// the TriggerRun spec on create and update, refuses to delete a TriggerRun that
// is still live, defaults the environment label on creation, stamps the owning
// Pipeline as the controller ownerReference, and stamps the owning Pipeline's
// type and manifest type as labels. The Pipeline-related stamps are
// best-effort: if the owning Pipeline can't be resolved, creation proceeds
// without them. defaultEnv is the operator-configured default
// (api.UnspecifiedEnvironment is used instead when the operator has configured
// none), mirroring RegisterPipelineRunAPIHook.
func RegisterTriggerRunAPIHook(logger *zap.Logger, apiHandler api.Handler, scheme *runtime.Scheme, defaultEnv string) {
	v2.RegisterTriggerRunAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		scheme:     scheme,
		defaultEnv: defaultEnv,
	})
}

type apiHook struct {
	v2.NoopTriggerRunAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	scheme     *runtime.Scheme
	defaultEnv string
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateTriggerRunRequest) error {
	if err := validateSpec(request.TriggerRun); err != nil {
		return err
	}
	if err := validateOnCreate(request.TriggerRun, time.Now()); err != nil {
		return err
	}

	setIfAbsent(request.TriggerRun, api.EnvironmentLabel, a.defaultEnvironment())

	// TODO(https://github.com/michelangelo-ai/michelangelo/issues/2155): a
	// production-environment / branch-protection restriction on TriggerRun
	// create is still being designed and is not implemented here.

	pipelineRef := request.TriggerRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	// pipelineRef.Namespace was defaulted to the TriggerRun's namespace by validateSpec.
	pipeline := &v2.Pipeline{}
	if err := a.apiHandler.Get(ctx, pipelineRef.GetNamespace(), pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
		if utils.IsNotFoundError(err) {
			return nil
		}
		a.logger.Warn("BeforeCreate: failed to resolve owning Pipeline for ownerRef",
			zap.String("pipeline", pipelineRef.GetName()), zap.Error(err))
		return nil
	}

	if pipelineType := pipeline.Spec.GetType(); pipelineType != v2.PIPELINE_TYPE_INVALID {
		api.StampSourcePipelineTypeLabelOnCreate(request.TriggerRun, pipelineType.String())
	}
	// The worker copies this label onto every PipelineRun the trigger starts, and
	// notifications read it from there.
	if manifestType := pipeline.Spec.GetManifest().GetType(); manifestType != v2.PIPELINE_MANIFEST_TYPE_INVALID {
		request.TriggerRun.ObjectMeta.Labels[api.SourcePipelineManifestTypeLabelName] = manifestType.String()
	}

	return cascadedelete.StampOwnerRefOnCreate(ctx, a.logger, a.scheme, request.TriggerRun, pipeline)
}

// BeforeUpdate re-applies the defaults and the structural rules (validateSpec)
// and nothing else: the creation-only rules are skipped so an object that already
// exists can always be updated, and a kill skips the structural rules too, so an
// object created before a rule existed can always be stopped.
func (a apiHook) BeforeUpdate(_ context.Context, request *v2.UpdateTriggerRunRequest) error {
	if isKillRequest(&request.TriggerRun.Spec) {
		return nil
	}
	return validateSpec(request.TriggerRun)
}

// BeforeDelete refuses to delete a TriggerRun that is still live. Deleting it
// would remove the record of what was running while its schedule and child
// runs are still being torn down; killing it first keeps a KILLED object and
// cancels the in-flight runs. A TriggerRun that does not exist is left to the
// delete itself, which reports NotFound.
func (a apiHook) BeforeDelete(ctx context.Context, request *v2.DeleteTriggerRunRequest) error {
	run := &v2.TriggerRun{}
	if err := a.apiHandler.Get(ctx, request.GetNamespace(), request.GetName(), &metav1.GetOptions{}, run); err != nil {
		if utils.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return rejectIfLive(run)
}

// BeforeDeleteCollection applies the same rule to every TriggerRun the request
// selects. The request's list options are honoured, so a selector-scoped
// delete only checks the objects it would remove; the whole request is refused
// if any of them is live.
func (a apiHook) BeforeDeleteCollection(ctx context.Context, request *v2.DeleteTriggerRunCollectionRequest) error {
	listOptions := request.GetListOptions()
	if listOptions == nil {
		listOptions = &metav1.ListOptions{}
	}
	runs := &v2.TriggerRunList{}
	if err := a.apiHandler.List(ctx, request.GetNamespace(), listOptions, nil, runs); err != nil {
		return err
	}
	for i := range runs.Items {
		if err := rejectIfLive(&runs.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

func rejectIfLive(run *v2.TriggerRun) error {
	if triggerrun.IsTerminalState(run.Status.State) {
		return nil
	}
	return status.Errorf(codes.FailedPrecondition,
		"TriggerRun %s/%s is %s and cannot be deleted: only a FAILED, KILLED or SUCCEEDED TriggerRun can be deleted; "+
			"set spec.action to TRIGGER_RUN_ACTION_KILL to stop it first",
		run.Namespace, run.Name, run.Status.State)
}

// defaultEnvironment returns the configured default, or
// api.UnspecifiedEnvironment when the operator has configured none —
// mirrors pipelinerun/apihook.go's identically-named method.
func (a apiHook) defaultEnvironment() string {
	if a.defaultEnv == "" {
		return api.UnspecifiedEnvironment
	}
	return a.defaultEnv
}

func setIfAbsent(run *v2.TriggerRun, key, value string) {
	if run.ObjectMeta.Labels == nil {
		run.ObjectMeta.Labels = map[string]string{}
	}
	if _, ok := run.ObjectMeta.Labels[key]; !ok {
		run.ObjectMeta.Labels[key] = value
	}
}

// scheduleWindowOf reads spec.start_timestamp and spec.end_timestamp. Zero times mean
// unset; a nil timestamp must not go through TimestampFromProto, which maps it to 1970.
func scheduleWindowOf(spec *v2.TriggerRunSpec) (startAt, endAt time.Time, err error) {
	if spec.StartTimestamp != nil {
		if startAt, err = pbtypes.TimestampFromProto(spec.StartTimestamp); err != nil {
			return startAt, endAt, status.Errorf(codes.InvalidArgument, "invalid start_timestamp: %v", err)
		}
	}
	if spec.EndTimestamp != nil {
		if endAt, err = pbtypes.TimestampFromProto(spec.EndTimestamp); err != nil {
			return startAt, endAt, status.Errorf(codes.InvalidArgument, "invalid end_timestamp: %v", err)
		}
	}
	return startAt, endAt, nil
}

// validateScheduleWindow enforces the clock-independent contract on
// spec.start_timestamp, spec.end_timestamp and spec.catchup that the controller
// relies on, so a bad spec is rejected synchronously instead of surfacing later as a
// FAILED TriggerRun.
func validateScheduleWindow(spec *v2.TriggerRunSpec) error {
	startAt, endAt, err := scheduleWindowOf(spec)
	if err != nil {
		return err
	}
	if !startAt.IsZero() && !endAt.IsZero() && !endAt.After(startAt) {
		return status.Error(codes.InvalidArgument, "end_timestamp must be after start_timestamp")
	}
	if spec.Catchup && spec.Trigger.GetBatchRerun() != nil {
		return status.Error(codes.InvalidArgument, "catchup does not apply to a BatchRerun trigger")
	}
	return nil
}

// validateBackfillWindowIsPast rejects, at creation, a closed window without catchup
// that reaches into the future.
//
// Without catchup such a window still selects the one-shot backfill runner, which
// enumerates every occurrence in the range up front and cannot wait for occurrences
// that have not happened, so it would produce runs for dates that do not exist yet.
// To schedule forward over a window, set catchup: the window then becomes the
// schedule's own StartAt/EndAt.
func validateBackfillWindowIsPast(spec *v2.TriggerRunSpec, now time.Time) error {
	if spec.Catchup {
		return nil
	}
	startAt, endAt, err := scheduleWindowOf(spec)
	if err != nil {
		return err
	}
	if !startAt.IsZero() && !endAt.IsZero() && endAt.After(now) {
		return status.Error(codes.InvalidArgument,
			"a backfill window (start_timestamp and end_timestamp without catchup) must lie entirely in the past; "+
				"set catchup to true to run a schedule over a window that extends into the future")
	}
	return nil
}

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
