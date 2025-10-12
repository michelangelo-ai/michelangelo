package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"code.uber.internal/base/testing/contextmatcher"
	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta2"
	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/scheduler"
	matypes "code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework"
	sharedConstants "code.uber.internal/uberai/michelangelo/shared/constants"
	"github.com/go-logr/zapr"
	protoTypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/cluster/clustermock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/scheduler/framework/frameworkmock"
	"mock/github.com/michelangelo-ai/michelangelo/go/api/apimock"
	mockctrl "mock/sigs.k8s.io/controller-runtime/controller-runtimemock"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestJobAssignment(t *testing.T) {
	tests := []struct {
		job            framework.BatchJob
		setupMock      func(g *gomock.Controller) mocks
		msg            string
		wantCondition  *v2beta1pb.Condition
		wantAssignment *v2beta1pb.AssignmentInfo
		wantErr        string
	}{
		{
			msg: "explicit preference",
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedConstants.RunnableNameAnnotation: "test-runnable",
						},
					},
					Spec: v2beta1pb.SparkJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										sharedConstants.ClusterAnnotation:          "test-cluster",
										sharedConstants.ResourcePoolPathAnnotation: "test-pool",
										sharedConstants.ClusterTypeAnnotation:      string(matypes.PelotonCluster),
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) mocks {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "spark",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: "Peloton Cluster is specified",
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "test-pool",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "explicit preference region provider",
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedConstants.RunnableNameAnnotation: "test-runnable",
						},
					},
					Spec: v2beta1pb.SparkJobSpec{
						Affinity: &v2beta1pb.Affinity{
							ResourceAffinity: &v2beta1pb.ResourceAffinity{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										sharedConstants.RegionProviderAnnotation:   "test-cluster",
										sharedConstants.ResourcePoolPathAnnotation: "test-pool",
										sharedConstants.ClusterTypeAnnotation:      string(matypes.PelotonCluster),
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) mocks {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "spark",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: "Peloton Cluster is specified",
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "test-pool",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "no pools found",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(
					[]*cluster.ResourcePoolInfo{}, nil)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					poolCache:               poolCache,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_FALSE,
				Reason: constants.NoResourcePoolsFoundInCache,
			},
		},
		{
			msg: "owned pools fit",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).AnyTimes()

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)

				return mocks{
					poolCache:               poolCache,
					clusterCache:            clusterCache,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: constants.ResourcePoolMatchedBasedOnLoad,
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/uberAI/parent/owned",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "authorized pools fit",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				authorizedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					authorizedPools, nil)

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).Times(2)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)

				return mocks{
					poolCache:               poolCache,
					clusterCache:            clusterCache,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: constants.ResourcePoolMatchedBasedOnLoad,
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/uberAI/parent",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "parent owned pools fit",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				parentOwnedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					parentOwnedPools, nil)

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).Times(2)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)

				return mocks{
					poolCache:               poolCache,
					clusterCache:            clusterCache,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: constants.ResourcePoolMatchedBasedOnLoad,
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/uberAI/parent",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "uberAI pools fit",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				uberAIPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(
					uberAIPools, nil)

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).Times(2)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)

				return mocks{
					poolCache:               poolCache,
					clusterCache:            clusterCache,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: constants.ResourcePoolMatchedBasedOnLoad,
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/uberAI",
				Cluster:      "test-cluster",
			},
		},
		{
			msg: "already scheduled",
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-namespace",
						Name:      "test-name",
					},
					Status: v2beta1pb.RayJobStatus{
						StatusConditions: []*v2beta1pb.Condition{
							{
								Type:   constants.ScheduledCondition,
								Status: v2beta1pb.CONDITION_STATUS_TRUE,
							},
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) mocks {
				return mocks{}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
			},
		},
		{
			msg: "err from filter plugin",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)

				filterPlugin := frameworkmock.NewMockFilterPlugin(g)
				filterPlugin.EXPECT().Name().Return("mockFilter").Times(2)
				filterPlugin.EXPECT().Filter(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil, assert.AnError)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					poolCache:               poolCache,
					filterPlugin:            filterPlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantErr: "filter:mockFilter err:" + assert.AnError.Error(),
		},
		{
			msg: "no eligible pools with filter plugin applied - should add attempts metadata",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				authorizedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/authorized",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				parentOwnedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				uberAIPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					authorizedPools, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					parentOwnedPools, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(uberAIPools, nil)

				filterPlugin := frameworkmock.NewMockFilterPlugin(g)
				filterPlugin.EXPECT().Name().Return("mockFilter").Times(8) // 2 calls per filter execution (before and after)
				filterPlugin.EXPECT().Filter(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(4)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					poolCache:               poolCache,
					filterPlugin:            filterPlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: addAttemptsMetadataToCondition(&v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_FALSE,
				Reason: constants.NoResourcePoolMatchedRequirements,
			}, 1),
			wantAssignment: nil,
		},
		{
			msg: "no eligible pools with filter plugin applied - should increment attempts metadata",
			job: framework.BatchRayJob{
				RayJob: addAttemptsMetadataToJob(
					addStatusConditions(
						createRayJob(testCreateRayJobParams{
							head: testResourceParam{
								cpu: 10,
							},
							worker: testResourceParam{
								cpu: 10,
							},
							workerInstances:  1,
							environmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
							owningTeamUOwn:   _testProjectUUID,
						}),
						&v2beta1pb.Condition{
							Status: v2beta1pb.CONDITION_STATUS_FALSE,
							Reason: constants.NoResourcePoolMatchedRequirements,
						}),
					1),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				authorizedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/authorized",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				parentOwnedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				uberAIPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					authorizedPools, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					parentOwnedPools, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(uberAIPools, nil)

				filterPlugin := frameworkmock.NewMockFilterPlugin(g)
				filterPlugin.EXPECT().Name().Return("mockFilter").Times(8) // 2 calls per filter execution (before and after)
				filterPlugin.EXPECT().Filter(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(4)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					poolCache:               poolCache,
					filterPlugin:            filterPlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: addAttemptsMetadataToCondition(&v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_FALSE,
				Reason: constants.NoResourcePoolMatchedRequirements,
			}, 2),
			wantAssignment: nil,
		},
		{
			msg: "no eligible pools with filter plugin applied - UpdateStatus return error",
			job: framework.BatchRayJob{
				RayJob: addAttemptsMetadataToJob(
					addStatusConditions(
						createRayJob(testCreateRayJobParams{
							head: testResourceParam{
								cpu: 10,
							},
							worker: testResourceParam{
								cpu: 10,
							},
							workerInstances:  1,
							environmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
							owningTeamUOwn:   _testProjectUUID,
						}),
						&v2beta1pb.Condition{
							Status: v2beta1pb.CONDITION_STATUS_FALSE,
							Reason: constants.NoResourcePoolMatchedRequirements,
						}),
					1),
			},
			setupMock: func(g *gomock.Controller) mocks {
				mockHandler := apimock.NewMockHandler(g)
				rayJob := addAttemptsMetadataToJob(
					addStatusConditions(
						createRayJob(testCreateRayJobParams{
							head: testResourceParam{
								cpu: 10,
							},
							worker: testResourceParam{
								cpu: 10,
							},
							workerInstances:  1,
							environmentLabel: v2beta1pb.ENV_TYPE_DEVELOPMENT.String(),
							owningTeamUOwn:   _testProjectUUID,
						}),
						&v2beta1pb.Condition{
							Status: v2beta1pb.CONDITION_STATUS_FALSE,
							Reason: constants.NoResourcePoolMatchedRequirements,
						}),
					1)
				mockHandler.EXPECT().Get(contextmatcher.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).SetArg(4, *rayJob)
				mockHandler.EXPECT().UpdateStatus(contextmatcher.Any(), gomock.Any(), gomock.Any()).Return(assert.AnError)
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				authorizedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/authorized",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				parentOwnedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				uberAIPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					authorizedPools, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					parentOwnedPools, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(uberAIPools, nil)

				filterPlugin := frameworkmock.NewMockFilterPlugin(g)
				filterPlugin.EXPECT().Name().Return("mockFilter").Times(8) // 2 calls per filter execution (before and after)
				filterPlugin.EXPECT().Filter(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil).Times(4)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					handler:                 mockHandler,
					poolCache:               poolCache,
					filterPlugin:            filterPlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantErr: assert.AnError.Error(),
		},
		{
			msg: "err from score plugin",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).AnyTimes()

				scorePlugin := frameworkmock.NewMockScorePlugin(g)
				scorePlugin.EXPECT().Name().Return("mockScorer").Times(2)
				scorePlugin.EXPECT().Score(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return(nil, assert.AnError)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)
				return mocks{
					poolCache:               poolCache,
					clusterCache:            clusterCache,
					scorePlugin:             scorePlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantErr: "scoring plugin:mockScorer err:" + assert.AnError.Error(),
		},
		{
			msg: "dr preference",
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test-namespace",
						Annotations: map[string]string{
							sharedConstants.RunnableNameAnnotation: "test-runnable",
						},
					},
				},
			},
			setupMock: func(g *gomock.Controller) mocks {
				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "spark",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).AnyTimes().Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})

				routingConfig := DRRoutingConfig{
					Target:       "dca-gcp",
					ResourcePool: "/test/dr/pool",
				}
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(routingConfig, nil)

				return mocks{
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: "Peloton Cluster is specified",
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/test/dr/pool",
				Cluster:      "dca-gcp",
			},
		},
		{
			msg: "filter results in empty pools - should log warning",
			job: framework.BatchRayJob{
				RayJob: createRayJob(testCreateRayJobParams{
					head: testResourceParam{
						cpu: 10,
					},
					worker: testResourceParam{
						cpu: 10,
					},
					workerInstances:  1,
					environmentLabel: v2beta1pb.ENV_TYPE_PRODUCTION.String(),
					owningTeamUOwn:   _testProjectUUID,
				}),
			},
			setupMock: func(g *gomock.Controller) mocks {
				ownedPools := []*cluster.ResourcePoolInfo{
					createResourcePoolInfo(
						"/uberAI/parent/owned",
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 100,
						},
						testResourceParam{
							cpu: 10,
						}),
				}
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					ownedPools, nil)
				poolCache.EXPECT().GetAuthorizedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetParentOwnedResourcePools(_testProjectUUID).Return(
					[]*cluster.ResourcePoolInfo{}, nil)
				poolCache.EXPECT().GetDefaultResourcePools().Return(
					[]*cluster.ResourcePoolInfo{}, nil)

				// Mock filter that returns empty pools to trigger the logging
				filterPlugin := frameworkmock.NewMockFilterPlugin(g)
				filterPlugin.EXPECT().Name().Return("testFilter").Times(2)
				filterPlugin.EXPECT().Filter(contextmatcher.Any(), gomock.Any(), gomock.Any()).
					Return([]*cluster.ResourcePoolInfo{}, nil)

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)

				constraintsMap := map[string]interface{}{
					_runnableNameKey: "test-runnable",
					_projectNameKey:  "test-namespace",
					_jobTypeKey:      "ray",
				}
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(constraintsMap).Return(flipr.Constraints{
					func(m map[string]interface{}) {
						for k, v := range constraintsMap {
							m[k] = v
						}
					},
				})
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				return mocks{
					poolCache:               poolCache,
					filterPlugin:            filterPlugin,
					flipr:                   mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				}
			},
			wantCondition: addAttemptsMetadataToCondition(&v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_FALSE,
				Reason: constants.NoResourcePoolMatchedRequirements,
			}, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			testScope := tally.NewTestScope("test", map[string]string{})

			testJob := tt.job
			mocks := tt.setupMock(g)
			ctrl := setupTest(t, testParams{
				batchJob:                testJob,
				handler:                 mocks.handler,
				resourcePoolCache:       mocks.poolCache,
				clusterCache:            mocks.clusterCache,
				filterPlugin:            mocks.filterPlugin,
				scorePlugin:             mocks.scorePlugin,
				fliprClient:             mocks.flipr,
				fliprConstraintsBuilder: mocks.fliprConstraintsBuilder,
				testScope:               testScope,
			})

			err := ctrl.assignJobToResourcePool(context.Background(), testJob)

			if tt.wantErr != "" {
				require.Equal(t, tt.wantErr, err.Error())
				return
			}

			require.NoError(t, err)

			switch testJob.(type) {
			case framework.BatchRayJob:
				// retrieve ray object
				var rayJob v2beta1pb.RayJob
				err = ctrl.Get(context.Background(), testJob.GetNamespace(), testJob.GetName(), &metav1.GetOptions{}, &rayJob)
				require.NoError(t, err)

				// retrieve scheduler condition
				actualCondition := utils.GetCondition(&rayJob.Status.StatusConditions, constants.ScheduledCondition, rayJob.Generation)
				require.NotNil(t, actualCondition)
				require.Equal(t, tt.wantCondition.Status, actualCondition.Status)
				require.Equal(t, tt.wantCondition.Reason, actualCondition.Reason)
				require.Equal(t, tt.wantCondition.Metadata, actualCondition.Metadata)
				require.Equal(t, tt.wantAssignment, rayJob.Status.Assignment)
			case framework.BatchSparkJob:
				// retrieve spark object
				var sparkJob v2beta1pb.SparkJob
				err = ctrl.Get(context.Background(), testJob.GetNamespace(), testJob.GetName(), &metav1.GetOptions{}, &sparkJob)
				require.NoError(t, err)

				// retrieve scheduler condition
				actualCondition := utils.GetCondition(&sparkJob.Status.StatusConditions, constants.ScheduledCondition, sparkJob.Generation)
				require.NotNil(t, actualCondition)
				require.Equal(t, tt.wantCondition.Status, actualCondition.Status)
				require.Equal(t, tt.wantCondition.Reason, actualCondition.Reason)
				require.Equal(t, tt.wantCondition.Metadata, actualCondition.Metadata)
				require.Equal(t, tt.wantAssignment, sparkJob.Status.Assignment)
			default:
				require.Fail(t, "unrecognized job type")
			}

		})
	}
}

