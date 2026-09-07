//go:build integration

// Integration coverage for cron catch-up against a real Temporal server.
//
// The unit tests assert we call Backfill with the right arguments; they cannot
// prove Temporal actually replays a historical range, because the mock is the
// thing being asked. This exercises the real server:
//
//	temporal server start-dev --port 17233 --namespace default
//	go test -tags integration ./go/base/workflowclient/temporalclient/ -run Catchup -v
package temporalclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	"github.com/stretchr/testify/require"
	temporalEnumsV1 "go.temporal.io/api/enums/v1"
	temporalClient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.uber.org/zap"
)

const (
	integrationHostPort = "127.0.0.1:17233"
	integrationTaskList = "catchup-integration-tq"
	integrationWorkflow = "CatchUpProbeWorkflow"
)

// firedAction is one action the schedule took. logicalTs is what the real
// CronTrigger would use for its DS date string (cron_trigger_workflows.go:121
// does logicalTs := workflow.Now(ctx).UTC()); executionID carries the occurrence
// time Temporal actually assigned, as the workflow-ID suffix.
type firedAction struct {
	logicalTs   time.Time
	executionID string
}

type firedActions struct {
	mu      sync.Mutex
	actions []firedAction
}

func (f *firedActions) add(a firedAction) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, a)
}

func (f *firedActions) snapshot() []firedAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]firedAction(nil), f.actions...)
}

var _fired firedActions

// catchUpProbeWorkflow stands in for trigger.CronTrigger, recording the same clock
// the real workflow derives DS from alongside the identity Temporal gave the action.
func catchUpProbeWorkflow(ctx workflow.Context) error {
	_fired.add(firedAction{
		logicalTs:   workflow.Now(ctx).UTC(),
		executionID: workflow.GetInfo(ctx).WorkflowExecution.ID,
	})
	return nil
}

// occurrenceFromExecutionID extracts the scheduled occurrence time Temporal appends
// to a scheduled workflow's ID, e.g. "my-trigger-2026-09-06T14:55:00Z".
func occurrenceFromExecutionID(t *testing.T, executionID string) time.Time {
	t.Helper()

	idx := strings.LastIndex(executionID, "-2")
	require.Greaterf(t, idx, 0, "no occurrence suffix in execution ID %q", executionID)

	occurrence, err := time.Parse(time.RFC3339, executionID[idx+1:])
	require.NoErrorf(t, err, "parse occurrence suffix of %q", executionID)
	return occurrence.UTC()
}

func newIntegrationClient(t *testing.T) *TemporalClient {
	t.Helper()

	c, err := temporalClient.Dial(temporalClient.Options{
		HostPort:  integrationHostPort,
		Namespace: "default",
	})
	require.NoError(t, err, "dial temporal dev server at %s", integrationHostPort)
	t.Cleanup(c.Close)

	return &TemporalClient{
		Client:   c,
		Logger:   zap.NewNop(),
		Provider: "Temporal",
		Domain:   "default",
	}
}

func startProbeWorker(t *testing.T, c *TemporalClient) {
	t.Helper()

	w := worker.New(c.Client, integrationTaskList, worker.Options{})
	w.RegisterWorkflowWithOptions(catchUpProbeWorkflow, workflow.RegisterOptions{Name: integrationWorkflow})
	require.NoError(t, w.Start())
	t.Cleanup(w.Stop)
}

