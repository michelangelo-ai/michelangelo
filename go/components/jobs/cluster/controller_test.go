package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	handler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	handlermocks "github.com/michelangelo-ai/michelangelo/go/api/handler/handlermocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/clientmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	_testClusterName  = "cluster-1"
	_testCluster2Name = "cluster-2"
)

var _testCluster = v2pb.Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: "michelangelo.api/v2",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      _testClusterName,
		Namespace: constants.ClustersNamespace,
	},
	Spec: v2pb.ClusterSpec{
		Region: "phx",
		Zone:   "phx5",
		Dc:     v2pb.DC_TYPE_ON_PREM,
		Cluster: &v2pb.ClusterSpec_Kubernetes{
			Kubernetes: &v2pb.KubernetesSpec{
				Rest: &v2pb.ConnectionSpec{
					Host: "https://host.docker.internal",
					Port: "6443",
				},
			},
		},
	},
}

// testParams holds parameters for setting up test reconciler
type testParams struct {
	gCtrl   *gomock.Controller
	factory compute.Factory
	helper  client.Helper
}

// createTestScheme creates a test scheme with v2pb types registered
func createTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := kubescheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add kube scheme: %v", err)
	}
	if err := v2pb.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v2pb scheme: %v", err)
	}
	return scheme
}

// createTestClient creates a test client with proper type assertion
func createTestClient(t *testing.T, factory compute.Factory, helper client.Helper, logger *zap.Logger) *client.Client {
	federatedClient := client.NewClient(client.Params{
		Factory: factory,
		Helper:  helper,
		Logger:  logger,
	})
	clusterClient, ok := federatedClient.(*client.Client)
	if !ok {
		t.Fatal("failed to type assert FederatedClient to *Client")
	}
	return clusterClient
}

// setupReconciler creates a reconciler with mocked dependencies for testing
func setupReconciler(t *testing.T, params testParams) (*Reconciler, *apimocks.MockHandler) {
	gCtrl := gomock.NewController(t)
	if params.gCtrl != nil {
		gCtrl = params.gCtrl
	}

	apiHandler := apimocks.NewMockHandler(gCtrl)
	testScope := tally.NewTestScope("test", map[string]string{})
	logger := zaptest.NewLogger(t)

	r := NewReconciler(Params{
		Scope:         testScope,
		ClusterClient: createTestClient(t, params.factory, params.helper, logger),
	}).Reconciler

	r.log = zapr.NewLogger(zaptest.NewLogger(t))
	r.Handler = apiHandler
	return r, apiHandler
}

