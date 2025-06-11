package watch

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"code.uber.internal/go/envfx.git"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client"
	federatedClient "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/uke"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/clientmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster/clustermock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute/computemock"
	"mock/k8s.io/client-go/tools/cache/cachemock"
)

var _testCluster = v2beta1pb.Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: "michelangelo.uber.com/v2beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "testCluster",
		Namespace:       constants.ClustersNamespace,
		ResourceVersion: "999",
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
}

type test struct {
	cluster v2beta1pb.Cluster
	changed bool
	msg     string
}

func TestSync(t *testing.T) {
	tt := []test{
		{
			cluster: _testCluster,
			changed: false,
			msg:     "no change",
		},
		{
			cluster: v2beta1pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2beta1pb.ClusterSpec{
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: _testCluster.Spec.GetKubernetes().Rest.Host,
								Port: "NewPort",
							},
						},
					},
				},
			},
			changed: true,
			msg:     "port change",
		},
		{
			cluster: v2beta1pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2beta1pb.ClusterSpec{
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "NewHost",
								Port: _testCluster.Spec.GetKubernetes().Rest.Port,
							},
						},
					},
				},
			},
			changed: true,
			msg:     "host change",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			var wg sync.WaitGroup
			defer wg.Wait()
			gctrl := gomock.NewController(t)

			mockClusterCache := clustermock.NewMockRegisteredClustersCache(gctrl)
			mockClusterCache.EXPECT().GetClusters(cluster.AllClusters).Return([]*v2beta1pb.Cluster{&_testCluster})

			w := setupWatcher(t, test, &wg)
			federatedWatcher := w.(*watcher)
			federatedWatcher.clusterCache = mockClusterCache

			err := federatedWatcher.sync()
			require.NoError(t, err)

			// test that clusterInfoMap has _testCluster
			_, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			// change the cluster
			mockClusterCache.EXPECT().GetClusters(cluster.AllClusters).Return([]*v2beta1pb.Cluster{&test.cluster})

			err = federatedWatcher.sync()
			require.NoError(t, err)

			// test cluster update
			clusterInfo, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			cluster := clusterInfo.(watchInfo).cluster
			require.Equal(t, test.cluster.Name, cluster.Name)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Host, cluster.Spec.GetKubernetes().Rest.Host)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Port, cluster.Spec.GetKubernetes().Rest.Port)
		})
	}
}

func setupWatcher(t *testing.T, test test, wg *sync.WaitGroup) FederatedWatcher {
	gctrl := gomock.NewController(t)
	mockFactory := computemock.NewMockFactory(gctrl)
	mockCache := cachemock.NewMockController(gctrl)
	mockHelper := clientmock.NewMockHelper(gctrl)

	scheme := runtime.NewScheme()
	err := v2beta1pb.AddToScheme(scheme)
	require.NoError(t, err)

	times := 1 // the first call
	if test.changed {
		// if the test mimics a change it would trigger another sync.
		times++
	}

	wg.Add(times)
	mockCache.EXPECT().Run(gomock.Any()).Do(func(_ <-chan struct{}) {
		wg.Done()
	}).AnyTimes()
	mockHelper.EXPECT().Watcher(gomock.Any()).Return([]*federatedClient.ResourceWatcher{
		{
			Controller: mockCache,
		},
	}, nil).Times(times)
	mockFactory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil).Times(times)

	testScope := tally.NewTestScope("test", map[string]string{})

	federatedWatcher := NewFederatedWatcher(
		FederatedWatcherParams{
			Logger: zapr.NewLogger(zaptest.NewLogger(t)),
			FederatedClient: client.NewClient(client.Params{
				Factory: mockFactory,
				Logger:  zap.NewNop(),
				Mapper: uke.NewUkeMapper(uke.MapperParams{
					Env:   envfx.New().Environment,
					Scope: testScope,
				}).Mapper,
				Helper: mockHelper,
			}),
			Scope: testScope,
		})

	return federatedWatcher
}

func TestStartWatchControllerRecoverer(t *testing.T) {
	g := gomock.NewController(t)
	ctrl := cachemock.NewMockController(g)
	testScope := tally.NewTestScope("test", map[string]string{})
	w := &watcher{
		log: zapr.NewLogger(zap.NewNop()),
		metrics: &metrics.ControllerMetrics{
			MetricsScope: testScope,
		},
	}

	ctrl.EXPECT().Run(gomock.Any()).Do(func(_ <-chan struct{}) {
		panic(errors.New("test error"))
	})

	w.startWatchGoRoutine(&federatedClient.ResourceWatcher{
		Controller: ctrl,
	}, "testCluster")

	require.NotNil(t, testScope.Snapshot())
	require.NotNil(t, testScope.Snapshot().Counters())
	val, ok := testScope.Snapshot().Counters()[fmt.Sprintf("%s.%s+", "test", _failureWatchPanicMetricName)]
	require.True(t, ok)
	require.Equal(t, int64(1), val.Value())
}