func TestFetchLatestJob(t *testing.T) {
	tt := []struct {
		job       matypes.SchedulableJob
		wantError bool
		msg       string
	}{
		{
			job: framework.BatchSparkJob{
				SparkJob: &v2beta1pb.SparkJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-spark-ns",
						Name:      "test-spark-job",
					},
				},
			},
			msg: "fetch spark job",
		},
		{
			job: framework.BatchRayJob{
				RayJob: &v2beta1pb.RayJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test-ray-ns",
						Name:      "test-ray-job",
					},
				},
			},
			msg: "fetch ray job",
		},
		{
			job: matypes.NewSchedulableJob(matypes.SchedulableJobParams{
				Name:       "test-name",
				Namespace:  "test-ns",
				Generation: 1,
				JobType:    matypes.JobType(3),
			}),
			wantError: true,
			msg:       "fetch nil job",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := v2beta1pb.AddToScheme(scheme)
			require.NoError(t, err)

			runTimeClient := fake.
				NewClientBuilder().
				WithScheme(scheme).
				Build()

			apiHandler := apiHandler.NewFakeAPIHandler(runTimeClient)
			if batchJob, ok := test.job.(framework.BatchJob); ok {
				err := apiHandler.Create(context.TODO(), batchJob.GetObject(), &metav1.CreateOptions{})
				require.NoError(t, err)
			}

			sc := &Controller{
				Handler: apiHandler,
			}

			var latest framework.BatchJob
			err = sc.fetchLatestJob(context.TODO(), test.job, &latest)
			if test.wantError {
				require.Error(t, err)
				require.Equal(t, fmt.Errorf("unrecognized job type"), err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.job.GetNamespace(), latest.GetNamespace())
				require.Equal(t, test.job.GetName(), latest.GetName())
			}
		})
	}
}

