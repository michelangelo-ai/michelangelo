package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"code.uber.internal/base/testing/contextmatcher"
	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	clusterclient "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute"
	ctypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/types"
	"github.com/go-logr/zapr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/client/clientmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/compute/computemock"
	"mock/github.com/michelangelo-ai/michelangelo/go/api/apimock"
	"mock/sigs.k8s.io/controller-runtime/pkg/manager/managermock"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlRTClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	_testClusterName  = "cluster-1"
	_testCluster2Name = "cluster-2"
)

var _testCluster = v2beta1pb.Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: "michelangelo.uber.com/v2beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      _testClusterName,
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
					Port: "6443",
				},
			},
		},
	},
}

// MockFactory is a mock implementation of apiHandler.Factory
type MockFactory struct {
	handler api.Handler
}

func (m *MockFactory) GetAPIHandler(client ctrlRTClient.Client) (api.Handler, error) {
	return m.handler, nil
}

// MockFactoryWithError is a mock factory that returns an error when GetAPIHandler is called
type MockFactoryWithError struct{}

func (m *MockFactoryWithError) GetAPIHandler(client ctrlRTClient.Client) (api.Handler, error) {
	// Using fmt package to create error message
	return nil, fmt.Errorf("factory error")
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
				r, apiHandler := setupReconciler(t, testParams{})

				cluster := _testCluster.DeepCopy()
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *cluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil)

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
			msg: "delete new cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{})

				cluster := _testCluster.DeepCopy()
				cluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				cluster.Finalizers = []string{_clusterDeleteFinalizer}
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *cluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil)

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
				r, apiHandler := setupReconciler(t, testParams{})

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
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *updatedCluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should include the cluster with the new label
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Labels["sla"] == "test"
			},
		},
		{
			msg: "new test to see if equality works as expected",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				cachedCluster := _testCluster.DeepCopy()
				cachedCluster.ObjectMeta.Finalizers = []string{_clusterDeleteFinalizer}
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: cachedCluster,
				})

				updatedCluster := _testCluster.DeepCopy()
				updatedCluster.ObjectMeta.Finalizers = []string{_clusterDeleteFinalizer}
				updatedCluster.Spec.Sla = v2beta1pb.SLA_TYPE_PRODUCTION

				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *updatedCluster).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should include the cluster with the new label
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Spec.GetSla() == v2beta1pb.SLA_TYPE_PRODUCTION
			},
		},
		{
			msg: "update cluster annotation should not update cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				// add cluster to map with label sla=test
				cachedCluster := _testCluster.DeepCopy()
				cachedCluster.Annotations = map[string]string{
					"sla": "test",
				}
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: cachedCluster,
				})

				updatedCluster := _testCluster.DeepCopy()
				updatedCluster.Annotations = map[string]string{"sla": "production"}
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *updatedCluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should include the cluster with the new label
				clusterData := d.get(_testClusterName)
				return clusterData != nil && clusterData.cachedObj.Annotations["sla"] == "test"
			},
		},
		{
			msg: "no change to update cluster",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{
					cachedObj: &_testCluster,
				})

				cluster := &_testCluster
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *cluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(nil)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req: defaultReq,
			assertClusterData: func(d *clusterMap) bool {
				// map should include the cluster with the new label
				clusterData := d.get(_testClusterName)
				return clusterData != nil
			},
		},
		{
			msg: "mismatch cluster namespace",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, _ := setupReconciler(t, testParams{})

				cluster := _testCluster.DeepCopy()
				cluster.SetNamespace("some-other")

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
				r, apiHandler := setupReconciler(t, testParams{})

				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).Return(assert.AnError)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req:     defaultReq,
			wantErr: assert.AnError.Error(),
		},
		{
			msg: "err updating cluster not under deletion",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				cluster := _testCluster.DeepCopy()
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *cluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)

				return r, r.metrics.MetricsScope.(tally.TestScope)
			},
			req:     defaultReq,
			wantErr: assert.AnError.Error(),
		},
		{
			msg: "err updating cluster under deletion",
			setupFunc: func(t *testing.T) (*Reconciler, tally.TestScope) {
				r, apiHandler := setupReconciler(t, testParams{})

				// add cluster to map
				r.clusterDataMap.add(_testClusterName, &Data{})

				cluster := _testCluster.DeepCopy()
				cluster.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				cluster.Finalizers = []string{_clusterDeleteFinalizer}
				apiHandler.EXPECT().Get(contextmatcher.Any(), constants.ClustersNamespace, _testClusterName,
					gomock.Any(), gomock.Any()).SetArg(4, *cluster).Return(nil)
				apiHandler.EXPECT().Update(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)

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
			require.True(t, tt.assertClusterData(r.clusterDataMap))
			ta := testScope.Snapshot().Counters()
			require.Equal(t, ta["test.cluster.reconcile_count+controller=cluster"].Value(), int64(1))
			require.Equal(t, ta["test.cluster.success_count+controller=cluster"].Value(), int64(1))
		})
	}
}

