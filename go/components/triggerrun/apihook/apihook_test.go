package apihook

import (
	"context"
	"errors"
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
	"k8s.io/apimachinery/pkg/runtime"

	mlapi "github.com/michelangelo-ai/michelangelo/go/api"
	apimocks "github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2.AddToScheme(scheme))
	return scheme
}

func TestBeforeCreateStampsSourcePipelineLabels(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), "test-ns", "test-pipeline", gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2.Pipeline) error {
			obj.Name = "test-pipeline"
			obj.Namespace = "test-ns"
			obj.Spec = v2.PipelineSpec{
				Type:     v2.PIPELINE_TYPE_TRAIN,
				Manifest: &v2.PipelineManifest{Type: v2.PIPELINE_MANIFEST_TYPE_YAML},
			}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.TriggerRunSpec{
				Trigger:  cronSpec("0 * * * *"),
				Pipeline: &api.ResourceIdentifier{Name: "test-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, "PIPELINE_TYPE_TRAIN",
		request.TriggerRun.GetLabels()[mlapi.SourcePipelineTypeLabelName])
	require.Equal(t, "PIPELINE_MANIFEST_TYPE_YAML",
		request.TriggerRun.GetLabels()[mlapi.SourcePipelineManifestTypeLabelName])
}

func TestBeforeCreatePipelineNotFoundDoesNotFailCreation(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.TriggerRunSpec{
				Trigger:  cronSpec("0 * * * *"),
				Pipeline: &api.ResourceIdentifier{Name: "missing-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, mlapi.UnspecifiedEnvironment, request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreatePipelineLookupErrorDoesNotFailCreation(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("internal error"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.TriggerRunSpec{
				Trigger:  cronSpec("0 * * * *"),
				Pipeline: &api.ResourceIdentifier{Name: "test-pipeline"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, mlapi.UnspecifiedEnvironment, request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreateNoPipelineRefIsNoop(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	// No Get call is expected: BeforeCreate must return early without hitting the API.
	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, mlapi.UnspecifiedEnvironment, request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreateMissingEnvironmentLabelDefaulted(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t), defaultEnv: "development"}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, "development", request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreateMissingEnvironmentLabelDefaultsToUnspecified(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t)}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, mlapi.UnspecifiedEnvironment, request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreateExistingEnvironmentLabelNotOverwritten(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t), defaultEnv: "development"}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "run",
				Namespace: "test-ns",
				Labels:    map[string]string{mlapi.EnvironmentLabel: "production"},
			},
			Spec: v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, "production", request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func TestBeforeCreateRevisionPinnedMissingEnvironmentLabelDefaulted(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI, scheme: testScheme(t), defaultEnv: "development"}
	request := &v2.CreateTriggerRunRequest{
		TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec: v2.TriggerRunSpec{
				Trigger:  cronSpec("0 * * * *"),
				Revision: &api.ResourceIdentifier{Name: "test-revision"},
			},
		},
	}

	require.NoError(t, h.BeforeCreate(context.Background(), request))
	require.Equal(t, "development", request.TriggerRun.GetLabels()[mlapi.EnvironmentLabel])
}

func runInState(name string, state v2.TriggerRunState) v2.TriggerRun {
	return v2.TriggerRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Status:     v2.TriggerRunStatus{State: state},
	}
}

func TestBeforeDeleteOnlyTerminalTriggerRunsCanBeDeleted(t *testing.T) {
	tests := []struct {
		state    v2.TriggerRunState
		rejected bool
	}{
		{v2.TRIGGER_RUN_STATE_INVALID, true},
		{v2.TRIGGER_RUN_STATE_RUNNING, true},
		{v2.TRIGGER_RUN_STATE_PENDING_KILL, true},
		{v2.TRIGGER_RUN_STATE_PAUSED, true},
		{v2.TRIGGER_RUN_STATE_FAILED, false},
		{v2.TRIGGER_RUN_STATE_KILLED, false},
		{v2.TRIGGER_RUN_STATE_SUCCEEDED, false},
	}
	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			mc := gomock.NewController(t)
			defer mc.Finish()

			mockAPI := apimocks.NewMockHandler(mc)
			mockAPI.EXPECT().
				Get(gomock.Any(), "test-ns", "run", gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2.TriggerRun) error {
					*obj = runInState("run", tt.state)
					return nil
				})

			h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
			err := h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "run"})

			if !tt.rejected {
				require.NoError(t, err)
				return
			}
			require.Equal(t, codes.FailedPrecondition, status.Code(err))
			require.ErrorContains(t, err, "test-ns/run is "+tt.state.String())
			require.ErrorContains(t, err, "TRIGGER_RUN_ACTION_KILL")
		})
	}
}

