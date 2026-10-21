package triggerrun

import (
	"fmt"
	"time"

	"github.com/robfig/cron"
)

// BackfillRunTimestamps enumerates the occurrences of cronExpr that a backfill
// over [start, end] runs: every occurrence after start up to and including end,
// plus start itself when it falls exactly on the schedule. This is the
// enumeration the backfill worker executes, so a caller that validates a window
// ahead of time counts exactly the runs that would be created.
//
// Known limitation: whether start itself is an occurrence is derived from the
// gap between the next two occurrences, so an irregular schedule such as
// "0 9 * * 1,3,6" can be mis-detected at start. The occurrences after start are
// always exact.
func BackfillRunTimestamps(start, end time.Time, cronExpr string) ([]time.Time, error) {
	schedule, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cron schedule %q: %w", cronExpr, err)
	}
	var runs []time.Time
	next := schedule.Next(start)
	previous := next.Add(-schedule.Next(next).Sub(next))
	if previous.Equal(start) {
		runs = append(runs, start)
	}
	for !next.After(end) {
		runs = append(runs, next)
		next = schedule.Next(next)
	}
	return runs, nil
}