func TestReconcile(t *testing.T) {
	defaultReq := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      _testClusterName,
			Namespace: constants.ClustersNamespace,
		},
	}

	tests := []struct {
		msg               string
		setupFunc         func(t *testing.T) (*Reconciler, tally.TestScope)
		assertClusterData func(d *clusterMap) bool
		req               ctrl.Request
		wantErr           string
	}{
		{
			msg: "add new cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				cluster := _testCluster.DeepCopy()
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *cluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should include the cluster
				clusterData := d.get(_testClusterName)
				return clusterData != nil
			},
		},
		{
			msg: "delete cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{})

				cluster := _testCluster.DeepCopy()
				cluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				cluster.Finalizers = []string{_clusterDeleteFinalizer}
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *cluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// no cluster in the map
				clusterData := d.get(_testClusterName)
				return clusterData == nil
			},
		},
		{
			msg: "update cluster label should not update cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// add cluster to map with label sla=test
				cachedCluster := _testCluster.DeepCopy()
				cachedCluster.Labels = map[string]string{
					"sla": "test",
				}
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: cachedCluster,
				})

				updatedCluster := _testCluster.DeepCopy()
				updatedCluster.Labels = map[string]string{"sla": "production"}
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *updatedCluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should still have the cluster with old label (no reconcile)
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Labels["sla"] == "test"
			},
		},
		{
			msg: "update cluster spec should trigger update",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				cachedCluster := _testCluster.DeepCopy()
				cachedCluster.ObjectMeta.Finalizers = []string{_clusterDeleteFinalizer}
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: cachedCluster,
				})

				updatedCluster := _testCluster.DeepCopy()
				updatedCluster.ObjectMeta.Finalizers = []string{_clusterDeleteFinalizer}
				updatedCluster.Spec.Sla = v2pb.SLA_TYPE_PRODUCTION

				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *updatedCluster
						return nil
					})

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should have the cluster with new SLA
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Spec.GetSla() == v2pb.SLA_TYPE_PRODUCTION
			},
		},
		{
			msg: "update cluster annotation should not update cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// add cluster to map with annotation sla=test
				cachedCluster := _testCluster.DeepCopy()
				cachedCluster.Annotations = map[string]string{
					"sla": "test",
				}
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: cachedCluster,
				})

				updatedCluster := _testCluster.DeepCopy()
				updatedCluster.Annotations = map[string]string{"sla": "production"}
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *updatedCluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should still have cluster with old annotation
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Annotations["sla"] == "test"
			},
		},
		{
			msg: "no change to cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: &_testCluster,
				})

				cluster := _testCluster.DeepCopy()
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *cluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should still have the cluster
				clusterData := d.get(_testClusterName)
				return clusterData != nil
			},
		},
		{
			msg: "mismatch cluster namespace",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, _ := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      _testClusterName,
					Namespace: "some-other",
				},
			},
			wantErr: "cluster must only belong to the namespace " + constants.ClustersNamespace,
		},
		{
			msg: "err getting cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).Return(assert.AnError)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req:     defaultReq,
			wantErr: assert.AnError.Error(),
		},
		{
			msg: "err updating cluster not under deletion",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				cluster := _testCluster.DeepCopy()
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *cluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req:     defaultReq,
			wantErr: assert.AnError.Error(),
		},
		{
			msg: "err updating cluster under deletion",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{})

				cluster := _testCluster.DeepCopy()
				cluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				cluster.Finalizers = []string{_clusterDeleteFinalizer}
				apiHandler.EXPECT().Get(gomock.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj *v2pb.Cluster) error {
						*obj = *cluster
						return nil
					})
				apiHandler.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req:     defaultReq,
			wantErr: assert.AnError.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			r, testScope := tt.setupFunc(t)
			result, err := r.Reconcile(context.Background(), tt.req)
			if tt.wantErr != "" {
				require.Equal(t, tt.wantErr, err.Error())
				return
			}

			require.NoError(t, err)
			require.Equal(t, ctrl.Result{}, result)
			if tt.assertClusterData != nil {
				require.True(t, tt.assertClusterData(r.clusterDataMap))
			}
			ta := testScope.Snapshot().Counters()
			require.Equal(t, ta["test.cluster.reconcile_count+controller=cluster"].Value(), int64(1))
			require.Equal(t, ta["test.cluster.success_count+controller=cluster"].Value(), int64(1))
		})
	}
}

func TestReconciler_GetCluster(t *testing.T) {
	reconciler := &Reconciler{
		clusterDataMap: &clusterMap{},
	}

	// Test getting non-existent cluster
	cluster := reconciler.GetCluster("non-existent")
	assert.Nil(t, cluster)

	// Add a cluster and test retrieval
	testCluster := _testCluster.DeepCopy()
	reconciler.clusterDataMap.add(_testClusterName, &Data{
		cachedObj: testCluster,
	})

	retrievedCluster := reconciler.GetCluster(_testClusterName)
	assert.NotNil(t, retrievedCluster)
	assert.Equal(t, _testClusterName, retrievedCluster.Name)
}

func TestReconciler_GetClusters(t *testing.T) {
	reconciler := &Reconciler{
		clusterDataMap: &clusterMap{},
	}

	// Test with no clusters
	clusters := reconciler.GetClusters(AllClusters)
	assert.Empty(t, clusters)

	// Add a cluster
	testCluster := _testCluster.DeepCopy()
	reconciler.clusterDataMap.add(_testClusterName, &Data{
		cachedObj: testCluster,
		clusterStatus: &v2pb.ClusterStatus{
			StatusConditions: []*apipb.Condition{
				{
					Type:   constants.ClusterReady,
					Status: apipb.CONDITION_STATUS_TRUE,
				},
			},
		},
	})

	// Test getting all clusters
	allClusters := reconciler.GetClusters(AllClusters)
	assert.Len(t, allClusters, 1)
	assert.Equal(t, _testClusterName, allClusters[0].Name)

	// Test getting ready clusters
	readyClusters := reconciler.GetClusters(ReadyClusters)
	assert.Len(t, readyClusters, 1)

	// Test getting unready clusters
	unreadyClusters := reconciler.GetClusters(UnreadyClusters)
	assert.Empty(t, unreadyClusters)
}

