package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	osscommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func soakSettings(period time.Duration) osscommon.RolloutSettings {
	settings := testSettings
	settings.SoakPeriod = period
	return settings
}

func TestSoakActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		period            time.Duration
		progress          *osscommon.RolloutProgress
		expectedStatus    apipb.ConditionStatus
		expectedMessage   string
		expectedReasonSub string
		expectDone        bool
	}{
		{
			name:           "no soak configured is immediately satisfied",
			period:         0,
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "not started",
			period:            5 * time.Minute,
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonSoakNotStarted,
			expectedReasonSub: "soak of model model-v1 not started in cluster c1",
		},
		{
			name:              "still soaking",
			period:            5 * time.Minute,
			progress:          startedAgo(2*time.Minute, ""),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedMessage:   ReasonSoaking,
			expectedReasonSub: "2m0s of 5m0s elapsed",
		},
		{
			name:           "soak elapsed",
			period:         5 * time.Minute,
			progress:       startedAgo(5*time.Minute, ""),
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectDone:     true,
		},
		{
			name:           "recorded as done",
			period:         5 * time.Minute,
			progress:       &osscommon.RolloutProgress{StartedAt: testNow.Unix(), Done: true},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectDone:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks, target := newRolloutFixture(t, clientErrors{}, true)

			actor := NewSoakActor(mocks.deps(soakSettings(tt.period)), target)
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

func TestSoakActor_Run(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewSoakActor(mocks.deps(soakSettings(5*time.Minute)), target)

	// The first run starts the clock.
	got, err := actor.Run(context.Background(), rolloutDeployment(""), &apipb.Condition{})
	require.NoError(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, got.Status)
	assert.Equal(t, ReasonSoaking, got.Message)
	assert.Contains(t, got.Reason, "5m0s remaining")
	progress, err := osscommon.ReadRolloutProgress(got)
	require.NoError(t, err)
	assert.Equal(t, testNow.Unix(), progress.StartedAt)

	// A later run keeps the original start time and reports what is left.
	got, err = actor.Run(context.Background(), rolloutDeployment(""), conditionWithProgress(t, startedAgo(3*time.Minute, "")))
	require.NoError(t, err)
	assert.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, got.Status)
	assert.Contains(t, got.Reason, "2m0s remaining")
	progress, err = osscommon.ReadRolloutProgress(got)
	require.NoError(t, err)
	assert.Equal(t, testNow.Add(-3*time.Minute).Unix(), progress.StartedAt)
}

func TestSoakActor_GetType(t *testing.T) {
	mocks, target := newRolloutFixture(t, clientErrors{}, true)
	actor := NewSoakActor(mocks.deps(testSettings), target)
	assert.Equal(t, "SoakComplete-"+testCluster, actor.GetType())
}
