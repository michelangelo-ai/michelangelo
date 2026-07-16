package triggerrun

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	temporalclientpkg "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/temporalclient"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	temporalClient "go.temporal.io/sdk/client"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// MockRunner is a testify mock implementation for Runner interface.
type MockRunner struct {
	mock.Mock
}

// GetStatus mocks base method.
func (m *MockRunner) GetStatus(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	args := m.Called(ctx, triggerRun)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Error(1)
}

// Kill mocks base method.
func (m *MockRunner) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	args := m.Called(ctx, triggerRun)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Error(1)
}

// Run mocks base method.
func (m *MockRunner) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	args := m.Called(ctx, triggerRun)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Error(1)
}

// Pause mocks base method.
func (m *MockRunner) Pause(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	args := m.Called(ctx, triggerRun)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Error(1)
}

// Resume mocks base method.
func (m *MockRunner) Resume(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	args := m.Called(ctx, triggerRun)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Error(1)
}

// Update mocks base method.
func (m *MockRunner) Update(ctx context.Context, triggerRun *v2pb.TriggerRun, action v2pb.TriggerRunAction) (v2pb.TriggerRunStatus, bool, error) {
	args := m.Called(ctx, triggerRun, action)
	return args.Get(0).(v2pb.TriggerRunStatus), args.Bool(1), args.Error(2)
}

var (
	_namespace  = "test-namespace"
	_triggerRun = v2pb.TriggerRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TriggerRun",
			APIVersion: "api.michelangelo.ai/v2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: _namespace,
			Name:      "test-triggerrun-name",
		},
		Spec: v2pb.TriggerRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: _namespace,
				Name:      "test-pipeline-name",
			},
			Revision: &api.ResourceIdentifier{
				Namespace: _namespace,
				Name:      "test-revision-name",
			},
			Actor: &v2pb.UserInfo{Name: "test-user"},
			Trigger: &v2pb.Trigger{
				TriggerType: &v2pb.Trigger_CronSchedule{
					CronSchedule: &v2pb.CronSchedule{Cron: "0 0 * * *"},
				},
				ParametersMap: map[string]*v2pb.PipelineExecutionParameters{
					"global": {},
				},
			},
		},
		Status: v2pb.TriggerRunStatus{
			State: v2pb.TRIGGER_RUN_STATE_INVALID,
		},
	}
)