func TestReconciler_GetCluster(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) *Reconciler
		req       string
		want      *v2beta1pb.Cluster
	}{
		{
			name: "get cluster",
			setupFunc: func(t *testing.T) *Reconciler {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
							Labels: map[string]string{
								"sla": "test",
							},
						},
					},
				})

				k8sClient := fake.NewClientBuilder().Build()
				return &Reconciler{
					Handler:        apiHandler.NewFakeAPIHandler(k8sClient),
					log:            zapr.NewLogger(zaptest.NewLogger(t)),
					clusterDataMap: m,
				}
			},
			want: &v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      _testClusterName,
					Namespace: constants.ClustersNamespace,
					Labels: map[string]string{
						"sla": "test",
					},
				},
			},
			req: _testClusterName,
		},
		{
			name: "blank cluster",
			setupFunc: func(t *testing.T) *Reconciler {
				// add cluster to map with label sla=test
				m := &clusterMap{}

				k8sClient := fake.NewClientBuilder().Build()
				return &Reconciler{
					Handler:        apiHandler.NewFakeAPIHandler(k8sClient),
					log:            zapr.NewLogger(zaptest.NewLogger(t)),
					clusterDataMap: m,
				}
			},
			want: nil,
			req:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setupFunc(t)
			require.Equalf(t, tt.want, r.GetCluster(tt.req), "GetCluster(%v)", tt.req)
		})
	}
}

