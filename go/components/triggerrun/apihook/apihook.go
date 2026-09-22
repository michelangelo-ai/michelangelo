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

// RegisterTriggerRunAPIHook registers the API hook that validates the schedule
// window on create and update, stamps the owning Pipeline as the controller
// ownerReference on TriggerRuns at creation, and stamps the owning Pipeline's
// type as the michelangelo/SourcePipelineType label. The stamps are best-effort:
// if the owning Pipeline can't be resolved, creation proceeds without them.
func RegisterTriggerRunAPIHook(logger *zap.Logger, apiHandler api.Handler, scheme *runtime.Scheme) {
	v2.RegisterTriggerRunAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		scheme:     scheme,
	})
}

type apiHook struct {
	v2.NoopTriggerRunAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	scheme     *runtime.Scheme
}

func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateTriggerRunRequest) error {
	if err := validateScheduleWindow(&request.TriggerRun.Spec); err != nil {
		return err
	}
	if err := validateBackfillWindowIsPast(&request.TriggerRun.Spec, time.Now()); err != nil {
		return err
	}
	pipelineRef := request.TriggerRun.Spec.GetPipeline()
	if pipelineRef == nil || pipelineRef.GetName() == "" {
		return nil
	}
	namespace := pipelineRef.GetNamespace()
	if namespace == "" {
		namespace = request.TriggerRun.GetNamespace()
	}

	pipeline := &v2.Pipeline{}
	if err := a.apiHandler.Get(ctx, namespace, pipelineRef.GetName(), &metav1.GetOptions{}, pipeline); err != nil {
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

	return cascadedelete.StampOwnerRefOnCreate(ctx, a.logger, a.scheme, request.TriggerRun, pipeline)
}

// BeforeUpdate re-checks the structural window rules only. The "backfill window must
// be in the past" rule is creation-only: an update usually carries an action such as
// KILL for an object that already exists, and refusing it because the clock moved
// would leave exactly the objects that most need killing stuck.
func (a apiHook) BeforeUpdate(_ context.Context, request *v2.UpdateTriggerRunRequest) error {
	return validateScheduleWindow(&request.TriggerRun.Spec)
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