func TestIsClusterSpecSame(t *testing.T) {
	reconciler := &Reconciler{}

	tests := []struct {
		name                     string
		cachedObj                *v2pb.Cluster
		newObj                   *v2pb.Cluster
		expectedNewClusterIsSame bool
	}{
		{
			name: "identical clusters",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: true,
		},
		{
			name: "region changed",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "dca",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "zone changed",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx7",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "DC type changed",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_CLOUD_GCP,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "port changed",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6543",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "host changed",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://different.host.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "SLA got updated",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2pb.DC_TYPE_ON_PREM,
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
					Sla: v2pb.SLA_TYPE_PRODUCTION,
				},
			},
			expectedNewClusterIsSame: false,
		},
		{
			name: "nil kubernetes spec",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Cluster: nil,
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Cluster: nil,
				},
			},
			expectedNewClusterIsSame: true,
		},
		{
			name: "one nil kubernetes spec",
			cachedObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "https://host.docker.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2pb.Cluster{
				Spec: v2pb.ClusterSpec{
					Cluster: nil,
				},
			},
			expectedNewClusterIsSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reconciler.isClusterSpecSame(tt.cachedObj, tt.newObj)
			assert.Equal(t, tt.expectedNewClusterIsSame, result)
		})
	}
}

func TestClusterMap(t *testing.T) {
	cm := &clusterMap{}

	// Test add and get
	testData := &Data{
		cachedObj: _testCluster.DeepCopy(),
	}
	cm.add(_testClusterName, testData)

	retrievedData := cm.get(_testClusterName)
	assert.NotNil(t, retrievedData)
	assert.Equal(t, _testClusterName, retrievedData.cachedObj.Name)

	// Test delete
	cm.delete(_testClusterName)
	retrievedData = cm.get(_testClusterName)
	assert.Nil(t, retrievedData)
}

func TestAddAndDelCluster(t *testing.T) {
	reconciler := &Reconciler{
		clusterDataMap: &clusterMap{},
	}

	testCluster := _testCluster.DeepCopy()

	// Use discard logger for testing
	logger := logr.Discard()

	// Test adding a cluster
	reconciler.addCluster(testCluster, logger)

	// Verify cluster was added
	data := reconciler.clusterDataMap.get(_testClusterName)
	assert.NotNil(t, data)
	assert.Equal(t, _testClusterName, data.cachedObj.Name)

	// Test adding the same cluster again (should skip)
	reconciler.addCluster(testCluster, logger)

	// Test deleting the cluster
	reconciler.delCluster(testCluster)

	// Verify cluster was deleted
	data = reconciler.clusterDataMap.get(_testClusterName)
	assert.Nil(t, data)
}

func TestNewReconciler(t *testing.T) {
	testScope := tally.NewTestScope("test", map[string]string{})
	apiHandlerFactory := handlermocks.NewMockFactory(gomock.NewController(t))

	result := NewReconciler(Params{
		Scope:             testScope,
		ClusterClient:     nil, // Not used in basic tests
		APIHandlerFactory: apiHandlerFactory,
	})

	assert.NotNil(t, result.Reconciler)
	assert.NotNil(t, result.RegisteredClustersCache)
}