func TestReconciler_GetClusters(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) *Reconciler
		req       FilterType
		want      []*v2beta1pb.Cluster
	}{
		{
			name: "get clusters",
			setupFunc: func(t *testing.T) *Reconciler {
				// add cluster to map with label sla=test
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
							Labels: map[string]string{
								"sla": "test",
							},
						},
					},
				})

				m.add(_testClusterName+"-test", &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName + "-test",
							Namespace: constants.ClustersNamespace,
							Labels: map[string]string{
								"sla": "test1",
							},
						},
					},
				})

				k8sClient := fake.NewClientBuilder().Build()
				return &Reconciler{
					Handler:        apiHandler.NewFakeAPIHandler(k8sClient),
					log:            zapr.NewLogger(zaptest.NewLogger(t)),
					clusterDataMap: m,
				}
			},
			want: []*v2beta1pb.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      _testClusterName,
						Namespace: constants.ClustersNamespace,
						Labels: map[string]string{
							"sla": "test",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      _testClusterName + "-test",
						Namespace: constants.ClustersNamespace,
						Labels: map[string]string{
							"sla": "test1",
						},
					},
				},
			},
			req: AllClusters,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setupFunc(t)
			require.Equal(t, len(tt.want), len(r.GetClusters(tt.req)))
		})
	}
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
				// Set up manager and its dependencies
				mockClient := fake.NewClientBuilder().Build()
				logger := zapr.NewLogger(zaptest.NewLogger(t))
				mgr := managermock.NewMockManager(gCtrl)

				// Configure basic manager expectations
				mgr.EXPECT().GetLogger().Return(logger).AnyTimes()
				mgr.EXPECT().GetClient().Return(mockClient).AnyTimes()
				mgr.EXPECT().SetFields(gomock.Any()).Return(nil).AnyTimes()

				// Configure expectations for the three runnables that should be added
				// These must be added in a specific order as defined in SetupWithManager
				gomock.InOrder(
					// 1. Leader-only runnable for cluster status monitoring
					mgr.EXPECT().Add(gomock.Any()).Do(func(r interface{}) {
						_, ok := r.(ctypes.LeaderOnlyRunnable)
						require.True(t, ok, "Expected first runnable to be LeaderOnlyRunnable for cluster status monitoring")
					}).Return(nil),

					// 2. Non-leader runnable for resource pools cache updates
					mgr.EXPECT().Add(gomock.Any()).Do(func(r interface{}) {
						_, ok := r.(ctypes.NonLeaderRunnable)
						require.True(t, ok, "Expected second runnable to be NonLeaderRunnable for resource pools cache")
					}).Return(nil),

					// 3. Non-leader reconciler for the controller
					mgr.EXPECT().Add(gomock.Any()).Do(func(r interface{}) {
						_, ok := r.(*ctypes.NonLeaderReconciler)
						require.True(t, ok, "Expected third runnable to be NonLeaderReconciler for the controller")
					}).Return(nil),
				)

				// Set up the reconciler with required dependencies
				factory := computemock.NewMockFactory(gCtrl)
				helper := clientmock.NewMockHelper(gCtrl)

				reconciler, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// Configure the API handler factory
				reconciler.apiHandlerFactory = &MockFactory{handler: apiHandler}

				return reconciler, mgr
			},
			wantErr: false,
		},
		{
			name: "error - GetAPIHandler fails",
			setupFunc: func(t *testing.T, gCtrl *gomock.Controller) (*Reconciler, ctrl.Manager) {
				// Set up manager and its dependencies
				mockClient := fake.NewClientBuilder().Build()
				logger := zapr.NewLogger(zaptest.NewLogger(t))
				mgr := managermock.NewMockManager(gCtrl)

				// Configure basic manager expectations
				mgr.EXPECT().GetLogger().Return(logger).AnyTimes()
				mgr.EXPECT().GetClient().Return(mockClient).AnyTimes()

				// Set up the reconciler
				factory := computemock.NewMockFactory(gCtrl)
				helper := clientmock.NewMockHelper(gCtrl)

				reconciler, _ := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				// Configure the API handler factory to return an error
				mockFailFactory := &MockFactoryWithError{}
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

func TestUpdateClusterStatus(t *testing.T) {
	resourcePools := infraCrds.ResourcePoolList{
		Items: []infraCrds.ResourcePool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pool",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "owningTeamUUID",
					AuthorizedIdentities: []string{"owningTeamUUID2"},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "/pool/path",
					IsSchedulable: true,
				},
			},
		},
	}

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
				factory := computemock.NewMockFactory(gCtrl)
				helper := clientmock.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				factory.EXPECT().GetClientSetForCluster(gomock.Any()).
					Return(&compute.ClientSet{}, nil).AnyTimes()
				helper.EXPECT().GetClusterHealth(contextmatcher.Any(), gomock.Any()).
					Return(&v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterReady,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
							},
						},
					}, nil).AnyTimes()
				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "resourcepools", metav1.NamespaceNone, gomock.Any()).
					SetArg(4, resourcePools).
					Return(nil).AnyTimes()
				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "configmaps", "special-resource-list", gomock.Any()).
					AnyTimes()
				apiHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				return r
			},
			populateClusterData: func(r *Reconciler) {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
						},
					},
				})
				m.add(_testCluster2Name, &Data{
					cachedObj: &v2beta1pb.Cluster{
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
					require.Equal(t, c.Status.StatusConditions[0].Status, v2beta1pb.CONDITION_STATUS_TRUE)
				}

				pools, err := r.resourcePoolsCache.GetOwnedResourcePools("owningTeamUUID")
				require.NoError(t, err)
				require.Equal(t, 2, len(pools))

				pools, err = r.resourcePoolsCache.GetAuthorizedResourcePools("owningTeamUUID2")
				require.NoError(t, err)
				require.Equal(t, 2, len(pools))
			},
		},
		{
			name: "success with unhealthy cluster",
			setupMock: func(t *testing.T) *Reconciler {
				gCtrl := gomock.NewController(t)
				factory := computemock.NewMockFactory(gCtrl)
				helper := clientmock.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				factory.EXPECT().GetClientSetForCluster(gomock.Any()).
					Return(&compute.ClientSet{}, nil).AnyTimes()
				helper.EXPECT().GetClusterHealth(contextmatcher.Any(), gomock.Any()).
					Return(&v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterNotReady,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
							},
						},
					}, nil).AnyTimes()
				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "resourcepools", metav1.NamespaceNone, gomock.Any()).
					SetArg(4, resourcePools).
					Return(nil).AnyTimes()
				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "configmaps", "special-resource-list", gomock.Any()).
					AnyTimes()
				apiHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				return r
			},
			populateClusterData: func(r *Reconciler) {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
						},
					},
				})
				m.add(_testCluster2Name, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testCluster2Name,
							Namespace: constants.ClustersNamespace,
						},
					},
					clusterStatus: &v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterNotReady,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
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
					require.Equal(t, c.Status.StatusConditions[0].Status, v2beta1pb.CONDITION_STATUS_TRUE)
				}

				pools, err := r.resourcePoolsCache.GetOwnedResourcePools("owningTeamUUID")
				require.NoError(t, err)
				require.Equal(t, 0, len(pools))

				pools, err = r.resourcePoolsCache.GetAuthorizedResourcePools("owningTeamUUID2")
				require.NoError(t, err)
				require.Equal(t, 0, len(pools))
			},
		},
		{
			name: "success with spark zonal cluster",
			setupMock: func(t *testing.T) *Reconciler {
				gCtrl := gomock.NewController(t)
				factory := computemock.NewMockFactory(gCtrl)
				helper := clientmock.NewMockHelper(gCtrl)

				r, apiHandler := setupReconciler(t, testParams{
					gCtrl:   gCtrl,
					factory: factory,
					helper:  helper,
				})

				factory.EXPECT().GetClientSetForCluster(gomock.Any()).
					Return(&compute.ClientSet{}, nil).AnyTimes()
				helper.EXPECT().GetClusterHealth(contextmatcher.Any(), gomock.Any()).
					Return(&v2beta1pb.ClusterStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ClusterReady,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
							},
						},
					}, nil).AnyTimes()

				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "resourcepools", metav1.NamespaceNone, gomock.Any()).
					SetArg(4, resourcePools).
					Return(nil)
				helper.EXPECT().ListResources(contextmatcher.Any(), gomock.Any(), "configmaps", "special-resource-list", gomock.Any()).
					AnyTimes()
				apiHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				return r
			},
			populateClusterData: func(r *Reconciler) {
				m := &clusterMap{}
				m.add(_testClusterName, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
							Labels: map[string]string{
								_sparkZonalClusterLabelKey: "true",
							},
						},
					},
				})
				m.add(_testCluster2Name, &Data{
					cachedObj: &v2beta1pb.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      _testClusterName,
							Namespace: constants.ClustersNamespace,
							Labels: map[string]string{
								_sparkZonalClusterLabelKey: "false",
							},
						},
					},
				})

				r.clusterDataMap = m
			},
			testFunc: func(r *Reconciler) {
				clusters := r.clusterDataMap.getClustersByFilter(AllClusters)
				for _, c := range clusters {
					require.Equal(t, c.Status.StatusConditions[0].Type, constants.ClusterReady)
					require.Equal(t, c.Status.StatusConditions[0].Status, v2beta1pb.CONDITION_STATUS_TRUE)
				}

				pools, err := r.resourcePoolsCache.GetOwnedResourcePools("owningTeamUUID")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))

				pools, err = r.resourcePoolsCache.GetAuthorizedResourcePools("owningTeamUUID2")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.setupMock(t)
			tt.populateClusterData(r)

			ctx := context.Background()
			defer ctx.Done()

			r.updateClusterInfo(ctx)
			r.updateResourcePoolsCache(ctx)
			tt.testFunc(r)
		})
	}
}

