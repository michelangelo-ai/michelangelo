package secrets

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestModule_WithKubeClient_ProvidesSecretProvider(t *testing.T) {
	err := fx.ValidateApp(
		Module,
		fx.Supply(zap.NewNop()),
		fx.Provide(func() client.Client { return fake.NewClientBuilder().Build() }),
		fx.Invoke(func(SecretProvider) {}),
	)
	require.NoError(t, err)
}
