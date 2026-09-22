package common

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/modelconfig/modelconfigmocks"
)

func TestCheckModelExists(t *testing.T) {
	const (
		isName     = "test-server"
		namespace  = "default"
		deployment = "deployment-a"
		model      = "shared-model"
	)

	tests := []struct {
		name    string
		entries []modelconfig.ModelConfigEntry
		want    bool
	}{
		{
			name:    "empty config",
			entries: []modelconfig.ModelConfigEntry{},
			want:    false,
		},
		{
			name:    "this deployment's entry",
			entries: []modelconfig.ModelConfigEntry{{Name: model, DeploymentName: deployment}},
			want:    true,
		},
		{
			name:    "only another deployment's entry for the same model",
			entries: []modelconfig.ModelConfigEntry{{Name: model, DeploymentName: "deployment-b"}},
			want:    false,
		},
		{
			name: "this deployment's entry alongside another deployment's",
			entries: []modelconfig.ModelConfigEntry{
				{Name: model, DeploymentName: "deployment-b"},
				{Name: model, DeploymentName: deployment},
			},
			want: true,
		},
		{
			name:    "entry with no deployment name",
			entries: []modelconfig.ModelConfigEntry{{Name: model}},
			want:    false,
		},
		{
			name:    "different model",
			entries: []modelconfig.ModelConfigEntry{{Name: "other-model", DeploymentName: deployment}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			provider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
			provider.EXPECT().
				GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), isName, namespace).
				Return(tt.entries, nil)

			got, err := CheckModelExists(context.Background(), zap.NewNop(), provider, nil, deployment, model, isName, namespace)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckModelExistsPropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	provider := modelconfigmocks.NewMockModelConfigProvider(ctrl)
	provider.EXPECT().
		GetModelsFromConfig(gomock.Any(), gomock.Any(), gomock.Any(), "test-server", "default").
		Return(nil, errors.New("connection error"))

	got, err := CheckModelExists(context.Background(), zap.NewNop(), provider, nil, "deployment-a", "model", "test-server", "default")
	require.Error(t, err)
	assert.False(t, got)
}
