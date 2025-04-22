package workflowfx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	tempclient "go.temporal.io/sdk/client"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/zap"
)

// --- Mocks for Temporal ---

type MockTemporalClient struct {
	mock.Mock
	tempclient.Client
}

func (m *MockTemporalClient) Close() {
	m.Called()
}

type MockTemporalClientFactory struct {
	mock.Mock
}

func (m *MockTemporalClientFactory) NewTemporalClient(opts tempclient.Options) (tempclient.Client, error) {
	return tempclient.NewLazyClient(opts)
}

// --- Mocks for Cadence ---

type MockCadenceClient struct {
	mock.Mock
	workflowserviceclient.Interface
}

type MockCadenceClientFactory struct {
	mock.Mock
}

func (m *MockCadenceClientFactory) NewCadenceClient(conf Config) (workflowserviceclient.Interface, error) {
	args := m.Called(conf)
	return args.Get(0).(workflowserviceclient.Interface), args.Error(1)
}

// --- Tests ---

func TestNewTemporalWorker(t *testing.T) {
	mockClient := &MockTemporalClient{}

	mockFactory := &MockTemporalClientFactory{}

	conf := Config{
		Provider: "temporal",
		Host:     "localhost:7233",
		Client: ClientConfig{
			Domain: "test-domain",
		},
		Workers: []WorkerConfig{
			{TaskList: "test-tasklist"},
		},
	}

	logger := zap.NewNop()

	workers, err := newTemporalWorker(mockFactory, conf, logger)

	assert.NoError(t, err)
	assert.Len(t, workers, 1)

	mockFactory.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestNewCadenceWorker(t *testing.T) {
	mockClient := &MockCadenceClient{}

	mockFactory := &MockCadenceClientFactory{}
	mockFactory.
		On("NewCadenceClient", mock.Anything).
		Return(mockClient, nil)

	conf := Config{
		Provider:  "cadence",
		Transport: "grpc",
		Host:      "localhost:7833",
		Workers: []WorkerConfig{
			{
				Domain:   "test-domain",
				TaskList: "test-tasklist",
			},
		},
	}

	logger := zap.NewNop()

	workers, err := newCadenceWorker(mockFactory, conf, logger)

	assert.NoError(t, err)
	assert.Len(t, workers, 1)

	mockFactory.AssertExpectations(t)
}
