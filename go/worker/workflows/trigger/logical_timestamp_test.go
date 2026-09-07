package trigger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestResolveLogicalTimestamp covers the mapping from a scheduled action's workflow ID to
// the timestamp DS is derived from. The catch-up case is the one that matters: a backfill
// replays every missed occurrence at once, so deriving DS from the execution clock would
// stamp all of them with the same date.
func TestResolveLogicalTimestamp(t *testing.T) {
	// Well after every occurrence below, standing in for the moment a backfill drains.
	executedAt := time.Date(2026, 9, 6, 16, 25, 3, 0, time.UTC)

	tests := []struct {
		name        string
		executionID string
		want        time.Time
	}{
		{
			name:        "backfilled occurrence is used instead of the execution clock",
			executionID: "my-trigger-2026-09-04T02:00:00Z",
			want:        time.Date(2026, 9, 4, 2, 0, 0, 0, time.UTC),
		},
		{
			name:        "occurrence wins even when the trigger name itself contains dashes",
			executionID: "daily-training-run-2026-09-05T14:55:00Z",
			want:        time.Date(2026, 9, 5, 14, 55, 0, 0, time.UTC),
		},
		{
			name:        "numeric trigger name is not mistaken for an occurrence",
			executionID: "catchup-probe-1788711694188693000-2026-09-06T15:30:00Z",
			want:        time.Date(2026, 9, 6, 15, 30, 0, 0, time.UTC),
		},
		{
			name:        "non-UTC offset is normalized to UTC",
			executionID: "my-trigger-2026-09-06T10:00:00-04:00",
			want:        time.Date(2026, 9, 6, 14, 0, 0, 0, time.UTC),
		},
		{
			name:        "fractional seconds are accepted",
			executionID: "my-trigger-2026-09-06T14:55:00.123Z",
			want:        time.Date(2026, 9, 6, 14, 55, 0, 123000000, time.UTC),
		},
		{
			// Cadence has no schedules, and manually started runs carry no suffix.
			// These must keep the pre-existing behavior exactly.
			name:        "unsuffixed ID falls back to the execution clock",
			executionID: "my-trigger",
			want:        executedAt,
		},
		{
			name:        "trailing date without a time is not an occurrence",
			executionID: "my-trigger-2026-09-06",
			want:        executedAt,
		},
		{
			name:        "occurrence must be at the end, not embedded mid-ID",
			executionID: "my-trigger-2026-09-06T14:55:00Z-retry",
			want:        executedAt,
		},
		{
			name:        "calendar-invalid date falls back rather than rolling over",
			executionID: "my-trigger-2026-02-30T14:55:00Z",
			want:        executedAt,
		},
		{
			name:        "empty ID falls back",
			executionID: "",
			want:        executedAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLogicalTimestamp(tt.executionID, executedAt)
			require.Truef(t, tt.want.Equal(got), "want %s, got %s", tt.want, got)
			require.Equal(t, time.UTC, got.Location(), "logical timestamp must be UTC, DS formats from it")
		})
	}
}

// TestResolveLogicalTimestampGivesDistinctDatesAcrossABackfill is the regression guard for
// the defect this fix addresses: a multi-day catch-up must yield one DS per day, not the
// same DS repeated once per replayed occurrence.
func TestResolveLogicalTimestampGivesDistinctDatesAcrossABackfill(t *testing.T) {
	// A daily 02:00 cron caught up over three missed days, all draining at once.
	executedAt := time.Date(2026, 9, 6, 16, 25, 3, 0, time.UTC)
	executionIDs := []string{
		"daily-train-2026-09-03T02:00:00Z",
		"daily-train-2026-09-04T02:00:00Z",
		"daily-train-2026-09-05T02:00:00Z",
	}

	seen := make(map[string]bool, len(executionIDs))
	for _, id := range executionIDs {
		seen[resolveLogicalTimestamp(id, executedAt).Format("2006-01-02")] = true
	}

	require.Equal(t, map[string]bool{
		"2026-09-03": true,
		"2026-09-04": true,
		"2026-09-05": true,
	}, seen, "each caught-up run must carry the date it stands for")
}
