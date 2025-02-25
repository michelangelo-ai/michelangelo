package storage

import (
	"context"
	"testing"

	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"

	"go.uber.org/cadence/activity"
	"go.uber.org/cadence/workflow"

	"go.uber.org/cadence/worker"
)

// dummyStorage is a simple implementation of the Storage interface for testing.
type dummyStorage struct {
	proto string
}

func (d *dummyStorage) Read(ctx context.Context, path string) (any, error) {
	return nil, nil
}

func (d *dummyStorage) Protocol() string {
	return d.proto
}

// dummyWorker is a mock worker that records activities registered with it.
type dummyWorker struct {
	registeredActivities []interface{}
}

func (w2 *dummyWorker) RegisterWorkflow(w interface{}) {
	panic("implement me")
}

func (w2 *dummyWorker) RegisterWorkflowWithOptions(w interface{}, options workflow.RegisterOptions) {
	panic("implement me")
}

func (w2 *dummyWorker) RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions) {
	panic("implement me")
}

func (w2 *dummyWorker) Start() error {
	panic("implement me")
}

func (w2 *dummyWorker) Run() error {
	panic("implement me")
}

func (w2 *dummyWorker) Stop() {
	panic("implement me")
}

func (w *dummyWorker) RegisterActivity(activity interface{}) {
	w.registeredActivities = append(w.registeredActivities, activity)
}

// TestRegister verifies that the register function maps Storage implementations by protocol
// and registers the resulting activities with each Cadence worker.
func TestRegister(t *testing.T) {
	// Create dummy Storage implementations with distinct protocols.
	storages := []intf.Storage{
		&dummyStorage{proto: "s1"},
		&dummyStorage{proto: "s2"},
	}

	// Create two dummy workers.
	worker1 := &dummyWorker{}
	worker2 := &dummyWorker{}
	workers := []worker.Worker{worker1, worker2}

	// Call the register function.
	register(workers, storages)

	// Verify that each worker received exactly one registered activity.
	for i, w := range []*dummyWorker{worker1, worker2} {
		if len(w.registeredActivities) != 1 {
			t.Errorf("worker %d: expected 1 registered activity, got %d", i, len(w.registeredActivities))
			continue
		}

		// Assert that the registered activity is of type *activities.
		act, ok := w.registeredActivities[0].(*activities)
		if !ok {
			t.Errorf("worker %d: registered activity is not of type *activities", i)
			continue
		}

		// Check that the impls map has the expected protocols.
		if len(act.impls) != 2 {
			t.Errorf("worker %d: expected 2 storage implementations in activities.impls, got %d", i, len(act.impls))
		}
		if _, ok := act.impls["s1"]; !ok {
			t.Errorf("worker %d: activities.impls does not contain key 's1'", i)
		}
		if _, ok := act.impls["s2"]; !ok {
			t.Errorf("worker %d: activities.impls does not contain key 's2'", i)
		}
	}
}