var _testJob = framework.BatchSparkJob{
	SparkJob: &v2beta1pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-job",
		},
		Spec: v2beta1pb.SparkJobSpec{
			Driver: &v2beta1pb.DriverSpec{
				Pod: &v2beta1pb.PodSpec{
					Name: "driver",
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:    10,
						Gpu:    2,
						Memory: "200",
					},
				},
			},
		},
	}}

func TestEnqueue(t *testing.T) {

	testScope := tally.NewTestScope("test", map[string]string{})

	ctrl := setupTest(t, testParams{
		batchJob:  _testJob,
		testScope: testScope,
	})

	ctrl.initLock.Store(true)

	err := ctrl.Enqueue(context.Background(), _testJob)
	require.NoError(t, err)
	ta := testScope.Snapshot().Counters()
	assert.Equal(t, ta["test.scheduler.job.enqueue_success_count+controller=scheduler"].Value(), int64(1))
}

func TestRun(t *testing.T) {
	job := framework.BatchRayJob{
		RayJob: createRayJob(testCreateRayJobParams{
			head: testResourceParam{
				cpu: 10,
			},
			worker: testResourceParam{
				cpu: 10,
			},
			workerInstances: 1,
		}),
	}
	resourcePools := []*cluster.ResourcePoolInfo{
		createResourcePoolInfo(
			"/uberAI/parent/owned",
			testResourceParam{
				cpu: 100,
			},
			testResourceParam{
				cpu: 100,
			},
			testResourceParam{
				cpu: 10,
			}),
	}

	tests := []struct {
		msg            string
		setupMock      func(ctx context.Context, g *gomock.Controller, testScope tally.TestScope) *Controller
		wantCondition  *v2beta1pb.Condition
		wantAssignment *v2beta1pb.AssignmentInfo
		wantMetrics    map[string]int64
	}{
		{
			msg: "success enforced by ctx cancel",
			setupMock: func(ctx context.Context, g *gomock.Controller, testScope tally.TestScope) *Controller {
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					resourcePools, nil)

				rayJob := framework.BatchRayJob{RayJob: job.RayJob.DeepCopy()}
				rayJob.SetLabels(map[string]string{
					sharedConstants.EnvironmentLabel: constants.Production,
					constants.UOwnLabelKey:           _testProjectUUID,
				})

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).AnyTimes()

				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(gomock.Any()).Return(flipr.Constraints{})

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				ctrl := setupTest(t, testParams{
					batchJob:                rayJob,
					resourcePoolCache:       poolCache,
					clusterCache:            clusterCache,
					testScope:               testScope,
					fliprClient:             mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				})

				err := ctrl.internalQueue.Add(ctx, rayJob)
				require.NoError(t, err)

				return ctrl
			},
			wantCondition: &v2beta1pb.Condition{
				Status: v2beta1pb.CONDITION_STATUS_TRUE,
				Reason: constants.ResourcePoolMatchedBasedOnLoad,
			},
			wantAssignment: &v2beta1pb.AssignmentInfo{
				ResourcePool: "/uberAI/parent/owned",
				Cluster:      "test-cluster",
			},
			wantMetrics: map[string]int64{
				"test.scheduler.job_success_count+controller=scheduler,job_type=rayjob": int64(1),
			},
		},
		{
			msg: "err assigning job",
			setupMock: func(ctx context.Context, g *gomock.Controller, testScope tally.TestScope) *Controller {
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					nil, assert.AnError)

				rayJob := framework.BatchRayJob{RayJob: job.RayJob.DeepCopy()}
				rayJob.SetLabels(map[string]string{
					sharedConstants.EnvironmentLabel: constants.Production,
					constants.UOwnLabelKey:           _testProjectUUID,
				})

				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(gomock.Any()).Return(flipr.Constraints{})

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				ctrl := setupTest(t, testParams{
					batchJob:                rayJob,
					resourcePoolCache:       poolCache,
					testScope:               testScope,
					fliprClient:             mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				})

				err := ctrl.internalQueue.Add(ctx, rayJob)
				require.NoError(t, err)

				return ctrl
			},
			wantMetrics: map[string]int64{
				"test.scheduler.failed_count+controller=scheduler,failure_reason=error_fetching_resource_pools":                         int64(1),
				"test.scheduler.job_failed_count+controller=scheduler,failure_reason=assign_job_to_resource_pool_error,job_type=rayjob": int64(1),
			},
		},
		{
			msg: "err updating job status in resource pool assignment",
			setupMock: func(ctx context.Context, g *gomock.Controller, testScope tally.TestScope) *Controller {
				poolCache := clustermock.NewMockResourcePoolCache(g)
				poolCache.EXPECT().GetOwnedResourcePools(_testProjectUUID).Return(
					resourcePools, nil)

				rayJob := framework.BatchRayJob{RayJob: job.RayJob.DeepCopy()}
				rayJob.SetLabels(map[string]string{
					sharedConstants.EnvironmentLabel: constants.Production,
					constants.UOwnLabelKey:           _testProjectUUID,
				})

				clusterCache := clustermock.NewMockRegisteredClustersCache(g)
				clusterCache.EXPECT().GetCluster("test-cluster").Return(&v2beta1pb.Cluster{
					Spec: v2beta1pb.ClusterSpec{
						Region: "phx",
					},
				}).AnyTimes()

				mockFlipr := fliprmock.NewMockFliprClient(g)
				mockFlipr.EXPECT().GetStringValue(gomock.Any(), "rayJobsInCloud", gomock.Any(), "").
					Return("", nil)
				mockFlipr.EXPECT().GetValueWithConstraints(gomock.Any(), _fliprDRRoutingKey, gomock.Any()).Return(nil, nil)

				mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(g)
				mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(gomock.Any()).Return(flipr.Constraints{})

				ctrl := setupTest(t, testParams{
					batchJob:                rayJob,
					testScope:               testScope,
					resourcePoolCache:       poolCache,
					clusterCache:            clusterCache,
					fliprClient:             mockFlipr,
					fliprConstraintsBuilder: mockFliprConstraintsBuilder,
				})

				mockHandler := apimock.NewMockHandler(g)
				mockHandler.EXPECT().Get(gomock.Any(), rayJob.GetNamespace(), rayJob.GetName(), gomock.Any(), gomock.Any()).
					SetArg(4, *rayJob.RayJob).Return(nil).AnyTimes()

				statusError := &apiErrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReasonConflict,
					},
				}
				mockHandler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).
					// this mirrors how APIHAndler wraps the K8s status error
					// https://sourcegraph.uberinternal.com/code.uber.internal/go-code/-/blob/src/code.uber.internal/uberai/michelangelo/apiserver/internal/api/api.go#L610:17
					Return(status.Errorf(codes.FailedPrecondition, "failed to %v API object. namespace: %v, name: %v. %v",
						"updateStatus", rayJob.GetNamespace(), rayJob.GetName(), statusError.Error())).AnyTimes()
				ctrl.Handler = mockHandler

				err := ctrl.internalQueue.Add(ctx, rayJob)
				require.NoError(t, err)

				return ctrl
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			testScope := tally.NewTestScope("test", map[string]string{})

			ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelFn()

			ctrl := tt.setupMock(ctx, g, testScope)

			// run go routine to check expected condition and terminate scheduler loop by context cancellation.
			go require.Eventually(t, func() bool {
				var rayJob v2beta1pb.RayJob
				err := ctrl.Get(context.Background(), job.GetNamespace(), job.GetName(), &metav1.GetOptions{}, &rayJob)
				require.NoError(t, err)

				if tt.wantCondition != nil {
					// retrieve scheduler condition
					actualCondition := utils.GetCondition(&rayJob.Status.StatusConditions, constants.ScheduledCondition, rayJob.Generation)
					require.NotNil(t, actualCondition)
					require.Equal(t, tt.wantCondition.Status, actualCondition.Status)
					require.Equal(t, tt.wantCondition.Reason, actualCondition.Reason)
				}

				if tt.wantAssignment != nil {
					require.Equal(t, tt.wantAssignment, rayJob.Status.Assignment)
				}

				if tt.wantMetrics != nil {
					ta := testScope.Snapshot().Counters()
					require.Equal(t, len(tt.wantMetrics), len(ta))
					for k, v := range ta {
						val, ok := tt.wantMetrics[k]
						require.True(t, ok)
						require.Equal(t, val, v.Value())
					}
				}

				// cancel context to terminate scheduler loop
				cancelFn()

				return true
			}, time.Second*5, time.Millisecond*200)

			err := ctrl.run(ctx)
			require.Equal(t, context.Canceled, err)

			// check that the job is not in the queue
			require.Equal(t, 0, ctrl.internalQueue.Length())
		})
	}
}

