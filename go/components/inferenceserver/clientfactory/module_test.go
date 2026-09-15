package clientfactory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory/secrets"
)

func TestNewSecretProviderSelection(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	tests := []struct {
		name      string
		cfg       secrets.Config
		expectESO bool
		expectErr string
	}{
		{name: "default is sample", cfg: secrets.Config{}},
		{name: "explicit sample", cfg: secrets.Config{Provider: secrets.ProviderSample}},
		{name: "eso", cfg: secrets.Config{Provider: secrets.ProviderESO}, expectESO: true},
		{name: "unknown provider", cfg: secrets.Config{Provider: "vault"}, expectErr: "unknown secrets.provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := newSecretProvider(kubeClient, tt.cfg)
			if tt.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectErr)
				return
			}
			require.NoError(t, err)
			if tt.expectESO {
				require.IsType(t, &secrets.ESOProvider{}, provider)
			} else {
				require.IsType(t, &secrets.Provider{}, provider)
			}
		})
	}
}

func TestNewClientFactory(t *testing.T) {
	kubeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	factory, err := newClientFactory(kubeClient, secrets.Config{}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, factory)

	_, err = newClientFactory(kubeClient, secrets.Config{Provider: "vault"}, zap.NewNop())
	require.Error(t, err)
}