func TestBeforeDeleteNotFoundIsLeftToTheDelete(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "not found"))

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.NoError(t, h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "missing"}))
}

func TestBeforeDeleteLookupErrorIsReturned(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	lookupErr := errors.New("internal error")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(lookupErr)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.ErrorIs(t, h.BeforeDelete(context.Background(), &v2.DeleteTriggerRunRequest{Namespace: "test-ns", Name: "run"}), lookupErr)
}

func TestBeforeDeleteCollectionHonoursListOptions(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), "test-ns", gomock.Any(), gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts *metav1.ListOptions, _ *api.ListOptionsExt, list *v2.TriggerRunList) error {
			require.Equal(t, "team=ml", opts.LabelSelector, "the request's selector scopes the check")
			list.Items = []v2.TriggerRun{
				runInState("done", v2.TRIGGER_RUN_STATE_SUCCEEDED),
				runInState("stopped", v2.TRIGGER_RUN_STATE_KILLED),
			}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.NoError(t, h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{
		Namespace:   "test-ns",
		ListOptions: &metav1.ListOptions{LabelSelector: "team=ml"},
	}))
}

func TestBeforeDeleteCollectionRejectsIfAnySelectedTriggerRunIsLive(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), "test-ns", gomock.Any(), gomock.Nil(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts *metav1.ListOptions, _ *api.ListOptionsExt, list *v2.TriggerRunList) error {
			require.NotNil(t, opts, "absent list options are replaced by an empty set")
			list.Items = []v2.TriggerRun{
				runInState("done", v2.TRIGGER_RUN_STATE_SUCCEEDED),
				runInState("live", v2.TRIGGER_RUN_STATE_RUNNING),
			}
			return nil
		})

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	err := h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{Namespace: "test-ns"})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.ErrorContains(t, err, "test-ns/live is TRIGGER_RUN_STATE_RUNNING")
}

func TestBeforeDeleteCollectionListErrorIsReturned(t *testing.T) {
	mc := gomock.NewController(t)
	defer mc.Finish()

	listErr := errors.New("internal error")
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(listErr)

	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	require.ErrorIs(t, h.BeforeDeleteCollection(context.Background(), &v2.DeleteTriggerRunCollectionRequest{Namespace: "test-ns"}), listErr)
}

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
			name:    "interval_schedule is rejected until a runner is wired",
			run:     newRun("run", v2.TriggerRunSpec{Trigger: intervalSpec(time.Hour)}),
			wantErr: "spec.trigger.interval_schedule is not supported yet",
		},
		{
			name:    "batch_rerun is rejected until a runner is wired",
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
			name: "backfill window on a cron that never fires is rejected",
			run: newRun("run", v2.TriggerRunSpec{
				Trigger:        cronSpec("0 0 30 2 *"),
				StartTimestamp: ts(jan10.AddDate(-20, 0, 0)),
				EndTimestamp:   ts(jan10),
			}),
			wantErr: `contains no occurrence of cron "0 0 30 2 *"`,
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

// updateAgainst returns a BeforeUpdate caller whose stored copy of the object is
// stored, or NotFound when stored is nil.
func updateAgainst(t *testing.T, stored *v2.TriggerRun) func(*v2.TriggerRun) error {
	t.Helper()
	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)
	mockAPI := apimocks.NewMockHandler(mc)
	mockAPI.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2.TriggerRun) error {
			if stored == nil {
				return status.Error(codes.NotFound, "not found")
			}
			*obj = *stored
			return nil
		}).AnyTimes()
	h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
	return func(run *v2.TriggerRun) error {
		return h.BeforeUpdate(context.Background(), &v2.UpdateTriggerRunRequest{TriggerRun: run})
	}
}

