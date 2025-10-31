package storage

import (
	"context"
	"testing"

	"github.com/cadence-workflow/starlark-worker/worker"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// dummyStorage is a simple implementation of the Storage interface for testing.
type dummyStorage struct {
	scheme string
}

var _ worker.Worker = (*dummyWorker)(nil)

func (d *dummyStorage) Get(ctx context.Context, uri string) ([]byte, error) {
	return nil, nil
}

func (d *dummyStorage) Scheme() string {
	return d.scheme
}

// dummyWorker is a mock worker that records activities registered with it.
type dummyWorker struct {
	registeredActivities []interface{}
}

func (w2 *dummyWorker) RegisterWorkflow(w interface{}, name string) {
	panic("implement me")
}

func (w2 *dummyWorker) RegisterWorkflowWithOptions(w interface{}, options worker.RegisterWorkflowOptions) {
	panic("implement me")
}

func (w2 *dummyWorker) RegisterActivityWithOptions(a interface{}, options worker.RegisterActivityOptions) {
	panic("implement me")
}

func (w2 *dummyWorker) Start() error {
	panic("implement me")
}

func (w2 *dummyWorker) Run(interruptCh <-chan interface{}) error {
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

	// Create two dummy workers.
	worker1 := &dummyWorker{}
	worker2 := &dummyWorker{}
	workers := []worker.Worker{worker1, worker2}

	blobStore := blobstore.BlobStore{}
	blobStore.RegisterClient(&dummyStorage{scheme: "s1"})
	blobStore.RegisterClient(&dummyStorage{scheme: "s2"})
	logger := zap.NewNop()
	params := storagesIn{
		Workers:   workers,
		BlobStore: &blobStore,
		Logger:    logger,
	}
	// Call the register function.
	register(params)

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

		// Check that the blobstore is properly configured
		if act.blobStore == nil {
			t.Errorf("worker %d: expected blobstore in activities.blobStore, got nil", i)
		}
	}
}

// mockBlobStoreClient is a mock client for testing
type mockBlobStoreClient struct {
	scheme      string
	providerKey string
	readFn      func(ctx context.Context, blobURI string) ([]byte, error)
}

func (m *mockBlobStoreClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	if m.readFn != nil {
		return m.readFn(ctx, blobURI)
	}
	return []byte(`{"test": "data"}`), nil
}

func (m *mockBlobStoreClient) Scheme() string {
	return m.scheme
}

func (m *mockBlobStoreClient) ProviderKey() string {
	return m.providerKey
}

// mockLegacyClient is a mock client that only implements scheme-based interface
type mockLegacyClient struct {
	scheme string
	readFn func(ctx context.Context, blobURI string) ([]byte, error)
}

func (m *mockLegacyClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	if m.readFn != nil {
		return m.readFn(ctx, blobURI)
	}
	return []byte(`{"test": "legacy"}`), nil
}

func (m *mockLegacyClient) Scheme() string {
	return m.scheme
}

