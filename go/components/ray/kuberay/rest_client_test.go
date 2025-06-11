package kuberay

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestNewRestClient(t *testing.T) {
	type args struct {
		config *rest.Config
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test new restClient",
			args: args{config: &rest.Config{
				Host: "host1",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRestClient(tt.args.config)
			require.NotNil(t, got)
			require.Nil(t, err)
		})
	}
}