func TestPeriodicallyMonitorCluster(t *testing.T) {
	// Initialize test controller and mocks
	gCtrl := gomock.NewController(t)
	defer gCtrl.Finish()

	factory := computemock.NewMockFactory(gCtrl)
	helper := clientmock.NewMockHelper(gCtrl)

	r, apiHandler := setupReconciler(t, testParams{
		gCtrl:   gCtrl,
		factory: factory,
		helper:  helper,
	})

	// Set up test clusters with proper namespace
	cluster1 := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: constants.ClustersNamespace,
		},
	}
	cluster2 := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: constants.ClustersNamespace,
		},
	}

	// Add clusters to the cluster map
	r.clusterDataMap.add(cluster1.Name, &Data{cachedObj: cluster1})
	r.clusterDataMap.add(cluster2.Name, &Data{cachedObj: cluster2})

	// Set up health status response
	healthStatus := &v2beta1pb.ClusterStatus{
		StatusConditions: []*v2beta1pb.Condition{
			{
				Type:   constants.ClusterReady,
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
			},
		},
	}

	// Set up mock expectations with gomock.Any() for context
	helper.EXPECT().
		GetClusterHealth(gomock.Any(), gomock.Any()).
		Return(healthStatus, nil).
		Times(2)

	factory.EXPECT().
		GetClientSetForCluster(gomock.Any()).
		Return(&compute.ClientSet{}, nil).
		AnyTimes()

	// Expect UpdateStatus to be called for each cluster
	apiHandler.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
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
		require.Equal(t, v2beta1pb.CONDITION_STATUS_TRUE, data.clusterStatus.StatusConditions[0].Status)
	}
}