func TestReconcile(t *testing.T) {
	tests := []struct {
		name               string
		request            ctrl.Request
		initialObject      v2pb.TriggerRun
		initialStatus      v2pb.TriggerRunStatus
		cronRunnerProvider func() Runner
		expectErr          bool
		expectedErrStr     string
		expectedStatus     v2pb.TriggerRunStatus
		expectedAction     v2pb.TriggerRunAction
		expectRequeue      bool
		isImmutable        bool
	}{
		{
			name:          "first time enable failed",
			request:       ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: _triggerRun,
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_INVALID},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Run", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{}, fmt.Errorf("failed to start workflow"))
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
			expectRequeue:  true,
		},
		{
			name:          "first time enable succeeded",
			request:       ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: _triggerRun,
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_INVALID},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Run", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectRequeue:  true,
		},
		{
			name:    "kill trigger after initial creation",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Kill = true
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_INVALID},
			cronRunnerProvider: func() Runner {
				return &MockRunner{}
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
			expectRequeue:  true,
		},
		{
			name:    "kill an running trigger - succeeded",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Kill = true
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("Kill", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
			expectRequeue:  true,
		},
		{
			name:    "kill an running trigger - failed with invalid state",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Kill = true
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("Kill", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{}, fmt.Errorf("failed to cancel the cadence workflow"))
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{ErrorMessage: "failed to cancel the cadence workflow", State: v2pb.TRIGGER_RUN_STATE_INVALID},
			expectRequeue:  true,
		},
		{
			name:    "kill an running trigger - failed with running state",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Kill = true
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("Kill", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectRequeue:  true,
		},
		{
			name:    "kill an running trigger - failed with failed state",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Kill = true
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("Kill", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
						fmt.Errorf("execution workflow id is empty"))
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED, ErrorMessage: "execution workflow id is empty"},
			expectRequeue:  true,
		},
		{
			name:    "pause action is persisted as consumed after atomic update",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				tr.Spec.Action = v2pb.TRIGGER_RUN_ACTION_PAUSE
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, v2pb.TRIGGER_RUN_ACTION_PAUSE).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_PAUSED}, true, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_PAUSED},
			expectedAction: v2pb.TRIGGER_RUN_ACTION_NO_ACTION,
			expectRequeue:  true,
		},
		{
			name:          "triggerrun in failed status",
			request:       ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: _triggerRun,
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
			cronRunnerProvider: func() Runner {
				return &MockRunner{}
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
			expectRequeue:  false,
			isImmutable:    true,
		},
		{
			name:          "triggerrun in killed status",
			request:       ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: _triggerRun,
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
			cronRunnerProvider: func() Runner {
				return &MockRunner{}
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_KILLED},
			expectRequeue:  false,
			isImmutable:    true,
		},
		{
			name:    "running triggerrun GetStatus - failed",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, fmt.Errorf("failed to GetStatus"))
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING, ErrorMessage: "failed to GetStatus"},
			expectRequeue:  true,
		},
		{
			name:    "running triggerrun GetStatus - succeeded with running status",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectRequeue:  true,
		},
		{
			name:    "running triggerrun Update fails and sets error status",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, fmt.Errorf("update failed"))
				// GetStatus should NOT be called - StateMachine breaks on Update error
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{
				State:        v2pb.TRIGGER_RUN_STATE_RUNNING,
				ErrorMessage: "update failed",
			},
			expectRequeue: true,
		},
		{
			name:    "running triggerrun GetStatus - succeeded with succeeded status",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_SUCCEEDED},
			expectRequeue:  true,
		},
		{
			name:    "running triggerrun GetStatus - failed with failed status",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
			expectRequeue:  true,
		},
		{
			name:    "cadence trigger - running state reconcile success",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
				// Update() skips for Cadence - returns success immediately
				mockRunner.On("Update", mock.Anything, mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, false, nil)
				// GetStatus() succeeds
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			expectRequeue:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			initialObj := test.initialObject.DeepCopy()
			initialObj.Status = test.initialStatus
			initialObjects := []runtime.Object{initialObj}
			params := Params{
				Logger:      zapr.NewLogger(zaptest.NewLogger(t)),
				CronTrigger: test.cronRunnerProvider(),
			}
			reconciler := setUpReconciler(t, initialObjects, params)
			tr := &v2pb.TriggerRun{}
			err := reconciler.Get(ctx, _namespace, test.request.NamespacedName.Name, &metav1.GetOptions{}, tr)
			assert.NoError(t, err, test.name)
			assert.Equal(t, test.initialStatus, tr.Status, test.name)

			// reconcile
			res, err := reconciler.Reconcile(ctx, test.request)
			if test.expectErr {
				assert.Error(t, err, test.name)
				assert.ErrorContains(t, err, test.expectedErrStr, test.name)
				return
			}
			reconciler.Get(ctx, _namespace, test.request.NamespacedName.Name, &metav1.GetOptions{}, tr)
			assert.NoError(t, err, test.name)
			assert.NotNil(t, res, test.name)
			assert.Equal(t, test.expectedStatus, tr.Status)
			assert.Equal(t, test.expectedAction, tr.Spec.Action)
			if test.expectRequeue {
				assert.NotZero(t, res.RequeueAfter)
			} else {
				assert.Zero(t, res.RequeueAfter)
			}
			if test.isImmutable {
				assert.True(t, apiutils.IsImmutable(tr))
			}
		})
	}

}

func TestGetRunner(t *testing.T) {
	tests := []struct {
		name       string
		runnerType string
		triggerRun v2pb.TriggerRun
	}{
		{
			name:       "cron trigger",
			runnerType: "*triggerrun.cronTrigger",
			triggerRun: v2pb.TriggerRun{
				Spec: v2pb.TriggerRunSpec{
					Trigger: &v2pb.Trigger{
						TriggerType: &v2pb.Trigger_CronSchedule{CronSchedule: &v2pb.CronSchedule{Cron: "5 * * * *"}},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initialObj := test.triggerRun.DeepCopy()
			initialObjects := []runtime.Object{initialObj}
			params := Params{
				Logger:      zapr.NewLogger(zaptest.NewLogger(t)),
				CronTrigger: &cronTrigger{},
			}
			reconciler := setUpReconciler(t, initialObjects, params)
			runner := reconciler.getRunner(&test.triggerRun)
			runnerType := reflect.TypeOf(runner).String()
			assert.NotNil(t, runner, test.name)
			assert.Equal(t, test.runnerType, runnerType, test.name)

		})
	}
}

// TestWatchPredicateIgnoresStatusOnlyUpdates locks in the fix for the
// notification-drift retry storm: a status-only write (e.g. Status.ErrorMessage
// from a failed workflow-engine sync, with no Spec/Generation change) must not
// re-trigger an immediate reconcile via the controller's watch, since that
// self-triggered loop is what turned a permanently-failing Temporal call into
// an unbounded-rate retry storm. Spec changes (Generation bump) must still be
// observed immediately.
func TestWatchPredicateIgnoresStatusOnlyUpdates(t *testing.T) {
	base := _triggerRun.DeepCopy()
	base.ObjectMeta.Generation = 5
	base.Status = v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING}

	statusOnlyChange := base.DeepCopy()
	statusOnlyChange.Status.ErrorMessage = "exceeded workflow execution limit for signal events"

	specChange := base.DeepCopy()
	specChange.ObjectMeta.Generation = 6

	pred := predicate.GenerationChangedPredicate{}

	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: statusOnlyChange}),
		"status-only update must not trigger an immediate reconcile")
	assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: base, ObjectNew: specChange}),
		"a generation-bumping spec update must still trigger an immediate reconcile")
}

