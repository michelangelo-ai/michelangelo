package common

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionsutil "github.com/michelangelo-ai/michelangelo/go/base/conditions/utils"
	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ conditionInterfaces.ConditionActor[*v2pb.Deployment] = &ZonalSoakActor{}

// Metadata field keys — matches internal zonal actor keys exactly.
const (
	_soakLoadedKey      = "modelLoaded"
	_soakLastUpdatedKey = "lastUpdatedSec"
	_defaultSoakPeriod  = float64(20 * 60) // 20 min — matches internal DefaultWaitPeriod
)

// ZonalSoakActor enforces a soak period after a cluster's traffic route is live.
// It mirrors the internal zonal actor's soak logic:
//   - Run() records modelLoaded=true + lastUpdatedSec=<unix> in condition.Metadata (types.Struct)
//   - Retrieve() checks elapsed time; returns TRUE once rolloutPeriodInSeconds have passed
//
// One instance is created per cluster at actor-chain construction time.
type ZonalSoakActor struct {
	target                 *v2pb.ClusterTarget
	rolloutPeriodInSeconds float64
	logger                 *zap.Logger
}

// NewZonalSoakActor creates a soak gate for the given cluster.
func NewZonalSoakActor(target *v2pb.ClusterTarget, rolloutPeriodInSeconds float64, logger *zap.Logger) *ZonalSoakActor {
	period := rolloutPeriodInSeconds
	if period <= 0 {
		period = _defaultSoakPeriod
	}
	return &ZonalSoakActor{
		target:                 target,
		rolloutPeriodInSeconds: period,
		logger:                 logger,
	}
}

// GetType returns a unique condition key per cluster.
func (a *ZonalSoakActor) GetType() string {
	return osscommon.ActorTypeZonalRollout + "-soak-" + a.target.GetClusterId()
}

// Retrieve checks whether the soak period has elapsed.
// Mirrors internal: reads lastUpdatedSec from types.Struct metadata, compares elapsed.
func (a *ZonalSoakActor) Retrieve(_ context.Context, _ *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	if condition.Status == apipb.CONDITION_STATUS_TRUE {
		return conditionsutil.GenerateTrueCondition(condition), nil
	}

	meta := &types.Struct{}
	if condition.Metadata != nil {
		_ = types.UnmarshalAny(condition.Metadata, meta)
	}
	fields := meta.GetFields()

	if len(fields) == 0 {
		// Run hasn't fired yet.
		return conditionsutil.GenerateFalseCondition(condition, "ZonalSoakPending",
			fmt.Sprintf("cluster %s: waiting for soak to begin", a.target.GetClusterId())), nil
	}

	_, loaded := fields[_soakLoadedKey]
	lastUpdatedVal, hasTs := fields[_soakLastUpdatedKey]

	if loaded && hasTs {
		lastUpdatedSec := int64(lastUpdatedVal.GetNumberValue())
		elapsed := time.Now().Unix() - lastUpdatedSec
		if elapsed >= int64(a.rolloutPeriodInSeconds) {
			a.logger.Info("zonal soak complete, advancing to next cluster",
				zap.String("cluster", a.target.GetClusterId()),
				zap.Float64("soakSeconds", a.rolloutPeriodInSeconds))
			return conditionsutil.GenerateTrueCondition(condition), nil
		}
		msg := fmt.Sprintf("cluster %s: soaking %ds / %.0fs", a.target.GetClusterId(), elapsed, a.rolloutPeriodInSeconds)
		a.logger.Info(msg)
		return conditionsutil.GenerateFalseCondition(condition, "ZonalSoaking", msg), nil
	}

	return conditionsutil.GenerateFalseCondition(condition, "ZonalSoakPending",
		fmt.Sprintf("cluster %s: metadata incomplete", a.target.GetClusterId())), nil
}

// Run records modelLoaded=true and lastUpdatedSec=<now> in condition.Metadata.
// Uses types.Struct — same format as the internal zonal actor.
func (a *ZonalSoakActor) Run(_ context.Context, _ *v2pb.Deployment, condition *apipb.Condition) (*apipb.Condition, error) {
	meta := &types.Struct{Fields: map[string]*types.Value{
		_soakLoadedKey:      {Kind: &types.Value_BoolValue{BoolValue: true}},
		_soakLastUpdatedKey: {Kind: &types.Value_NumberValue{NumberValue: float64(time.Now().Unix())}},
	}}
	metadata, err := types.MarshalAny(meta)
	if err != nil {
		return conditionsutil.GenerateFalseCondition(condition, "ZonalSoakMetaError", err.Error()), nil
	}
	condition.Metadata = metadata

	a.logger.Info("zonal soak started",
		zap.String("cluster", a.target.GetClusterId()),
		zap.Float64("soakSeconds", a.rolloutPeriodInSeconds))
	return conditionsutil.GenerateFalseCondition(condition, "ZonalSoakStarted",
		fmt.Sprintf("cluster %s: soak started, waiting %.0fs", a.target.GetClusterId(), a.rolloutPeriodInSeconds)), nil
}