func TestBeforeUpdateSpecRules(t *testing.T) {
	storedCron := newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("0 9 * * *")})

	t.Run("kill action bypasses the structural rules", func(t *testing.T) {
		require.NoError(t, updateAgainst(t, storedCron)(newRun("run", v2.TriggerRunSpec{
			Trigger: cronSpec("every day"),
			Action:  v2.TRIGGER_RUN_ACTION_KILL,
		})))
	})

	t.Run("deprecated kill flag bypasses the structural rules", func(t *testing.T) {
		require.NoError(t, updateAgainst(t, storedCron)(newRun("run", v2.TriggerRunSpec{
			Trigger: cronSpec("every day"),
			Kill:    true,
		})))
	})

	t.Run("other updates still apply the structural rules", func(t *testing.T) {
		err := updateAgainst(t, storedCron)(newRun("run", v2.TriggerRunSpec{
			Trigger: cronSpec("every day"),
			Action:  v2.TRIGGER_RUN_ACTION_PAUSE,
		}))
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid spec.trigger.cron_schedule.cron")
	})

	t.Run("an update that keeps the trigger's shape is not held to the window rules", func(t *testing.T) {
		// A 64-character name, an empty backfill window and auto_flip on it are all
		// rejected at creation; an existing object with them must stay editable.
		spec := v2.TriggerRunSpec{
			Trigger:        cronSpec("0 9 * * *"),
			StartTimestamp: ts(jan10.Add(10 * time.Hour)),
			EndTimestamp:   ts(jan10.Add(11 * time.Hour)),
			AutoFlip:       true,
			Pipeline:       &api.ResourceIdentifier{Name: "test-pipeline"},
		}
		stored := newRun(strings.Repeat("a", 64), spec)
		run := newRun(strings.Repeat("a", 64), spec)
		run.Spec.Action = v2.TRIGGER_RUN_ACTION_PAUSE
		require.NoError(t, updateAgainst(t, stored)(run))
	})

	t.Run("an object that no longer exists is left to the update", func(t *testing.T) {
		require.NoError(t, updateAgainst(t, nil)(newRun("run", v2.TriggerRunSpec{
			Trigger:        cronSpec("0 9 * * *"),
			StartTimestamp: ts(jan10),
			EndTimestamp:   ts(jan11),
			AutoFlip:       true,
		})))
	})

	t.Run("a lookup error is returned", func(t *testing.T) {
		mc := gomock.NewController(t)
		defer mc.Finish()
		lookupErr := errors.New("internal error")
		mockAPI := apimocks.NewMockHandler(mc)
		mockAPI.EXPECT().
			Get(gomock.Any(), "test-ns", "run", gomock.Any(), gomock.Any()).
			Return(lookupErr)
		h := apiHook{logger: zaptest.NewLogger(t), apiHandler: mockAPI}
		err := h.BeforeUpdate(context.Background(), &v2.UpdateTriggerRunRequest{TriggerRun: storedCron})
		require.ErrorIs(t, err, lookupErr)
	})
}