func TestSetupWithManager(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T, gCtrl *gomock.Controller) (*Reconciler, ctrl.Manager)
		wantErr   bool
	}{
		{
			name: "success - all runnables added in correct order",
			setupFunc: func(t *testing.T, gCtrl *gomock.Controller) (*Reconciler, ctrl.Manager) {
				// Create a test scheme with v2pb types registered
				scheme := createTestScheme(t)

				// Set up manager and its dependencies with the scheme
				mockClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
					Scheme: scheme,
				})
				require.NoError(t, err)

				// Set up the reconciler with required dependencies
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)
				testScope := tally.NewTestScope("test", map[string]string{})
				zapLogger := zaptest.NewLogger(t)
				apiHandlerFactory := handlermocks.NewMockFactory(gCtrl)

				reconciler := NewReconciler(Params{
					Scope:         testScope,
					ClusterClient: createTestClient(t, factory, helper, zapLogger),
				}).Reconciler

				// Setup Mock Expectations
				apiHandlerFactory.EXPECT().
					GetAPIHandler(gomock.Any()).
					Return(handler.NewFakeAPIHandler(mockClient), nil)

				// Configure the API handler factory
				reconciler.apiHandlerFactory = apiHandlerFactory

				return reconciler, mgr
			},
			wantErr: false,
		},
		{
			name: "error - GetAPIHandler fails",
			setupFunc: func(t *testing.T, gCtrl *gomock.Controller) (*Reconciler, ctrl.Manager) {
				// Create a test scheme with v2pb types registered
				scheme := createTestScheme(t)

				// Set up manager and its dependencies with the scheme
				mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
					Scheme: scheme,
				})
				require.NoError(t, err)

				// Set up the reconciler
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)
				testScope := tally.NewTestScope("test", map[string]string{})
				zapLogger := zaptest.NewLogger(t)

				reconciler := NewReconciler(Params{
					Scope:         testScope,
					ClusterClient: createTestClient(t, factory, helper, zapLogger),
				}).Reconciler

				// Configure the API handler factory to return an error
				mockFailFactory := handlermocks.NewMockFactory(gCtrl)
				mockFailFactory.EXPECT().
					GetAPIHandler(gomock.Any()).
					Return(nil, fmt.Errorf("factory error"))
				reconciler.apiHandlerFactory = mockFailFactory

				return reconciler, mgr
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gCtrl := gomock.NewController(t)
			defer gCtrl.Finish()

			reconciler, mgr := tt.setupFunc(t, gCtrl)
			err := reconciler.SetupWithManager(mgr)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPeriodicallyMonitorCluster(t *testing.T) {
	// Initialize test controller and mocks
	gCtrl := gomock.NewController(t)
	defer gCtrl.Finish()

	factory := computemocks.NewMockFactory(gCtrl)
	helper := clientmocks.NewMockHelper(gCtrl)

	testScope := tally.NewTestScope("test", map[string]string{})
	zapLogger := zaptest.NewLogger(t)
	r := NewReconciler(Params{
		Scope:         testScope,
		ClusterClient: createTestClient(t, factory, helper, zapLogger),
	}).Reconciler

	r.log = zapr.NewLogger(zaptest.NewLogger(t))
	r.Handler = handler.NewFakeAPIHandler(fake.NewClientBuilder().Build())

	// Set up test clusters with proper namespace
	cluster1 := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: constants.ClustersNamespace,
		},
	}
	cluster2 := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: constants.ClustersNamespace,
		},
	}
	healthStatus := &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:   constants.ClusterReady,
				Status: apipb.CONDITION_STATUS_TRUE,
			},
		},
	}

	// Add clusters to the cluster map
	r.clusterDataMap.add(cluster1.Name, &Data{cachedObj: cluster1})
	r.clusterDataMap.add(cluster2.Name, &Data{cachedObj: cluster2})

	// Set up mock expectations
	factory.EXPECT().
		GetClientSetForCluster(gomock.Any()).
		Return(&compute.ClientSet{}, nil).
		Times(2)
	helper.EXPECT().
		GetClusterHealth(gomock.Any(), gomock.Any()).
		Return(healthStatus, nil).
		Times(2)

	// Create a context with cancel to stop the monitoring after a short duration
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start monitoring in a goroutine
	go r.periodicallyMonitorCluster(ctx)

	// Wait for the context to be cancelled
	<-ctx.Done()

	// Give a small grace period for the goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// Verify that cluster statuses were updated
	clusters := r.clusterDataMap.getClustersByFilter(AllClusters)
	for _, cluster := range clusters {
		data := r.clusterDataMap.get(cluster.Name)
		require.NotNil(t, data)
		require.NotNil(t, data.clusterStatus)
		require.Equal(t, constants.ClusterReady, data.clusterStatus.StatusConditions[0].Type)
		require.Equal(t, apipb.CONDITION_STATUS_TRUE, data.clusterStatus.StatusConditions[0].Status)
	}
}

