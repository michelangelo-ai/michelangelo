package compute

import (
	"testing"

	v2beta1pb "michelangelo/api/v2beta1"

	infraAuth "code.uber.internal/infra/compute/k8s-auth"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewClientSetFactory(t *testing.T) {

	type test struct {
		name          string
		clientAuthMap map[string]*infraAuth.ClientAuth
		want          *factory
	}
	tt := []test{
		{
			name:          "constructor success",
			clientAuthMap: map[string]*infraAuth.ClientAuth{},
			want: &factory{
				zonalAuth: map[string]*infraAuth.ClientAuth{},
			},
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			factory := setupFactory(test.clientAuthMap, nil)
			require.NotNil(t, factory)
			require.Equal(t, test.want, factory)
		})
	}
}

func TestGetClientSetForCluster(t *testing.T) {

	type test struct {
		name          string
		clientAuthMap map[string]*infraAuth.ClientAuth
		clients       map[string]*ClientSet
		req           v2beta1pb.Cluster
		wantError     string
	}

	tt := []test{
		{
			name:          "auth missing for zone in cfg",
			clientAuthMap: map[string]*infraAuth.ClientAuth{},
			clients:       map[string]*ClientSet{},
			req: v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testCluster",
					Namespace: constants.ClustersNamespace,
				},
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "port",
							},
						},
					},
				},
			},
			wantError: "client cfg err:auth for zone phx5 not provided in the configuration",
		},
		{
			name:          "GetClientSetForCluster success",
			clientAuthMap: map[string]*infraAuth.ClientAuth{},
			clients:       map[string]*ClientSet{"https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal:port": {}},
			req: v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testCluster",
					Namespace: constants.ClustersNamespace,
				},
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "port",
							},
						},
					},
				},
			},
			wantError: "",
		},
		{
			name:          "failure - kuberay client error",
			clientAuthMap: map[string]*infraAuth.ClientAuth{"phx5": {}},
			clients:       map[string]*ClientSet{},
			req: v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testCluster",
					Namespace: constants.ClustersNamespace,
				},
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "port",
							},
						},
					},
				},
			},
			wantError: "kuberay client err:host must be a URL or a host:port pair: \"https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal:port\"",
		},
		{
			name:          "success",
			clientAuthMap: map[string]*infraAuth.ClientAuth{"phx5": {}},
			clients:       map[string]*ClientSet{},
			req: v2beta1pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testCluster",
					Namespace: constants.ClustersNamespace,
				},
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "80",
							},
						},
					},
				},
			},
			wantError: "",
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			factory := setupFactory(test.clientAuthMap, test.clients)
			out, err := factory.GetClientSetForCluster(&test.req)
			if test.wantError == "" {
				require.NoError(t, err)
				require.NotNil(t, out)
				return
			}
			require.Error(t, err)
			require.Equal(t, test.wantError, err.Error())
		})
	}
}

func setupFactory(clientAuthMap map[string]*infraAuth.ClientAuth, clients map[string]*ClientSet) Factory {
	f := factory{
		zonalAuth: clientAuthMap,
	}

	if clients != nil {
		for k, v := range clients {
			f.clients.Store(k, v)
		}
	}
	return &f
}
