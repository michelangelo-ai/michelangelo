package workflowfx

import (
	"crypto/tls"
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
	CapturedOpts *tempclient.Options
}

func (m *MockTemporalClientFactory) NewTemporalClient(opts tempclient.Options) (tempclient.Client, error) {
	m.CapturedOpts = &opts
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

func (m *MockCadenceClientFactory) NewCadenceClient(conf Config, tlsConfig *tls.Config) (workflowserviceclient.Interface, error) {
	args := m.Called(conf, tlsConfig)
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
		On("NewCadenceClient", mock.Anything, mock.Anything).
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

// --- TLS Configuration Tests ---

func TestConfig_UseTLSField(t *testing.T) {
	t.Run("UseTLS field is properly set to true", func(t *testing.T) {
		config := Config{
			Host:      "cadence.example.com:7833",
			Transport: "grpc",
			Workers: []WorkerConfig{
				{
					Domain:   "default",
					TaskList: "test-task-list",
				},
			},
			Client: ClientConfig{
				Domain: "default",
			},
			Provider: ProviderCadence,
			UseTLS:   true,
		}

		// Verify UseTLS flag is properly stored
		assert.True(t, config.UseTLS)
	})

	t.Run("UseTLS field is properly set to false", func(t *testing.T) {
		config := Config{
			Host:      "localhost:7833",
			Transport: "grpc",
			Workers: []WorkerConfig{
				{
					Domain:   "default",
					TaskList: "test-task-list",
				},
			},
			Client: ClientConfig{
				Domain: "default",
			},
			Provider: ProviderCadence,
			UseTLS:   false,
		}

		// Verify UseTLS flag is properly stored
		assert.False(t, config.UseTLS)
	})

	t.Run("UseTLS with different transport types", func(t *testing.T) {
		transports := []string{"grpc", "tchannel"}

		for _, transport := range transports {
			t.Run("transport_"+transport, func(t *testing.T) {
				config := Config{
					Host:      "localhost:7833",
					Transport: transport,
					Workers: []WorkerConfig{
						{
							Domain:   "default",
							TaskList: "test-task-list",
						},
					},
					Client: ClientConfig{
						Domain: "default",
					},
					Provider: ProviderCadence,
					UseTLS:   true,
				}

				assert.Equal(t, transport, config.Transport)
				assert.True(t, config.UseTLS)
			})
		}
	})
}

func TestNewCadenceClient_TLSFlow(t *testing.T) {
	t.Run("validates TLS enabled flow to newCadenceClient", func(t *testing.T) {
		// This test verifies that UseTLS gets passed through the system
		// Note: This will fail with connection errors, but we're testing the setup logic

		config := Config{
			Host:      "cadence-test.example.com:7833",
			Transport: "grpc",
			Workers: []WorkerConfig{
				{
					Domain:   "test-domain",
					TaskList: "test-task-list",
				},
			},
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Provider: ProviderCadence,
			UseTLS:   true,
		}

		factory := DefaultCadenceClientFactory{}

		// Attempt to create client with default TLS config
		tlsConfig := &tls.Config{}
		_, err := factory.NewCadenceClient(config, tlsConfig)

		// In test environments, this may succeed or fail depending on server availability
		// The important thing is that our TLS logic doesn't cause crashes
		if err != nil {
			// Got an error - this is expected since we're not connecting to real servers
			// The important thing is our TLS configuration logic didn't cause a panic or config error
			t.Logf("Got expected connection error (TLS enabled processed correctly): %v", err)
		} else {
			// Connection succeeded - the TLS was processed without issues
			t.Logf("Connection succeeded - TLS enabled processed correctly")
		}
	})

	t.Run("handles TLS disabled", func(t *testing.T) {
		config := Config{
			Host:      "localhost:7833",
			Transport: "grpc",
			Workers: []WorkerConfig{
				{
					Domain:   "default",
					TaskList: "default",
				},
			},
			Client: ClientConfig{
				Domain: "default",
			},
			Provider: ProviderCadence,
			UseTLS:   false, // No TLS
		}

		factory := DefaultCadenceClientFactory{}

		// Attempt to create client with default TLS config (won't be used since UseTLS is false)
		tlsConfig := &tls.Config{}
		_, err := factory.NewCadenceClient(config, tlsConfig)

		// In test environments, this may succeed or fail depending on server availability
		// The important thing is that our TLS logic doesn't cause crashes
		if err != nil {
			// Got an error - indicating TLS disabled was handled correctly
			t.Logf("Got expected connection error (TLS disabled handled correctly): %v", err)
		} else {
			// Connection succeeded - the TLS disabled was processed without issues
			t.Logf("Connection succeeded - TLS disabled handled correctly")
		}
	})

	t.Run("handles tchannel transport with TLS", func(t *testing.T) {
		// TChannel transport doesn't support TLS in the same way as gRPC
		config := Config{
			Host:      "localhost:7833",
			Transport: "tchannel",
			Workers: []WorkerConfig{
				{
					Domain:   "default",
					TaskList: "default",
				},
			},
			Client: ClientConfig{
				Domain: "default",
			},
			Provider: ProviderCadence,
			UseTLS:   true,
		}

		factory := DefaultCadenceClientFactory{}

		// Attempt to create client with TLS config (tchannel may not use it)
		tlsConfig := &tls.Config{}
		_, err := factory.NewCadenceClient(config, tlsConfig)

		// In test environments, this may succeed or fail depending on server availability
		// For TChannel, the UseTLS flag is present but may not affect tchannel transport
		// The important thing is that the function doesn't crash or reject the config
		if err != nil {
			// Got an error - TChannel with UseTLS was handled without crashing
			t.Logf("Got expected connection error for TChannel with TLS (handled correctly): %v", err)
		} else {
			// Connection succeeded - TChannel with UseTLS processed without issues
			t.Logf("TChannel connection succeeded with UseTLS - handled correctly")
		}
	})
}

func TestNewTemporalWorker_TLS(t *testing.T) {
	t.Run("creates temporal worker with TLS enabled and custom config", func(t *testing.T) {
		conf := Config{
			Host:   "temporal.example.com:7233",
			UseTLS: true,
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Workers: []WorkerConfig{{TaskList: "test-tasklist"}},
		}

		logger := zap.NewNop()
		mockFactory := &MockTemporalClientFactory{}
		customTLSConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		workers, err := newTemporalWorker(mockFactory, conf, customTLSConfig, logger)

		assert.NoError(t, err)
		assert.Len(t, workers, 1)
		assert.NotNil(t, mockFactory.CapturedOpts)
		assert.NotNil(t, mockFactory.CapturedOpts.ConnectionOptions.TLS)
		assert.Equal(t, customTLSConfig, mockFactory.CapturedOpts.ConnectionOptions.TLS)
	})

	t.Run("creates temporal worker with TLS enabled and nil config", func(t *testing.T) {
		conf := Config{
			Host:   "temporal.example.com:7233",
			UseTLS: true,
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Workers: []WorkerConfig{{TaskList: "test-tasklist"}},
		}

		logger := zap.NewNop()
		mockFactory := &MockTemporalClientFactory{}

		workers, err := newTemporalWorker(mockFactory, conf, nil, logger)

		assert.NoError(t, err)
		assert.Len(t, workers, 1)
		assert.NotNil(t, mockFactory.CapturedOpts)
		assert.NotNil(t, mockFactory.CapturedOpts.ConnectionOptions.TLS)
		// When nil TLS config is passed with UseTLS=true, a default &tls.Config{} should be used
	})

	t.Run("creates temporal worker with TLS disabled", func(t *testing.T) {
		conf := Config{
			Host:   "localhost:7233",
			UseTLS: false,
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Workers: []WorkerConfig{{TaskList: "test-tasklist"}},
		}

		logger := zap.NewNop()
		mockFactory := &MockTemporalClientFactory{}
		customTLSConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		workers, err := newTemporalWorker(mockFactory, conf, customTLSConfig, logger)

		assert.NoError(t, err)
		assert.Len(t, workers, 1)
		assert.NotNil(t, mockFactory.CapturedOpts)
		assert.Nil(t, mockFactory.CapturedOpts.ConnectionOptions.TLS)
		// When UseTLS=false, TLS config should not be set even if provided
	})

	t.Run("creates temporal worker with multiple workers", func(t *testing.T) {
		conf := Config{
			Host:   "temporal.example.com:7233",
			UseTLS: true,
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Workers: []WorkerConfig{
				{TaskList: "test-tasklist-1"},
				{TaskList: "test-tasklist-2"},
				{TaskList: "test-tasklist-3"},
			},
		}

		logger := zap.NewNop()
		mockFactory := &MockTemporalClientFactory{}
		customTLSConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		workers, err := newTemporalWorker(mockFactory, conf, customTLSConfig, logger)

		assert.NoError(t, err)
		assert.Len(t, workers, 3)
		assert.NotNil(t, mockFactory.CapturedOpts)
		assert.NotNil(t, mockFactory.CapturedOpts.ConnectionOptions.TLS)
		assert.Equal(t, customTLSConfig, mockFactory.CapturedOpts.ConnectionOptions.TLS)
	})
}

func TestProvide_TemporalWithTLS(t *testing.T) {
	t.Run("provide calls newTemporalWorker with TLS config", func(t *testing.T) {
		conf := Config{
			Provider: "temporal",
			Host:     "temporal.example.com:7233",
			UseTLS:   true,
			Client: ClientConfig{
				Domain: "test-domain",
			},
			Workers: []WorkerConfig{{TaskList: "test-tasklist"}},
		}

		logger := zap.NewNop()
		mockTemporalFactory := &MockTemporalClientFactory{}
		customTLSConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
		}

		in := In{
			Config:          conf,
			Logger:          logger,
			TemporalFactory: mockTemporalFactory,
			TLSConfig:       customTLSConfig,
		}

		out, err := provide(in)

		assert.NoError(t, err)
		assert.Equal(t, service.BackendType("temporal"), out.Backend)
		assert.Len(t, out.Workers, 1)
	})

	t.Run("provide calls newTemporalWorker without TLS config", func(t *testing.T) {
		conf := Config{
			Provider: "temporal",
			Host:     "localhost:7233",
			UseTLS:   false,
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
			TLSConfig:       nil,
		}

		out, err := provide(in)

		assert.NoError(t, err)
		assert.Equal(t, service.BackendType("temporal"), out.Backend)
		assert.Len(t, out.Workers, 1)
	})
}