func TestHandlePanic(t *testing.T) {
	testScope := tally.NewTestScope("test", map[string]string{})

	g := gomock.NewController(t)
	mockMgr := mockctrl.NewMockManager(g)
	mockMgr.EXPECT().Elected().DoAndReturn(func() <-chan struct{} {
		ch := make(chan struct{}, 1)
		ch <- struct{}{}
		return ch
	}).Times(1)

	ctrl := setupTest(t, testParams{
		batchJob:  _testJob,
		testScope: testScope,
	})
	ctrl.mgr = mockMgr
	ctrl.scheduleFunc = func(ctx context.Context) error {
		return fmt.Errorf("an error")
	}

	errorChan := ctrl.init()
	val := <-errorChan // wait for error

	err, ok := val.(error)
	require.True(t, ok)
	require.EqualError(t, err, "an error")

	// expected metrics
	wantMetrics := map[string]int64{
		"test.scheduler.loop_exited_count+controller=scheduler": int64(1),
	}

	ta := testScope.Snapshot().Counters()
	require.NotEmpty(t, ta)
	for k, v := range ta {
		val, ok := wantMetrics[k]
		require.True(t, ok)
		require.Equal(t, val, v.Value())
	}
}

func TestUpdateIfChanged(t *testing.T) {
	tt := []struct {
		condition    *v2beta1pb.Condition
		updateParams utils.ConditionUpdateParams
		shouldUpdate bool
		msg          string
	}{
		{
			condition: &v2beta1pb.Condition{
				Status:               v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:               constants.NoResourcePoolMatchedRequirements,
				ObservedGeneration:   1,
				LastUpdatedTimestamp: time.Now().Add(-time.Minute).Unix(),
			},
			updateParams: utils.ConditionUpdateParams{
				Status:     v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:     constants.NoResourcePoolsFoundInCache,
				Generation: 1,
			},
			shouldUpdate: true,
			msg:          "reason changed - should change condition",
		},
		{
			condition: &v2beta1pb.Condition{
				Status:               v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:               constants.NoResourcePoolMatchedRequirements,
				ObservedGeneration:   1,
				LastUpdatedTimestamp: time.Now().Add(-time.Minute).Unix(),
			},
			updateParams: utils.ConditionUpdateParams{
				Status:     v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:     constants.NoResourcePoolsFoundInCache,
				Generation: 2,
			},
			shouldUpdate: true,
			msg:          "generation changed - should change condition",
		},
		{
			condition: &v2beta1pb.Condition{
				Status:               v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:               constants.NoResourcePoolMatchedRequirements,
				ObservedGeneration:   1,
				LastUpdatedTimestamp: time.Now().Add(-time.Minute).Unix(),
			},
			updateParams: utils.ConditionUpdateParams{
				Status:     v2beta1pb.CONDITION_STATUS_TRUE,
				Reason:     constants.ResourcePoolMatchedBasedOnLoad,
				Generation: 1,
			},
			shouldUpdate: true,
			msg:          "status changed - should change condition",
		},
		{
			condition: &v2beta1pb.Condition{
				Status:               v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:               constants.NoResourcePoolMatchedRequirements,
				ObservedGeneration:   1,
				LastUpdatedTimestamp: time.Now().Add(-time.Minute).Unix(),
			},
			updateParams: utils.ConditionUpdateParams{
				Status:     v2beta1pb.CONDITION_STATUS_FALSE,
				Reason:     constants.NoResourcePoolMatchedRequirements,
				Generation: 1,
			},
			msg: "did not change",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			c := Controller{}
			conditionLastUpdated := test.condition.GetLastUpdatedTimestamp()
			c.updateIfChanged(test.condition, test.updateParams)
			if test.shouldUpdate {
				require.True(t, test.condition.GetLastUpdatedTimestamp() > conditionLastUpdated)
			} else {
				require.Equal(t, test.condition.GetLastUpdatedTimestamp(), conditionLastUpdated)
			}
		})
	}
}