func TestPeriodicallyUpdateResourcePoolsCache(t *testing.T) {
	// Initialize test controller and mocks
	gCtrl := gomock.NewController(t)
	defer gCtrl.Finish()

	factory := computemock.NewMockFactory(gCtrl)
	helper := clientmock.NewMockHelper(gCtrl)

	r, _ := setupReconciler(t, testParams{
		gCtrl:   gCtrl,
		factory: factory,
		helper:  helper,
	})

	// Set up test clusters with proper status
	cluster1 := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-1",
			Namespace: constants.ClustersNamespace,
		},
		Status: v2beta1pb.ClusterStatus{
			StatusConditions: []*v2beta1pb.Condition{
				{
					Type:   constants.ClusterReady,
					Status: v2beta1pb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}
	cluster2 := &v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-2",
			Namespace: constants.ClustersNamespace,
		},
		Status: v2beta1pb.ClusterStatus{
			StatusConditions: []*v2beta1pb.Condition{
				{
					Type:   constants.ClusterReady,
					Status: v2beta1pb.CONDITION_STATUS_TRUE,
				},
			},
		},
	}

	// Add clusters to the cluster map with their status
	r.clusterDataMap.add(cluster1.Name, &Data{
		cachedObj:     cluster1,
		clusterStatus: &cluster1.Status,
	})
	r.clusterDataMap.add(cluster2.Name, &Data{
		cachedObj:     cluster2,
		clusterStatus: &cluster2.Status,
	})

	// Set up mock resource pools
	resourcePools := infraCrds.ResourcePoolList{
		Items: []infraCrds.ResourcePool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pool-1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID: "team1",
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "/pool/path",
					IsSchedulable: true,
				},
			},
		},
	}

	// Set up mock client set
	clientSet := &compute.ClientSet{}

	// Expect GetClientSetForCluster to be called multiple times
	factory.EXPECT().
		GetClientSetForCluster(gomock.Any()).
		Return(clientSet, nil).
		AnyTimes()

	// Expect ListResources to be called for resource pools for each cluster
	helper.EXPECT().
		ListResources(gomock.Any(), gomock.Any(), "resourcepools", metav1.NamespaceNone, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ interface{}, _, _ string, out interface{}) error {
			// Copy the resource pools to the output parameter
			outList := out.(*infraCrds.ResourcePoolList)
			*outList = resourcePools
			return nil
		}).
		AnyTimes()

	// Expect ListResources to be called for SKU config maps for each cluster
	helper.EXPECT().
		ListResources(gomock.Any(), gomock.Any(), "configmaps", "special-resource-list", gomock.Any()).
		Return(nil).
		AnyTimes()

	// Create a context with timeout to stop the monitoring after a short duration
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start cache updates in a goroutine
	go r.periodicallyUpdateResourcePoolsCache(ctx)

	// Wait for the context to be cancelled
	<-ctx.Done()

	// Give a small grace period for the goroutine to finish
	time.Sleep(50 * time.Millisecond)

	// Verify that resource pools were cached
	pools, err := r.resourcePoolsCache.GetOwnedResourcePools("team1")
	require.NoError(t, err)
	require.Equal(t, 2, len(pools), "Expected resource pools to be cached for both clusters")
}