func TestUpdateClusterStatus(t *testing.T) {
	tests := []struct {
		name                string
		setupMock           func(t *testing.T) *Reconciler
		populateClusterData func(r *Reconciler)
		testFunc            func(r *Reconciler)
	}{
		{
			name: "success",
			setupMock: func(t *testing.T) *Reconciler {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				testScope := tally.NewTestScope("test", map[string]string{})
				zapLogger := zaptest.NewLogger(t)
				r := NewReconciler(Params{
					Scope:         testScope,
					ClusterClient: createTestClient(t, factory, helper, zapLogger),
				}).Reconciler

				r.log = zapr.NewLogger(zaptest.NewLogger(t))
				r.Handler = handler.NewFakeAPIHandler(fake.NewClientBuilder().Build())

				// Mock expectations
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).
					Return(&compute.ClientSet{}, nil).AnyTimes()

				clusterHealthStatus := &v2pb.ClusterStatus{
					StatusConditions: []*apipb.Condition{
						{
							Type:   constants.ClusterReady,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
					},
				}
				helper.EXPECT().
					GetClusterHealth(gomock.Any(), gomock.Any()).
					Return(clusterHealthStatus, nil).
					AnyTimes()

				return r
			},
			populateClusterData: func(r *Reconciler) {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
						},
					},
				})
				m.add(_testCluster2Name, &Data{
					cachedObj: &v2pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testCluster2Name,
							Namespace: constants.ClustersNamespace,
						},
					},
				})

				r.clusterDataMap = m
			},
			testFunc: func(r *Reconciler) {
				clusters := r.clusterDataMap.getClustersByFilter(AllClusters)
				for _, c := range clusters {
					require.Equal(t, c.Status.StatusConditions[0].Type, constants.ClusterReady)
					require.Equal(t, c.Status.StatusConditions[0].Status, apipb.CONDITION_STATUS_TRUE)
				}
			},
		},
		{
			name: "success with unhealthy cluster",
			setupMock: func(t *testing.T) *Reconciler {
				gCtrl := gomock.NewController(t)
				factory := computemocks.NewMockFactory(gCtrl)
				helper := clientmocks.NewMockHelper(gCtrl)

				testScope := tally.NewTestScope("test", map[string]string{})
				zapLogger := zaptest.NewLogger(t)
				r := NewReconciler(Params{
					Scope:         testScope,
					ClusterClient: createTestClient(t, factory, helper, zapLogger),
				}).Reconciler

				r.log = zapr.NewLogger(zaptest.NewLogger(t))
				r.Handler = handler.NewFakeAPIHandler(fake.NewClientBuilder().Build())

				// Mock expectations
				factory.EXPECT().GetClientSetForCluster(gomock.Any()).
					Return(&compute.ClientSet{}, nil).AnyTimes()

				clusterHealthStatus := &v2pb.ClusterStatus{
					StatusConditions: []*apipb.Condition{
						{
							Type:   constants.ClusterNotReady,
							Status: apipb.CONDITION_STATUS_TRUE,
						},
					},
				}
				helper.EXPECT().
					GetClusterHealth(gomock.Any(), gomock.Any()).
					Return(clusterHealthStatus, nil).
					AnyTimes()

				return r
			},
			populateClusterData: func(r *Reconciler) {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
						},
					},
				})
				m.add(_testCluster2Name, &Data{
					cachedObj: &v2pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testCluster2Name,
							Namespace: constants.ClustersNamespace,
						},
					},
					clusterStatus: &v2pb.ClusterStatus{
						StatusConditions: []*apipb.Condition{
							{
								Type:   constants.ClusterNotReady,
								Status: apipb.CONDITION_STATUS_TRUE,
							},
						},
					},
				})

				r.clusterDataMap = m
			},
			testFunc: func(r *Reconciler) {
				clusters := r.clusterDataMap.getClustersByFilter(AllClusters)
				for _, c := range clusters {
					require.Equal(t, c.Status.StatusConditions[0].Type, constants.ClusterNotReady)
					require.Equal(t, c.Status.StatusConditions[0].Status, apipb.CONDITION_STATUS_TRUE)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setupMock(t)
			tt.populateClusterData(r)

			ctx := context.Background()

			r.updateClusterInfo(ctx)
			tt.testFunc(r)
		})
	}
}

func TestModule(t *testing.T) {
	// Test that Module is not nil and contains expected providers
	assert.NotNil(t, Module)
}