// Test Helpers
// -------------

var _testProjectUUID = "uuid1"

// Define mocks struct at the top level
type mocks struct {
	handler                 api.Handler
	poolCache               cluster.ResourcePoolCache
	clusterCache            cluster.RegisteredClustersCache
	filterPlugin            framework.FilterPlugin
	scorePlugin             framework.ScorePlugin
	flipr                   flipr.FliprClient
	fliprConstraintsBuilder *typesmock.MockFliprConstraintsBuilder
}

func setupTest(
	t *testing.T,
	params testParams) *Controller {
	logger := zapr.NewLoggerWithOptions(zaptest.NewLogger(t), zapr.AllowZapFields(true))

	scheme := runtime.NewScheme()
	err := v2beta1pb.AddToScheme(scheme)
	require.NoError(t, err)

	project := v2beta1pb.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.batchJob.GetNamespace(),
			Namespace: params.batchJob.GetNamespace(),
		},
		Spec: v2beta1pb.ProjectSpec{
			Owner: &v2beta1pb.OwnerInfo{
				OwningTeam: _testProjectUUID,
			},
		},
	}

	runTimeClient := fake.
		NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(params.batchJob.GetObject(), &project).
		WithStatusSubresource(&v2beta1pb.RayJob{}, &v2beta1pb.SparkJob{}).
		Build()

	handler := params.handler
	if handler == nil {
		handler = apiHandler.NewFakeAPIHandler(runTimeClient)
	}

	ctrl := newController(Params{
		OptionBuilder:           framework.NewOptionBuilder(),
		Scope:                   params.testScope,
		ClusterCache:            params.clusterCache,
		FliprClient:             params.fliprClient,
		FliprConstraintsBuilder: params.fliprConstraintsBuilder,
	}, logger, handler)

	ctrl.internalQueue = scheduler.New().Queue
	ctrl.resourcePoolCache = params.resourcePoolCache
	if params.filterPlugin != nil {
		ctrl.filterPlugins = []framework.FilterPlugin{params.filterPlugin}
	}
	if params.scorePlugin != nil {
		ctrl.scorePlugins = []framework.ScorePlugin{params.scorePlugin}
	}

	return ctrl
}