type testParams struct {
	gCtrl   *gomock.Controller
	factory compute.Factory
	helper  clusterclient.Helper
}

func setupReconciler(t *testing.T, params testParams) (*Reconciler, *apimock.MockHandler) {
	gCtrl := gomock.NewController(t)
	if params.gCtrl != nil {
		gCtrl = params.gCtrl
	}

	apiHandler := apimock.NewMockHandler(gCtrl)
	testScope := tally.NewTestScope("test", map[string]string{})
	logger := zaptest.NewLogger(t)
	r := NewReconciler(Params{
		ResourcePoolCache: NewResourcePoolCache(ResourcePoolCacheParams{
			Log:   logger,
			Scope: testScope,
		}),
		SkuConfigCache: &skuConfigCache{},
		Scope:          testScope,
		ClusterClient: clusterclient.NewClient(clusterclient.Params{
			Factory: params.factory,
			Helper:  params.helper,
			Logger:  logger,
		}),
	}).Reconciler.(*Reconciler)
	r.log = zapr.NewLogger(zaptest.NewLogger(t))
	r.Handler = apiHandler
	return r, apiHandler
}

func TestIsClusterSpecSame(t *testing.T) {
	tt := []struct {
		cachedObj                *v2beta1pb.Cluster
		newObj                   *v2beta1pb.Cluster
		expectedNewClusterIsSame bool
		msg                      string
	}{
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			expectedNewClusterIsSame: true,
			msg:                      "spec did not change",
		},
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "dca",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			msg: "region changed",
		},
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx7",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			msg: "zone changed",
		},
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			msg: "DC type changed",
		},
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6543",
							},
						},
					},
				},
			},
			msg: "host url changed",
		},
		{
			cachedObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_ON_PREM,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6443",
							},
						},
					},
				},
			},
			newObj: &v2beta1pb.Cluster{
				Spec: v2beta1pb.ClusterSpec{
					Region: "phx",
					Zone:   "phx5",
					Dc:     v2beta1pb.DC_TYPE_CLOUD_GCP,
					Cluster: &v2beta1pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2beta1pb.KubernetesSpec{
							Rest: &v2beta1pb.ConnectionSpec{
								Host: "https://k8s-apiserver-kubernetes-batch01.phx5.uber.internal",
								Port: "6543",
							},
						},
					},
					Sla: v2beta1pb.SLA_TYPE_PRODUCTION,
				},
			},
			msg: "SLA got updated",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			r := Reconciler{}
			require.Equal(t, test.expectedNewClusterIsSame, r.isClusterSpecSame(test.cachedObj, test.newObj))
		})
	}
}
