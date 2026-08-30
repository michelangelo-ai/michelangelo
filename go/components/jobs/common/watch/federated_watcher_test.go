package watch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/cluster/clustermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/metrics"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// fakeController implements cache.Controller for testing.
type fakeController struct {
	runFunc func(stopCh <-chan struct{})
}

func (f *fakeController) Run(stopCh <-chan struct{}) {
	if f.runFunc != nil {
		f.runFunc(stopCh)
	}
}

func (f *fakeController) HasSynced() bool                 { return true }
func (f *fakeController) LastSyncResourceVersion() string { return "" }

var _testCluster = v2pb.Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: "michelangelo.uber.com/v2beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:            "testCluster",
		Namespace:       constants.ClustersNamespace,
		ResourceVersion: "999",
	},
	Spec: v2pb.ClusterSpec{
		Region: "phx",
		Zone:   "phx5",
		Cluster: &v2pb.ClusterSpec_Kubernetes{
			Kubernetes: &v2pb.KubernetesSpec{
				Rest: &v2pb.ConnectionSpec{
					Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
					Port: "port",
				},
			},
		},
	},
}

type test struct {
	cluster v2pb.Cluster
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
			cluster: v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
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
			cluster: v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
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

			mockClusterCache := clustermocks.NewMockRegisteredClustersCache(gctrl)
			mockClusterCache.EXPECT().GetClusters(cluster.ReadyClusters).Return([]*v2pb.Cluster{&_testCluster})

			w := setupWatcher(t, test, &wg, gctrl)
			federatedWatcher := w.(*watcher)
			federatedWatcher.clusterCache = mockClusterCache

			federatedWatcher.sync()

			// test that clusterInfoMap has _testCluster
			_, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			// change the cluster
			mockClusterCache.EXPECT().GetClusters(cluster.ReadyClusters).Return([]*v2pb.Cluster{&test.cluster})

			federatedWatcher.sync()

			// test cluster update
			clusterInfo, ok := federatedWatcher.clusterInfoMap.Load(_testCluster.Name)
			require.True(t, ok)

			cl := clusterInfo.(watchInfo).cluster
			require.Equal(t, test.cluster.Name, cl.Name)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Host, cl.Spec.GetKubernetes().Rest.Host)
			require.Equal(t, test.cluster.Spec.GetKubernetes().Rest.Port, cl.Spec.GetKubernetes().Rest.Port)
		})
	}
}

func setupWatcher(t *testing.T, test test, wg *sync.WaitGroup, gctrl *gomock.Controller) FederatedWatcher {
	times := 1 // the first call
	if test.changed {
		// if the test mimics a change it would trigger another sync.
		times++
	}

	wg.Add(times)

	mockClient := clientmocks.NewMockFederatedClient(gctrl)
	mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ []*client.WatcherParams, _ *v2pb.Cluster) ([]*client.ResourceWatcher, error) {
			return []*client.ResourceWatcher{
				{
					Controller: &fakeController{
						runFunc: func(_ <-chan struct{}) {
							wg.Done()
						},
					},
				},
			}, nil
		},
	).Times(times)

	testScope := tally.NewTestScope("test", map[string]string{})

	federatedWatcher := NewFederatedWatcher(
		FederatedWatcherParams{
			Logger:          zapr.NewLogger(zaptest.NewLogger(t)),
			FederatedClient: mockClient,
			Scope:           testScope,
		})

	return federatedWatcher
}

func TestStartWatchControllerRecoverer(t *testing.T) {
	testScope := tally.NewTestScope("test", map[string]string{})
	w := &watcher{
		log: zapr.NewLogger(zap.NewNop()),
		metrics: &metrics.ControllerMetrics{
			MetricsScope: testScope,
		},
	}

	ctrl := &fakeController{
		runFunc: func(_ <-chan struct{}) {
			panic(errors.New("test error"))
		},
	}

	stopCh := make(chan struct{})

	w.startWatchGoRoutine(&client.ResourceWatcher{
		Controller: ctrl,
	}, stopCh, "testCluster")

	require.NotNil(t, testScope.Snapshot())
	require.NotNil(t, testScope.Snapshot().Counters())
	val, ok := testScope.Snapshot().Counters()[fmt.Sprintf("%s.%s+", "test", _failureWatchPanicMetricName)]
	require.True(t, ok)
	require.Equal(t, int64(1), val.Value())
}

func TestSyncError(t *testing.T) {
	tt := []struct {
		existingCluster *v2pb.Cluster
		newCluster      *v2pb.Cluster
		msg             string
	}{
		{
			existingCluster: &v2pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existingCluster",
				},
			},
			newCluster: &_testCluster,
			msg:        "error in adding cluster",
		},
		{
			existingCluster: &_testCluster,
			newCluster: &v2pb.Cluster{
				ObjectMeta: _testCluster.ObjectMeta,
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: _testCluster.Spec.GetKubernetes().Rest.Host,
								Port: "NewPort",
							},
						},
					},
				},
			},
			msg: "error in updating cluster",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
			mockClusterCache.EXPECT().GetClusters(cluster.ReadyClusters).Return([]*v2pb.Cluster{test.newCluster})

			mockClient := clientmocks.NewMockFederatedClient(g)
			mockClient.EXPECT().Watcher(gomock.Any(), gomock.Any()).Return(nil, errors.New("test error"))

			federatedWatcher := &watcher{
				clusterCache:    mockClusterCache,
				federatedClient: mockClient,
				log:             zapr.NewLogger(zaptest.NewLogger(t)),
				metrics: &metrics.ControllerMetrics{
					MetricsScope: tally.NewTestScope("test", map[string]string{}),
				},
			}

			federatedWatcher.clusterInfoMap.Store(test.existingCluster.Name, watchInfo{
				cluster: test.existingCluster,
			})

			federatedWatcher.sync()
		})
	}
}

func TestStart(t *testing.T) {
	g := gomock.NewController(t)
	mockClusterCache := clustermocks.NewMockRegisteredClustersCache(g)
	mockClusterCache.EXPECT().GetClusters(cluster.ReadyClusters).Return([]*v2pb.Cluster{}).AnyTimes()

	federatedWatcher := &watcher{
		clusterCache: mockClusterCache,
		log:          zapr.NewLogger(zaptest.NewLogger(t)),
		period:       time.Millisecond,
	}

	// cancel the context after allowing the watcher to run at least once. There's nothing to sync.
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	require.NotPanics(t, func() {
		federatedWatcher.start(ctx)
	})
}