type testCreateRayJobParams struct {
	head             testResourceParam
	worker           testResourceParam
	workerInstances  int
	environmentLabel string
	owningTeamUOwn   string
}

type testParams struct {
	batchJob                framework.BatchJob
	handler                 api.Handler
	resourcePoolCache       cluster.ResourcePoolCache
	clusterCache            cluster.RegisteredClustersCache
	filterPlugin            framework.FilterPlugin
	scorePlugin             framework.ScorePlugin
	fliprClient             flipr.FliprClient
	fliprConstraintsBuilder matypes.FliprConstraintsBuilder
	testScope               tally.Scope
}

type testResourceParam struct {
	cpu    int64
	memory int64 // scale by 10^9
	gpu    int64
	disk   int64
}

func createRayJob(p testCreateRayJobParams) *v2beta1pb.RayJob {
	return &v2beta1pb.RayJob{
		TypeMeta: metav1.TypeMeta{
			Kind: "RayJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-name",
			Labels: map[string]string{
				sharedConstants.EnvironmentLabel: p.environmentLabel,
				constants.UOwnLabelKey:           p.owningTeamUOwn,
			},
			Annotations: map[string]string{
				sharedConstants.RunnableNameAnnotation: "test-runnable",
			},
		},
		Spec: v2beta1pb.RayJobSpec{
			Head: &v2beta1pb.HeadSpec{
				Pod: &v2beta1pb.PodSpec{
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:      int32(p.head.cpu),
						Memory:   fmt.Sprintf("%vG", p.head.memory),
						Gpu:      int32(p.head.gpu),
						DiskSize: fmt.Sprintf("%vG", p.head.disk),
					},
				},
			},
			Worker: &v2beta1pb.WorkerSpec{
				MinInstances: int32(p.workerInstances),
				Pod: &v2beta1pb.PodSpec{
					Resource: &v2beta1pb.ResourceSpec{
						Cpu:      int32(p.worker.cpu),
						Memory:   fmt.Sprintf("%vG", p.worker.memory),
						Gpu:      int32(p.worker.gpu),
						DiskSize: fmt.Sprintf("%vG", p.worker.disk),
					},
				},
			},
		},
	}
}

