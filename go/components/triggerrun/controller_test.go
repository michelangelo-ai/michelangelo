package triggerrun

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/zapr"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	api "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			name:    "running triggerrun GetStatus - succeeded with succeeded status",
			request: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: _namespace, Name: _triggerRun.Name}},
			initialObject: func() v2pb.TriggerRun {
				tr := _triggerRun.DeepCopy()
				return *tr
			}(),
			initialStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_RUNNING},
			cronRunnerProvider: func() Runner {
				mockRunner := &MockRunner{}
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
				mockRunner.On("GetStatus", mock.Anything, mock.Anything).
					Return(v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED}, nil)
				return mockRunner
			},
			expectErr:      false,
			expectedErrStr: "",
			expectedStatus: v2pb.TriggerRunStatus{State: v2pb.TRIGGER_RUN_STATE_FAILED},
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
	return *reconciler
}