// An update that changes the trigger's shape (type, window, catchup, auto_flip
// or cron) must satisfy the window rules, or UpdateTriggerRun would be a way
// around them: a cron trigger with no window passes every create check, and a
// later update could turn it into a future-dated backfill.
func TestBeforeUpdateShapeChangeReappliesWindowRules(t *testing.T) {
	now := time.Now()
	cronNoWindow := func() *v2.TriggerRun {
		return newRun("run", v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *")})
	}
	pastBackfill := func() *v2.TriggerRun {
		return newRun("run", v2.TriggerRunSpec{
			Trigger:        cronSpec("0 * * * *"),
			StartTimestamp: ts(jan10),
			EndTimestamp:   ts(jan11),
		})
	}
	futureWindow := func(r *v2.TriggerRun) {
		r.Spec.StartTimestamp = ts(now.Add(-time.Hour))
		r.Spec.EndTimestamp = ts(now.Add(24 * time.Hour))
	}

	tests := []struct {
		name    string
		stored  *v2.TriggerRun
		update  func(*v2.TriggerRun)
		wantErr string
	}{
		{
			name:    "adding a future-dated window to a cron trigger is rejected",
			stored:  cronNoWindow(),
			update:  futureWindow,
			wantErr: "must lie entirely in the past",
		},
		{
			name:   "adding a window with no cron occurrence is rejected",
			stored: cronNoWindow(),
			update: func(r *v2.TriggerRun) {
				r.Spec.Trigger = cronSpec("0 0 29 2 *")
				r.Spec.StartTimestamp, r.Spec.EndTimestamp = ts(jan10), ts(jan11)
			},
			wantErr: "contains no occurrence of cron",
		},
		{
			name:   "adding a past window with occurrences is accepted",
			stored: cronNoWindow(),
			update: func(r *v2.TriggerRun) { r.Spec.StartTimestamp, r.Spec.EndTimestamp = ts(jan10), ts(jan11) },
		},
		{
			name:   "adding a future window with catchup is accepted",
			stored: cronNoWindow(),
			update: func(r *v2.TriggerRun) { futureWindow(r); r.Spec.Catchup = true },
		},
		{
			name:    "setting auto_flip on an existing backfill is rejected",
			stored:  pastBackfill(),
			update:  func(r *v2.TriggerRun) { r.Spec.AutoFlip = true },
			wantErr: "auto_flip cannot be set on a backfill window",
		},
		{
			name:    "moving a backfill window into the future is rejected",
			stored:  pastBackfill(),
			update:  func(r *v2.TriggerRun) { r.Spec.EndTimestamp = ts(now.Add(24 * time.Hour)) },
			wantErr: "must lie entirely in the past",
		},
		{
			name:    "changing the cron of a backfill to one with no occurrence is rejected",
			stored:  pastBackfill(),
			update:  func(r *v2.TriggerRun) { r.Spec.Trigger = cronSpec("0 0 29 2 *") },
			wantErr: "contains no occurrence of cron",
		},
		{
			name:   "a kill that also changes the shape is accepted",
			stored: cronNoWindow(),
			update: func(r *v2.TriggerRun) { futureWindow(r); r.Spec.Action = v2.TRIGGER_RUN_ACTION_KILL },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := newRun(tt.stored.Name, tt.stored.Spec)
			tt.update(run)
			err := updateAgainst(t, tt.stored)(run)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

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

// A timestamp that cannot be converted is a client error on either side of the window,
// and the error names the field.
func TestScheduleWindowInvalidTimestamps(t *testing.T) {
	invalid := &pbtypes.Timestamp{Seconds: 0, Nanos: 1_000_000_000} // nanos out of range

	t.Run("start_timestamp", func(t *testing.T) {
		err := apiHook{}.BeforeCreate(context.Background(), &v2.CreateTriggerRunRequest{TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *"), StartTimestamp: invalid},
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid start_timestamp")
	})

	t.Run("end_timestamp", func(t *testing.T) {
		err := apiHook{}.BeforeCreate(context.Background(), &v2.CreateTriggerRunRequest{TriggerRun: &v2.TriggerRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "test-ns"},
			Spec:       v2.TriggerRunSpec{Trigger: cronSpec("0 * * * *"), EndTimestamp: invalid},
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		require.ErrorContains(t, err, "invalid end_timestamp")
	})

	t.Run("past-window rule reports the conversion error too", func(t *testing.T) {
		err := validateBackfillWindowIsPast(&v2.TriggerRunSpec{StartTimestamp: invalid}, time.Now())
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// An update must be able to reach an object that already exists, however the clock has
// moved since it was created: the past-window rule is creation-only, while the
// clock-independent rules still apply.
func TestBeforeUpdateScheduleWindowRules(t *testing.T) {
	now := time.Now()
	update := updateAgainst(t, nil)

	t.Run("kill on a legacy backfill whose end is in the future is accepted", func(t *testing.T) {
		require.NoError(t, update(newRun("run", v2.TriggerRunSpec{
			Trigger:        cronSpec("0 * * * *"),
			StartTimestamp: ts(now.Add(-24 * time.Hour)),
			EndTimestamp:   ts(now.Add(24 * time.Hour)),
			Action:         v2.TRIGGER_RUN_ACTION_KILL,
		})))
	})

	t.Run("end before start is still rejected", func(t *testing.T) {
		err := update(newRun("run", v2.TriggerRunSpec{
			Trigger:        cronSpec("0 * * * *"),
			StartTimestamp: ts(now),
			EndTimestamp:   ts(now.Add(-time.Hour)),
		}))
		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
