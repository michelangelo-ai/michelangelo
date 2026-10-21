package apihook

import (
	"context"
	"strings"
	"testing"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apimocks "github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Fixed dates keep the backfill cases deterministic and in the past.
var (
	jan10 = time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	jan11 = jan10.Add(24 * time.Hour)
)

func intervalSpec(d time.Duration) *v2.Trigger {
	return &v2.Trigger{TriggerType: &v2.Trigger_IntervalSchedule{IntervalSchedule: &v2.IntervalSchedule{
		Interval: pbtypes.DurationProto(d),
	}}}
}

func batchRerunSpec() *v2.Trigger {
	return &v2.Trigger{TriggerType: &v2.Trigger_BatchRerun{BatchRerun: &v2.BatchRerun{}}}
}

func newRun(name string, spec v2.TriggerRunSpec) *v2.TriggerRun {
	return &v2.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec:       spec,
	}
}

// Requests below carry no Pipeline reference, so BeforeCreate returns right after
// validation and these cases exercise the spec rules alone.
func TestBeforeCreateSpecRules(t *testing.T) {
	tests := []struct {
		name    string
		run     *v2.TriggerRun
		wantErr string
	}{
		{
			name:    "trigger without a schedule is rejected",
			run:     newRun("run", v2.TriggerRunSpec{}),
			wantErr: "spec.trigger.cron_schedule is required",
		},
		{
			name:    "interval_schedule is rejected until a runner is registered",
			run:     newRun("run", v2.TriggerRunSpec{Trigger: intervalSpec(time.Hour)}),
			wantErr: "spec.trigger.interval_schedule is not supported yet",
		},
		{
			name:    "batch_rerun is rejected until a runner is registered",
			run:     newRun("run", v2.TriggerRunSpec{Trigger: batchRerunSpec()}),
			wantErr: "spec.trigger.batch_rerun is not supported yet",
		},
		{
			name:    "unparsable cron is rejected",
			run:     newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("every day")}),
			wantErr: `invalid spec.trigger.cron_schedule.cron "every day"`,
		},
		{
			name:    "six-field cron is rejected",
			run:     newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("0 0 9 * * *")}),
			wantErr: "invalid spec.trigger.cron_schedule.cron",
		},
		{
			name: "descriptor cron is accepted",
			run:  newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("@every 1h")}),
		},
		{
			name: "63-character name is accepted",
			run:  newRun(strings.Repeat("a", 63), v2.TriggerRunSpec{Trigger: cronSpec("0 9 * * *")}),
		},
		{
			name:    "64-character name is rejected",
			run:     newRun(strings.Repeat("a", 64), v2.TriggerRunSpec{Trigger: cronSpec("0 9 * * *")}),
			wantErr: "must be a valid label value",
		},
		{
			name: "backfill window with no occurrence is rejected",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 9 * * *"),
				StartTimestamp: ts(jan10.Add(10 * time.Hour)),
				EndTimestamp:   ts(jan10.Add(11 * time.Hour)),
			}),
			wantErr: `contains no occurrence of cron "0 9 * * *"`,
		},
		{
			name: "backfill window with an occurrence is accepted",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 9 * * *"),
				StartTimestamp: ts(jan10.Add(8 * time.Hour)),
				EndTimestamp:   ts(jan10.Add(10 * time.Hour)),
			}),
		},
		{
			name: "backfill window whose start is on the schedule is accepted",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 * * * *"),
				StartTimestamp: ts(jan10),
				EndTimestamp:   ts(jan10.Add(30 * time.Minute)),
			}),
		},
		{
			name: "auto_flip on a backfill window is rejected",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 9 * * *"),
				StartTimestamp: ts(jan10),
				EndTimestamp:   ts(jan11),
				AutoFlip:       true,
			}),
			wantErr: "auto_flip cannot be set on a backfill window",
		},
		{
			name: "auto_flip on a cron schedule is accepted",
			run:  newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("0 9 * * *"), AutoFlip: true}),
		},
		{
			name: "auto_flip on a catch-up window is accepted",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 9 * * *"),
				StartTimestamp: ts(jan10),
				EndTimestamp:   ts(jan11),
				Catchup:        true,
				AutoFlip:       true,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiHook{}.BeforeCreate(context.Background(), &v2.CreateTriggerRunRequest{TriggerRun: tt.run})

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err), "spec violations are client errors")
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateBackfillWindowUnparsableCron(t *testing.T) {
	err := validateBackfillWindow(&v2.TriggerRunSpec{
		Trigger:        cronSpec("every day"),
		StartTimestamp: ts(jan10),
		EndTimestamp:   ts(jan11),
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.ErrorContains(t, err, "failed to parse cron schedule")
}

func TestBeforeCreateDefaultsReferenceNamespaces(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-pipeline", gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	run := newRun("run", v2.TriggerRunSpec{
		Trigger:  cronSpec("0 9 * * *"),
		Pipeline: &api.ResourceIdentifier{Name: "test-pipeline"},
		Revision: &api.ResourceIdentifier{Name: "test-revision", Namespace: "other-ns"},
	})

	require.NoError(t, h.BeforeCreate(context.Background(), &v2.CreateTriggerRunRequest{TriggerRun: run}))
	require.Equal(t, "test-ns", run.Spec.Pipeline.Namespace, "empty pipeline namespace defaults to the TriggerRun's")
	require.Equal(t, "other-ns", run.Spec.Revision.Namespace, "an explicit namespace is kept")
}

func TestBeforeUpdateSpecRules(t *testing.T) {
	h := apiHook{}
	update := func(run *v2.TriggerRun) error {
		return h.BeforeUpdate(context.Background(), &v2.UpdateTriggerRunRequest{TriggerRun: run})
	}

	t.Run("kill action bypasses the structural rules", func(t *testing.T) {
		require.NoError(t, update(newRun("run", v2.TriggerRunSpec{
			Trigger: cronSpec("every day"),
			Action:  v2.TRIGGER_RUN_ACTION_KILL,
		})))
	})

	t.Run("deprecated kill flag bypasses the structural rules", func(t *testing.T) {
		require.NoError(t, update(newRun("run", v2.TriggerRunSpec{
			Trigger: intervalSpec(time.Hour),
			Kill:    true,
		})))
	})

	t.Run("other updates still apply the structural rules", func(t *testing.T) {
		err := update(newRun("run", v2.TriggerRunSpec{
			Trigger: cronSpec("every day"),
			Action:  v2.TRIGGER_RUN_ACTION_PAUSE,
		}))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid spec.trigger.cron_schedule.cron")
	})

	t.Run("creation-only rules are not re-checked", func(t *testing.T) {
		// A 64-character name, an empty backfill window and auto_flip on it are all
		// rejected at creation; an existing object with them must stay updatable.
		run := newRun(strings.Repeat("a", 64), v2.TriggerRunSpec{
			Trigger:        cronSpec("0 9 * * *"),
			StartTimestamp: ts(jan10.Add(10 * time.Hour)),
			EndTimestamp:   ts(jan10.Add(11 * time.Hour)),
			AutoFlip:       true,
			Pipeline:       &api.ResourceIdentifier{Name: "test-pipeline"},
		})
		require.NoError(t, update(run))
		require.Equal(t, "test-ns", run.Spec.Pipeline.Namespace, "defaults are applied on update too")
	})
}
