package apihook

import (
	"context"
	"testing"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func ts(t time.Time) *pbtypes.Timestamp {
	return &pbtypes.Timestamp{Seconds: t.Unix()}
}

func cronSpec(cron string) *v2.Trigger {
	return &v2.Trigger{TriggerType: &v2.Trigger_CronSchedule{CronSchedule: &v2.CronSchedule{Cron: cron}}}
}

// Requests below carry no Pipeline reference on purpose: BeforeCreate returns right
// after validation in that case, so these tests exercise the window rules alone.
func TestBeforeCreateScheduleWindowRules(t *testing.T) {
	now := time.Now()
	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)

	tests := []struct {
		name    string
		spec    v2.TriggerRunSpec
		wantErr string
	}{
		{
			name: "regular schedule with no window is accepted",
			spec: v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")},
		},
		{
			name: "end before start is rejected",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(past),
				EndTimestamp:   ts(past.Add(-time.Hour)),
			},
			wantErr: "end_timestamp must be after start_timestamp",
		},
		{
			name: "end equal to start is rejected",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(past),
				EndTimestamp:   ts(past),
			},
			wantErr: "end_timestamp must be after start_timestamp",
		},
		{
			name: "catchup on a BatchRerun trigger is rejected",
			spec: v2.TriggerRunSpec{
				Trigger: &v2.Trigger{TriggerType: &v2.Trigger_BatchRerun{BatchRerun: &v2.BatchRerun{}}},
				Catchup: true,
			},
			wantErr: "catchup does not apply to a BatchRerun trigger",
		},
		{
			name: "legacy backfill window entirely in the past is accepted",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(past),
				EndTimestamp:   ts(past.Add(24 * time.Hour)),
			},
		},
		{
			// The future-dated backfill that produced runs for dates that had not happened.
			name: "legacy backfill window reaching into the future is rejected",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(past),
				EndTimestamp:   ts(future),
			},
			wantErr: "must lie entirely in the past",
		},
		{
			name: "legacy backfill window entirely in the future is rejected",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(future),
				EndTimestamp:   ts(future.Add(24 * time.Hour)),
			},
			wantErr: "must lie entirely in the past",
		},
		{
			// With catchup the same dates are a schedule window, which may extend forward.
			name: "catch-up window reaching into the future is accepted",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(past),
				EndTimestamp:   ts(future),
				Catchup:        true,
			},
		},
		{
			name: "fixed future window with catchup is accepted",
			spec: v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(future),
				EndTimestamp:   ts(future.Add(24 * time.Hour)),
				Catchup:        true,
			},
		},
		{
			name: "open-ended future end without start is accepted",
			spec: v2.TriggerRunSpec{
				Trigger:      cronSpec("0 * * * *"),
				EndTimestamp: ts(future),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := apiHook{}
			request := &v2.CreateTriggerRunRequest{TriggerRun: &v2.TriggerRun{
				ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
				Spec:       tt.spec,
			}}

			err := h.BeforeCreate(context.Background(), request)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err), "window violations are client errors")
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// An update must be able to reach an object that already exists, however the clock has
// moved since it was created: the past-window rule is creation-only, while the
// clock-independent rules still apply.
func TestBeforeUpdateScheduleWindowRules(t *testing.T) {
	now := time.Now()
	h := apiHook{}

	t.Run("kill on a legacy backfill whose end is in the future is accepted", func(t *testing.T) {
		request := &v2.UpdateTriggerRunRequest{TriggerRun: &v2.TriggerRun{Spec: v2.TriggerRunSpec{
			Trigger:        cronSpec("0 * * * *"),
			StartTimestamp: ts(now.Add(-24 * time.Hour)),
			EndTimestamp:   ts(now.Add(24 * time.Hour)),
			Action:         v2.TRIGGER_RUN_ACTION_KILL,
		}}}
		require.NoError(t, h.BeforeUpdate(context.Background(), request))
	})

	t.Run("end before start is still rejected", func(t *testing.T) {
		request := &v2.UpdateTriggerRunRequest{TriggerRun: &v2.TriggerRun{Spec: v2.TriggerRunSpec{
			Trigger:        cronSpec("0 * * * *"),
			StartTimestamp: ts(now),
			EndTimestamp:   ts(now.Add(-time.Hour)),
		}}}
		err := h.BeforeUpdate(context.Background(), request)
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
