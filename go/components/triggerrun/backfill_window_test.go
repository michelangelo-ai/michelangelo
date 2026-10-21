package triggerrun

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackfillRunTimestamps(t *testing.T) {
	day := func(d, h int) time.Time { return time.Date(2024, 1, d, h, 0, 0, 0, time.UTC) }

	tests := []struct {
		name    string
		cron    string
		start   time.Time
		end     time.Time
		want    []time.Time
		wantErr string
	}{
		{
			name:  "every occurrence after start up to and including end",
			cron:  "0 8 * * *",
			start: day(1, 7),
			end:   day(3, 8),
			want:  []time.Time{day(1, 8), day(2, 8), day(3, 8)},
		},
		{
			name:  "start on the schedule is itself an occurrence",
			cron:  "0 8 * * *",
			start: day(1, 8),
			end:   day(2, 7),
			want:  []time.Time{day(1, 8)},
		},
		{
			name:  "window between two occurrences is empty",
			cron:  "0 8 * * *",
			start: day(1, 9),
			end:   day(1, 10),
		},
		{
			name:  "end before start is empty",
			cron:  "0 * * * *",
			start: day(2, 0).Add(30 * time.Minute),
			end:   day(1, 0),
		},
		{
			name:  "start on the schedule counts even when end is before it",
			cron:  "0 * * * *",
			start: day(2, 0),
			end:   day(1, 0),
			want:  []time.Time{day(2, 0)},
		},
		{
			name:    "unparsable cron",
			cron:    "every day",
			start:   day(1, 0),
			end:     day(2, 0),
			wantErr: "failed to parse cron schedule",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BackfillRunTimestamps(tt.start, tt.end, tt.cron)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
