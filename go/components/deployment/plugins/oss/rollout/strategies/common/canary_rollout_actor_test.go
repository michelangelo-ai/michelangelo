package common

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestCanaryRolloutActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		progress          *osscommon.RolloutProgress
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectDone        bool
	}{
		{
			name:           "short-circuit once the canary is recorded as done",
			progress:       &osscommon.RolloutProgress{StartedAt: testNow.Unix(), Replica: "pod-a", Done: true},
			setupMocks:     func(*rolloutMocks) {},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectDone:     true,
		},
		{
			name:              "not started yet means no probe",
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryNotStarted,
			expectedReasonSub: "canary of model model-v1 not started in cluster c1",
		},
		{
			name:              "GetClient errors",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			progress:          startedAgo(time.Minute, "pod-a"),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonClientUnavailable,
			expectedReasonSub: "auth refused",
		},
		{
			name:     "GetModelStatus errors",
			progress: startedAgo(time.Minute, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, nil, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   osscommon.ReasonModelStatusCheckFailed,
			expectedReasonSub: "api error",
		},
		{
			name:     "canary replica gone is retried",
			progress: startedAgo(time.Minute, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryReplicaMissing,
			expectedReasonSub: `canary replica "pod-a" for model model-v1 is not running in cluster c1`,
		},
		{
			name:     "canary replica gone past the budget is terminal",
			progress: startedAgo(testSettings.ModelLoadTimeout+time.Second, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, loadingReplica("pod-b")), nil)
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonCanaryLoadTimeout,
		},
		{
			name:     "canary load failed is terminal",
			progress: startedAgo(time.Minute, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, failedReplica("pod-a", "unsupported backend"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryLoadFailed,
			expectedReasonSub: "model model-v1 failed to load on canary replica pod-a in cluster c1: unsupported backend",
		},
		{
			name:     "canary still loading keeps waiting",
			progress: startedAgo(time.Minute, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, loadingReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryLoading,
			expectedReasonSub: "model model-v1 loading on canary replica pod-a in cluster c1",
		},
		{
			name:     "canary loading past the budget is terminal",
			progress: startedAgo(testSettings.ModelLoadTimeout+time.Second, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, loadingReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryLoadTimeout,
			expectedReasonSub: "within 10m0s",
		},
		{
			// Only the canary replica has to be ready; the others have not been asked to load.
			name:     "canary ready",
			progress: startedAgo(time.Minute, "pod-a"),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, readyReplica("pod-a"), loadingReplica("pod-b")), nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectDone:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)

			actor := NewCanaryRolloutActor(mocks.deps(testSettings), target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(""), conditionWithProgress(t, tt.progress))

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			progress, err := osscommon.ReadRolloutProgress(got)
			require.NoError(t, err)
			assert.Equal(t, tt.expectDone, progress.Done)
		})
	}
}

func TestCanaryRolloutActor_Run(t *testing.T) {
	canaryEntry := func(pod string) modelconfig.ModelConfigEntry {
		return modelconfig.ModelConfigEntry{
			Name:           testModelName,
			StoragePath:    testModelStoragePath,
			DeploymentName: testDeploymentName,
			Phase:          modelconfig.ModelPhaseCanary,
			CanaryPod:      pod,
		}
	}
	expectCanaryEntry := func(m *rolloutMocks, pod string) {
		m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *zap.Logger, _ client.Client, _, _ string, entry modelconfig.ModelConfigEntry) error {
				assert.Equal(t, canaryEntry(pod), entry)
				return nil
			})
	}

	tests := []struct {
		name              string
		clientErrs        clientErrors
		condition         *apipb.Condition
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectedReplica   string
		expectedStartedAt int64
	}{
		{
			name:            "terminal failure from Retrieve passes through untouched",
			condition:       terminalCondition(ReasonCanaryLoadFailed),
			setupMocks:      func(*rolloutMocks) {},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonCanaryLoadFailed,
		},
		{
			name:              "GetClient errors retry and start the clock",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   osscommon.ReasonClientUnavailable,
			expectedReasonSub: "auth refused",
			expectedStartedAt: testNow.Unix(),
		},
		{
			name:              "GetClient errors keep counting from the first failure",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			condition:         conditionWithProgress(t, startedAgo(2*time.Minute, "")),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedStartedAt: testNow.Add(-2 * time.Minute).Unix(),
		},
		{
			name:              "GetClient errors fail once the load budget is spent",
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			condition:         conditionWithProgress(t, startedAgo(11*time.Minute, "")),
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryLoadTimeout,
			expectedReasonSub: "auth refused",
		},
		{
			name: "a transient probe failure retries",
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, nil, errors.New("connection reset"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   osscommon.ReasonModelStatusCheckFailed,
			expectedStartedAt: testNow.Unix(),
		},
		{
			name:      "a transient probe failure fails once the load budget is spent",
			condition: conditionWithProgress(t, startedAgo(11*time.Minute, "")),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, nil, errors.New("connection reset"))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: ReasonCanaryLoadTimeout,
		},
		{
			name: "a denied pod proxy fails at once",
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, nil, fmt.Errorf("probe: %w", backends.ErrProxyDenied))
			},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: osscommon.ReasonModelStatusCheckFailed,
		},
		{
			name: "picks the first running replica by name",
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(3, pendingReplica("pod-a"), loadingReplica("pod-b"), loadingReplica("pod-c")), nil)
				expectCanaryEntry(m, "pod-b")
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonCanaryLoading,
			expectedReasonSub: "model model-v1 loading on canary replica pod-b in cluster c1",
			expectedReplica:   "pod-b",
			expectedStartedAt: testNow.Unix(),
		},
		{
			name:      "keeps the replica it already picked",
			condition: conditionWithProgress(t, startedAgo(2*time.Minute, "pod-c")),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(3, loadingReplica("pod-a"), loadingReplica("pod-b"), loadingReplica("pod-c")), nil)
				expectCanaryEntry(m, "pod-c")
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedReplica:   "pod-c",
			expectedStartedAt: testNow.Add(-2 * time.Minute).Unix(),
		},
		{
			name:      "re-picks when the chosen replica is gone",
			condition: conditionWithProgress(t, startedAgo(2*time.Minute, "pod-z")),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, loadingReplica("pod-a"), loadingReplica("pod-b")), nil)
				expectCanaryEntry(m, "pod-a")
			},
			expectedStatus:  apipb.CONDITION_STATUS_UNKNOWN,
			expectedReplica: "pod-a",
		},
		{
			name: "no running replica yet starts the clock without writing an entry",
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(2, pendingReplica("pod-a")), nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   ReasonCanaryLoading,
			expectedReasonSub: "waiting for a running replica in cluster c1",
			expectedStartedAt: testNow.Unix(),
		},
		{
			name: "AddModelToConfig errors retry",
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(1, loadingReplica("pod-a")), nil)
				m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, gomock.Any()).
					Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedMessage:   "AddModelToConfigFailed",
			expectedReasonSub: "apply failed",
			expectedStartedAt: testNow.Unix(),
		},
		{
			name:      "AddModelToConfig errors fail once the load budget is spent",
			condition: conditionWithProgress(t, startedAgo(11*time.Minute, "")),
			setupMocks: func(m *rolloutMocks) {
				m.expectModelStatus(testModelName, statusOf(1, loadingReplica("pod-a")), nil)
				m.modelConfigProvider.EXPECT().AddModelToConfig(gomock.Any(), gomock.Any(), gomock.Any(), testISName, testNamespace, gomock.Any()).
					Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonCanaryLoadTimeout,
			expectedReasonSub: "apply failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)
			condition := tt.condition
			if condition == nil {
				condition = &apipb.Condition{}
			}

			actor := NewCanaryRolloutActor(mocks.deps(testSettings), target)
			got, err := actor.Run(context.Background(), rolloutDeployment(""), condition)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, got.Message)
			}
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			progress, err := osscommon.ReadRolloutProgress(got)
			require.NoError(t, err)
			if tt.expectedReplica != "" {
				assert.Equal(t, tt.expectedReplica, progress.Replica)
			}
			if tt.expectedStartedAt != 0 {
				assert.Equal(t, tt.expectedStartedAt, progress.StartedAt)
			}
		})
	}
}

func TestCanaryRolloutActor_ModelResolutionFailures(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	mocks.controlPlane = newControlPlaneClient(t) // no Model CR

	actor := NewCanaryRolloutActor(mocks.deps(testSettings), target)
	got, err := actor.Run(context.Background(), rolloutDeployment(""), &apipb.Condition{})

	require.NoError(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, got.Status)
	assert.Contains(t, got.Reason, "model default/model-v1 not found")
}

func TestCanaryRolloutActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewCanaryRolloutActor(mocks.deps(testSettings), target)
	assert.Equal(t, "CanaryRolloutComplete-"+testCluster, actor.GetType())
}
