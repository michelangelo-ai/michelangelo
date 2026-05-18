package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

// oldModelName is the previously-deployed revision used by cleanup tests. The "happy" path
// is when this model is still loaded and needs to be unloaded; the short-circuit paths set
// CurrentRevision to nil or to the desired revision.
const oldModelName = "model-v0"

func TestModelCleanupActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		registerBackend   bool
		currentRevision   string // pass empty to set Status.CurrentRevision = nil
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:            "no cleanup needed - no current revision",
			currentRevision: "",
			registerBackend: true,
			setupMocks:      func(*rolloutMocks) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:            "no cleanup needed - current equals desired",
			currentRevision: testModelName,
			registerBackend: true,
			setupMocks:      func(*rolloutMocks) {},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "GetClient errors",
			currentRevision:   oldModelName,
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			registerBackend:   true,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name:              "GetHTTPClient errors",
			currentRevision:   oldModelName,
			clientErrs:        clientErrors{getHTTPClient: errors.New("dial timeout")},
			registerBackend:   true,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "dial timeout",
		},
		{
			name:              "backend not in registry",
			currentRevision:   oldModelName,
			registerBackend:   false,
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "backend not found",
		},
		{
			name:            "CheckModelStatus errors",
			currentRevision: oldModelName,
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, oldModelName).Return(false, errors.New("api error"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "api error",
		},
		{
			name:            "old model still loaded",
			currentRevision: oldModelName,
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, oldModelName).Return(true, nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "model model-v0 still loaded in cluster c1",
		},
		{
			name:            "old model unloaded",
			currentRevision: oldModelName,
			registerBackend: true,
			setupMocks: func(m *rolloutMocks) {
				m.backend.EXPECT().CheckModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, oldModelName).Return(false, nil)
			},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, tt.registerBackend)
			tt.setupMocks(mocks)

			actor := NewModelCleanupActor(mocks.factory, mocks.backendRegistry, mocks.modelConfigProvider, zap.NewNop(), target)
			got, err := actor.Retrieve(context.Background(), rolloutDeployment(tt.currentRevision), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestModelCleanupActor_Run(t *testing.T) {
	tests := []struct {
		name              string
		clientErrs        clientErrors
		currentRevision   string
		setupMocks        func(*rolloutMocks)
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string
	}{
		{
			name:            "no cleanup needed",
			currentRevision: testModelName,
			setupMocks:      func(*rolloutMocks) {}, // no factory or provider call expected
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "GetClient errors",
			currentRevision:   oldModelName,
			clientErrs:        clientErrors{getClient: errors.New("auth refused")},
			setupMocks:        func(*rolloutMocks) {},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "auth refused",
		},
		{
			name:            "RemoveModelFromConfig errors",
			currentRevision: oldModelName,
			setupMocks: func(m *rolloutMocks) {
				m.modelConfigProvider.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, oldModelName).Return(errors.New("apply failed"))
			},
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "apply failed",
		},
		{
			name:            "happy path",
			currentRevision: oldModelName,
			setupMocks: func(m *rolloutMocks) {
				m.modelConfigProvider.EXPECT().RemoveModelFromConfig(gomock.Any(), gomock.Any(), gomock.Any(),
					testISName, testNamespace, oldModelName).Return(nil)
			},
			expectedStatus:    apipb.CONDITION_STATUS_UNKNOWN,
			expectedReasonSub: "model model-v0 unloading from cluster c1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, tt.clientErrs, true)
			tt.setupMocks(mocks)

			actor := NewModelCleanupActor(mocks.factory, mocks.backendRegistry, mocks.modelConfigProvider, zap.NewNop(), target)
			got, err := actor.Run(context.Background(), rolloutDeployment(tt.currentRevision), &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestModelCleanupActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewModelCleanupActor(mocks.factory, mocks.backendRegistry, mocks.modelConfigProvider, zap.NewNop(), target)
	assert.Equal(t, "ModelCleanupComplete-"+testCluster, actor.GetType())
}