// TestSignalStormRepro is a manual repro for the STG incident where the
// notification-drift fix (actual_notifications, see cron_trigger.go's
// notifDrifted) caused unconditional retries against Temporal schedules that
// had already exhausted history.maximumSignalsPerExecution, producing
// "exceeded workflow execution limit for signal events" and an 18 calls/sec
// storm for 3 triggers. Skipped by default; not run in CI.
//
// To run against a local Temporal dev server:
//
//	temporal server start-dev --port 17233 --ui-port 18233 \
//	    --dynamic-config-value history.maximumSignalsPerExecution=5 --headless
//	MA_TRIGGER_REPRO=1 go test ./components/triggerrun/... -run TestSignalStormRepro -v -timeout 120s
func TestSignalStormRepro(t *testing.T) {
	if os.Getenv("MA_TRIGGER_REPRO") == "" {
		t.Skip("manual repro only; set MA_TRIGGER_REPRO=1 to run against a local Temporal dev server")
	}

	ctx := context.Background()
	sdkClient, err := temporalClient.Dial(temporalClient.Options{
		HostPort:  "localhost:17233",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("dial temporal: %v", err)
	}
	defer sdkClient.Close()

	tc := &temporalclientpkg.TemporalClient{
		Client:   sdkClient,
		Provider: "temporal",
		Domain:   "default",
	}

	wid := "repro-trigger-1"

	// Step 1: create the schedule exactly the way cronTrigger.Run() does.
	_, err = tc.StartWorkflow(ctx, clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365,
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
		CronSchedule:                    "* * * * *",
	}, "trigger.CronTrigger", "initial-args")
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	t.Logf("created schedule for workflow id %q", wid)

	// Step 2: simulate the reconcile hot-loop. notifDrifted is stuck true forever
	// because Status.ActualNotifications is only persisted on a *successful*
	// UpdateTrigger call (cron_trigger.go:285-287) -- so every iteration below
	// re-sends the same "drifted" update, exactly like a real reconcile storm.
	var (
		calls, failures int
		firstLimitErrAt int
		start           = time.Now()
	)
	deadline := start.Add(30 * time.Second)
	for time.Now().Before(deadline) {
		calls++
		err := tc.UpdateTrigger(ctx, wid, "", nil, []interface{}{"drifted-args"})
		if err != nil {
			failures++
			if strings.Contains(err.Error(), "exceeded workflow execution limit for signal events") && firstLimitErrAt == 0 {
				firstLimitErrAt = calls
				t.Logf("call #%d: reproduced target error: %v", calls, err)
			}
		}
	}
	elapsed := time.Since(start)
	rps := float64(calls) / elapsed.Seconds()

	t.Logf("total calls=%d failures=%d elapsed=%s rps=%.1f firstLimitErrorAtCall=%d",
		calls, failures, elapsed, rps, firstLimitErrAt)

	if firstLimitErrAt == 0 {
		t.Fatalf("did not reproduce 'exceeded workflow execution limit for signal events' within %d calls", calls)
	}
}

func setUpReconciler(t *testing.T, initialObjects []runtime.Object, params Params) Reconciler {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	assert.NoError(t, err)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(initialObjects...).
		WithStatusSubresource(&v2pb.TriggerRun{}).
		Build()
	reconciler := NewReconciler(params)
	reconciler.Handler = apiHandler.NewFakeAPIHandler(k8sClient)
	reconciler.scheme = scheme
	return *reconciler
}
