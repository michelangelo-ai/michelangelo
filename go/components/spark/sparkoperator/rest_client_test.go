package sparkoperator

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			// The constructor copies the config by value: compute/factory.go reuses one
			// *rest.Config to build several clients, so mutating it would corrupt the siblings.
			require.Nil(t, tt.args.config.GroupVersion, "caller's config was mutated")
			require.Nil(t, tt.args.config.NegotiatedSerializer, "caller's config was mutated")
		})
	}
}

func TestScheme(t *testing.T) {
	// Pins the group/version to the vendored CRD package: this fails loudly if the
	// vendored group ever moves, which a re-declared literal would silently survive.
	require.Equal(t, "sparkoperator.k8s.io", SchemeGroupVersion.Group)
	require.Equal(t, "v1beta2", SchemeGroupVersion.Version)
	require.True(t, Scheme.Recognizes(SchemeGroupVersion.WithKind("SparkApplication")))
}

func TestParameterCodecEncodesListOptions(t *testing.T) {
	// The vendored addKnownTypes calls metav1.AddToGroupVersion, so meta v1 query parameters
	// encode against this group-version. The federated client's SparkApplication list-watch
	// (go/components/jobs/client/helper.go) relies on this for its list and watch options.
	v, err := ParameterCodec.EncodeParameters(&metav1.ListOptions{Watch: true}, SchemeGroupVersion)
	require.NoError(t, err)
	require.Equal(t, "true", v.Get("watch"))
}
