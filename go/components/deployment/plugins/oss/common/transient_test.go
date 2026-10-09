package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
)

func TestReportTransient(t *testing.T) {
	now := time.Unix(10_000, 0)
	started := func(ago time.Duration) RolloutProgress {
		return RolloutProgress{StartedAt: now.Add(-ago).Unix()}
	}
	tests := []struct {
		name       string
		progress   RolloutProgress
		timeout    time.Duration
		wantStatus apipb.ConditionStatus
		wantMsg    string
	}{
		{name: "within budget retries", progress: started(time.Minute), timeout: 5 * time.Minute, wantStatus: apipb.CONDITION_STATUS_UNKNOWN, wantMsg: "ClientUnavailable"},
		{name: "budget spent fails", progress: started(10 * time.Minute), timeout: 5 * time.Minute, wantStatus: apipb.CONDITION_STATUS_FALSE, wantMsg: "LoadTimeout"},
		{name: "zero timeout never fails", progress: started(24 * time.Hour), timeout: 0, wantStatus: apipb.CONDITION_STATUS_UNKNOWN, wantMsg: "ClientUnavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := &apipb.Condition{}
			got := ReportTransient(cond, tt.progress, now, tt.timeout, "LoadTimeout", "ClientUnavailable", "dial tcp: i/o timeout")
			assert.Equal(t, tt.wantStatus, got.Status)
			// The condition helpers store the reason token in Message and the text in Reason.
			assert.Equal(t, tt.wantMsg, got.Message)
			assert.Contains(t, got.Reason, "dial tcp: i/o timeout")
			if tt.wantStatus == apipb.CONDITION_STATUS_FALSE {
				assert.Contains(t, got.Reason, "still failing after 5m0s")
			}
			stored, err := ReadRolloutProgress(got)
			require.NoError(t, err)
			if tt.wantStatus == apipb.CONDITION_STATUS_UNKNOWN {
				assert.Equal(t, tt.progress.StartedAt, stored.StartedAt, "the retry keeps counting from the first failure")
			}
		})
	}
}

func TestStartClock(t *testing.T) {
	now := time.Unix(500, 0)
	assert.Equal(t, int64(500), StartClock(RolloutProgress{}, now).StartedAt)
	assert.Equal(t, int64(7), StartClock(RolloutProgress{StartedAt: 7}, now).StartedAt, "an existing start time is kept")
}

func TestProbeFailureTransient(t *testing.T) {
	assert.True(t, (&ProbeFailure{Reason: ReasonClientUnavailable}).Transient())
	assert.False(t, (&ProbeFailure{Reason: ReasonBackendUnavailable, Permanent: true}).Transient())
	assert.False(t, (*ProbeFailure)(nil).Transient())
}
