package apihook

import (
	"context"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/cascadedelete"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

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
