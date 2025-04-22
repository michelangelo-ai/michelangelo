package workflowfx

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
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

// --- Test provide() function directly ---

func TestProvide_Temporal(t *testing.T) {
	conf := Config{
		Provider: "temporal",
		Host:     "localhost:7233",
		Client: ClientConfig{
			Domain: "test-domain",
		},
		Workers: []WorkerConfig{{TaskList: "test-tasklist"}},
	}

	logger := zap.NewNop()
	mockTemporalFactory := &MockTemporalClientFactory{}

	in := In{
		Config:          conf,
		Logger:          logger,
		TemporalFactory: mockTemporalFactory,
	}

	out, err := provide(in)

	assert.NoError(t, err)
	assert.Equal(t, service.BackendType("temporal"), out.Backend)
	assert.Len(t, out.Workers, 1)
}

func TestProvide_Cadence(t *testing.T) {
	conf := Config{
		Provider:  "cadence",
		Transport: "grpc",
		Host:      "localhost:7833",
		Workers: []WorkerConfig{{
			Domain:   "test-domain",
			TaskList: "test-tasklist",
		}},
	}

	logger := zap.NewNop()
	mockCadenceClient := &MockCadenceClient{}
	mockCadenceFactory := &MockCadenceClientFactory{}
	mockCadenceFactory.
		On("NewCadenceClient", mock.Anything).
		Return(mockCadenceClient, nil)

	in := In{
		Config:         conf,
		Logger:         logger,
		CadenceFactory: mockCadenceFactory,
	}

	out, err := provide(in)

	assert.NoError(t, err)
	assert.Equal(t, service.BackendType("cadence"), out.Backend)
	assert.Len(t, out.Workers, 1)

	mockCadenceFactory.AssertExpectations(t)
}