// TestCatchupBackfillsMissedOccurrences is the real assertion: a cron trigger created
// with a past CatchUpFrom must fire one action per missed tick, through its own
// schedule, with no separate backfill trigger.
func TestCatchupBackfillsMissedOccurrences(t *testing.T) {
	c := newIntegrationClient(t)
	startProbeWorker(t, c)

	_fired = firedActions{}

	// Two hours back on a 5-minute cron is 24 missed ticks.
	catchUpFrom := time.Now().Add(-2 * time.Hour)
	workflowID := fmt.Sprintf("catchup-probe-%d", time.Now().UnixNano())

	exec, err := c.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{
		ID:           workflowID,
		TaskList:     integrationTaskList,
		CronSchedule: "*/5 * * * *",
		CatchUpFrom:  catchUpFrom,
	}, integrationWorkflow)
	require.NoError(t, err)

	handle := c.Client.ScheduleClient().GetHandle(context.Background(), exec.ID)
	t.Cleanup(func() { _ = handle.Delete(context.Background()) })

	// Backfill is asynchronous: the server queues the replayed actions and the
	// worker drains them. Poll rather than sleeping a fixed interval.
	var fired []firedAction
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		fired = _fired.snapshot()
		if len(fired) >= 24 {
			break
		}
		time.Sleep(2 * time.Second)
	}

	require.GreaterOrEqualf(t, len(fired), 24,
		"expected >=24 backfilled actions for 2h of a 5-minute cron, got %d", len(fired))

	sort.Slice(fired, func(i, j int) bool { return fired[i].executionID < fired[j].executionID })
	t.Logf("catch-up window opened at %s", catchUpFrom.UTC().Format(time.RFC3339))

	// Temporal must replay one action per missed tick, each with its own occurrence.
	occurrences := make(map[time.Time]bool, len(fired))
	for _, a := range fired {
		occurrence := occurrenceFromExecutionID(t, a.executionID)
		require.Falsef(t, occurrences[occurrence],
			"duplicate occurrence %s: burst was collapsed, not replayed", occurrence)
		occurrences[occurrence] = true
	}

	// Every occurrence must fall inside the requested window and land on a cron tick,
	// so the replay covers the gap rather than inventing times.
	for occurrence := range occurrences {
		require.Falsef(t, occurrence.Before(catchUpFrom.Add(-5*time.Minute)),
			"occurrence %s predates the catch-up window opening at %s", occurrence, catchUpFrom)
		require.Zerof(t, occurrence.Minute()%5, "occurrence %s is not on a 5-minute cron tick", occurrence)
	}

	// The execution clock is deliberately NOT the occurrence: a backfill replays
	// everything at once, so all 24 actions run within a second of each other. This is
	// why trigger.logicalTimestamp derives DS from the workflow ID rather than
	// workflow.Now - see TestLogicalTimestamp for that mapping. Asserting the gap here
	// keeps the reason for that indirection visible if Temporal ever changes it.
	require.Greaterf(t, fired[0].logicalTs.Sub(occurrenceFromExecutionID(t, fired[0].executionID)),
		time.Hour, "expected backfilled actions to execute long after their occurrence")

	// The contract logicalTimestamp depends on: the occurrence is recoverable from the
	// workflow ID. If Temporal drops this suffix, DS derivation silently falls back to
	// wall clock and every caught-up run collapses onto one date.
	for _, a := range fired {
		require.Regexpf(t, `-\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, a.executionID,
			"execution ID %q lost its occurrence suffix", a.executionID)
	}

	// The schedule must keep firing forward on its normal policy. Backfill overrides
	// overlap per-request only; leaking BUFFER_ALL into steady state would change
	// behavior for every existing trigger.
	desc, err := handle.Describe(context.Background())
	require.NoError(t, err)
	require.Equal(t, temporalEnumsV1.SCHEDULE_OVERLAP_POLICY_SKIP, desc.Schedule.Policy.Overlap,
		"steady-state overlap policy must remain SKIP")
	require.False(t, desc.Schedule.State.Paused, "schedule should be live and firing forward")
}

// TestCatchupNotRequestedFiresNothingImmediately is the counterpart guard: without a
// start time, a freshly created cron trigger must stay quiet until its next real tick.
// This is what regresses if the nil-timestamp guard is dropped, since a nil proto
// timestamp decodes to the Unix epoch and would request a 56-year backfill.
func TestCatchupNotRequestedFiresNothingImmediately(t *testing.T) {
	c := newIntegrationClient(t)
	startProbeWorker(t, c)

	_fired = firedActions{}

	workflowID := fmt.Sprintf("no-catchup-probe-%d", time.Now().UnixNano())

	exec, err := c.StartWorkflow(context.Background(), clientInterface.StartWorkflowOptions{
		ID:           workflowID,
		TaskList:     integrationTaskList,
		CronSchedule: "*/5 * * * *",
	}, integrationWorkflow)
	require.NoError(t, err)

	handle := c.Client.ScheduleClient().GetHandle(context.Background(), exec.ID)
	t.Cleanup(func() { _ = handle.Delete(context.Background()) })

	time.Sleep(15 * time.Second)

	require.Emptyf(t, _fired.snapshot(),
		"schedule without CatchUpFrom must not fire on creation, got %d actions", len(_fired.snapshot()))
}