func addStatusConditions(job *v2beta1pb.RayJob, conditions ...*v2beta1pb.Condition) *v2beta1pb.RayJob {
	job.Status.StatusConditions = conditions
	return job
}

func addAttemptsMetadataToJob(job *v2beta1pb.RayJob, counter int) *v2beta1pb.RayJob {
	scheduled := utils.GetCondition(&job.Status.StatusConditions, constants.ScheduledCondition, job.Generation)
	_ = addAttemptsMetadataToCondition(scheduled, counter)
	return job
}

func addAttemptsMetadataToCondition(condition *v2beta1pb.Condition, counter int) *v2beta1pb.Condition {
	metadata, _ := generateAttemptsMetadata(counter)
	condition.Metadata = metadata
	return condition
}

func generateAttemptsMetadata(counter int) (*protoTypes.Any, error) {
	metaFields := map[string]*protoTypes.Value{
		constants.NumSchedulerAttempts: {Kind: &protoTypes.Value_NumberValue{NumberValue: float64(counter)}},
	}
	metadata, err := protoTypes.MarshalAny(&protoTypes.Struct{Fields: metaFields})
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func createRequirements(p testResourceParam) corev1.ResourceList {
	req := make(corev1.ResourceList)

	if p.cpu > 0 {
		req[corev1.ResourceCPU] = *resource.NewQuantity(p.cpu, resource.DecimalSI)
	}
	if p.memory > 0 {
		req[corev1.ResourceMemory] = *resource.NewScaledQuantity(p.memory, 9)
	}
	if p.gpu > 0 {
		req[constants.ResourceNvidiaGPU] = *resource.NewQuantity(p.gpu, resource.DecimalSI)
	}
	if p.disk > 0 {
		req[corev1.ResourceEphemeralStorage] = *resource.NewQuantity(p.disk, resource.DecimalSI)
	}
	return req
}

func createResourcePoolInfo(path string, reservation, limit, usage testResourceParam) *cluster.ResourcePoolInfo {
	pool := &cluster.ResourcePoolInfo{}

	if reservation.cpu > 0 || limit.cpu > 0 {
		pool.Pool.Spec.Resources = append(pool.Pool.Spec.Resources, infraCrds.ResourceConfig{
			Kind:        corev1.ResourceCPU.String(),
			Reservation: *resource.NewQuantity(reservation.cpu, resource.DecimalSI),
			Limit:       *resource.NewQuantity(limit.cpu, resource.DecimalSI),
		})
	}
	if reservation.memory > 0 || limit.memory > 0 {
		pool.Pool.Spec.Resources = append(pool.Pool.Spec.Resources, infraCrds.ResourceConfig{
			Kind:        corev1.ResourceMemory.String(),
			Reservation: *resource.NewScaledQuantity(reservation.memory, 9),
			Limit:       *resource.NewScaledQuantity(limit.memory, 9),
		})
	}
	if reservation.gpu > 0 || limit.gpu > 0 {
		pool.Pool.Spec.Resources = append(pool.Pool.Spec.Resources, infraCrds.ResourceConfig{
			Kind:        constants.ResourceNvidiaGPU.String(),
			Reservation: *resource.NewQuantity(reservation.gpu, resource.DecimalSI),
			Limit:       *resource.NewQuantity(limit.gpu, resource.DecimalSI),
		})
	}
	if reservation.disk > 0 || limit.disk > 0 {
		pool.Pool.Spec.Resources = append(pool.Pool.Spec.Resources, infraCrds.ResourceConfig{
			Kind:        corev1.ResourceEphemeralStorage.String(),
			Reservation: *resource.NewQuantity(reservation.disk, resource.DecimalSI),
			Limit:       *resource.NewQuantity(limit.disk, resource.DecimalSI),
		})
	}

	pool.Pool.Status.Usage = createRequirements(usage)
	pool.Pool.Status.Path = path
	pool.Pool.Name = path
	pool.ClusterName = "test-cluster"

	pool.Pool.Labels = map[string]string{
		constants.ResourcePoolEnvProd: "true",
		constants.ResourcePoolEnvDev:  "false",
		constants.ResourcePoolEnvTest: "false",
	}

	return pool
}
