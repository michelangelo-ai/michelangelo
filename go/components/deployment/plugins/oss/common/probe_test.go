package common

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/clientfactorymocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestIsTerminalFailure(t *testing.T) {
	assert.False(t, IsTerminalFailure(nil, "A"))
	assert.False(t, IsTerminalFailure(&apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Message: "Other"}, "A", "B"))
	assert.False(t, IsTerminalFailure(&apipb.Condition{Status: apipb.CONDITION_STATUS_UNKNOWN, Message: "A"}, "A"))
	assert.True(t, IsTerminalFailure(&apipb.Condition{Status: apipb.CONDITION_STATUS_FALSE, Message: "B"}, "A", "B"))
}

func TestProbeModelStatus(t *testing.T) {
	target := &v2pb.ClusterTarget{ClusterId: "c1"}
	want := &backends.ModelStatus{Desired: 1, Replicas: []backends.ReplicaModelStatus{{Replica: "pod-a", Running: true, State: backends.ModelLoadStateReady}}}

	tests := []struct {
		name            string
		registerBackend bool
		getClientErr    error
		getHTTPErr      error
		statusErr       error
		wantReason      string
	}{
		{name: "backend missing", wantReason: ReasonBackendUnavailable},
		{name: "client unavailable", registerBackend: true, getClientErr: errors.New("auth refused"), wantReason: ReasonClientUnavailable},
		{name: "http client unavailable", registerBackend: true, getHTTPErr: errors.New("dial timeout"), wantReason: ReasonHTTPClientUnavailable},
		{name: "probe fails", registerBackend: true, statusErr: errors.New("api error"), wantReason: ReasonModelStatusCheckFailed},
		{name: "probe succeeds", registerBackend: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			factory := clientfactorymocks.NewMockClientFactory(ctrl)
			factory.EXPECT().GetClient(gomock.Any(), target).Return(client.Client(nil), tt.getClientErr).AnyTimes()
			factory.EXPECT().GetHTTPClient(gomock.Any(), target).Return((*http.Client)(nil), tt.getHTTPErr).AnyTimes()

			backend := backendsmocks.NewMockBackend(ctrl)
			if tt.registerBackend && tt.getClientErr == nil && tt.getHTTPErr == nil {
				backend.EXPECT().GetModelStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), "is", "ns", "model").
					Return(want, tt.statusErr)
			}
			registry := backends.NewRegistry()
			if tt.registerBackend {
				registry.Register(v2pb.BACKEND_TYPE_TRITON, backend)
			}

			got, failure := ProbeModelStatus(context.Background(), zap.NewNop(), factory, registry, v2pb.BACKEND_TYPE_TRITON, target, "ns", "is", "model")

			if tt.wantReason == "" {
				require.Nil(t, failure)
				assert.Equal(t, want, got)
				return
			}
			require.NotNil(t, failure)
			assert.Equal(t, tt.wantReason, failure.Reason)
			assert.NotEmpty(t, failure.Message)
			assert.Nil(t, got)
		})
	}
}
