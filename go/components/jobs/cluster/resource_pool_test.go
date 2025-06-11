package cluster

import (
	"reflect"
	"testing"

	infraCrds "code.uber.internal/infra/compute/k8s-crds/apis/compute.uber.com/v1beta1"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/metrics"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	uownpb "gogoproto/code.uber.internal/infra/uown"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/uberai/michelangelo/shared/gateways/uown/uownmock"
)

func TestGetOwnedResourcePools(t *testing.T) {
	type resPool struct {
		name           string
		owningTeamUUID string
	}

	tt := []struct {
		pools                    []resPool
		queryUUID                string
		parentUUIDChain          []string
		msg                      string
		expectedOwnedPools       map[string]interface{}
		expectedParentOwnedPools map[string]interface{}
		expectedDefaultPools     map[string]interface{}
	}{
		{
			pools: []resPool{
				{
					name:           "pool1",
					owningTeamUUID: "uuid1",
				},
				{
					name:           "pool2",
					owningTeamUUID: "uuid2",
				},
				{
					name:           "pool3",
					owningTeamUUID: "a544c669-dae0-4278-91cd-4c035dec7dd9",
				},
			},
			queryUUID:       "uuid1",
			parentUUIDChain: []string{"uuid2"},
			msg:             "test querying pools for immediate parent pools",
			expectedOwnedPools: map[string]interface{}{
				"pool1": "",
			},
			expectedParentOwnedPools: map[string]interface{}{
				"pool2": "",
			},
			expectedDefaultPools: map[string]interface{}{
				"pool3": "",
			},
		},
		{
			pools: []resPool{
				{
					name:           "pool1",
					owningTeamUUID: "uuid1",
				},
				{
					name:           "pool2",
					owningTeamUUID: "uuid3",
				},
				{
					name:           "pool3",
					owningTeamUUID: "a544c669-dae0-4278-91cd-4c035dec7dd9",
				},
			},
			queryUUID:       "uuid1",
			parentUUIDChain: []string{"uuid2", "uuid3"},
			msg:             "test querying pools for non-immediate parent pools",
			expectedOwnedPools: map[string]interface{}{
				"pool1": "",
			},
			expectedParentOwnedPools: map[string]interface{}{
				"pool2": "",
			},
			expectedDefaultPools: map[string]interface{}{
				"pool3": "",
			},
		},
		{
			pools: []resPool{
				{
					name:           "pool1",
					owningTeamUUID: "uuid1",
				},
				{
					name:           "pool3",
					owningTeamUUID: "a544c669-dae0-4278-91cd-4c035dec7dd9",
				},
			},
			queryUUID:       "uuid1",
			parentUUIDChain: []string{"uuid2"},
			msg:             "no parent pools",
			expectedOwnedPools: map[string]interface{}{
				"pool1": "",
			},
			expectedParentOwnedPools: map[string]interface{}{},
			expectedDefaultPools: map[string]interface{}{
				"pool3": "",
			},
		},
		{
			pools: []resPool{
				{
					name:           "pool2",
					owningTeamUUID: "uuid2",
				},
				{
					name:           "pool3",
					owningTeamUUID: "a544c669-dae0-4278-91cd-4c035dec7dd9",
				},
			},
			queryUUID:          "uuid1",
			parentUUIDChain:    []string{"uuid2"},
			msg:                "no self owned pools",
			expectedOwnedPools: map[string]interface{}{},
			expectedParentOwnedPools: map[string]interface{}{
				"pool2": "",
			},
			expectedDefaultPools: map[string]interface{}{
				"pool3": "",
			},
		},
	}

	cluster := v2beta1pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testCluster",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {

			testScope := tally.NewTestScope("test", map[string]string{})

			cache := resourcePoolCache{
				owned:         uOwnToPoolKeyMap{},
				authorized:    uOwnToPoolKeyMap{},
				resourcePools: map[resourcePoolKey]*ResourcePoolInfo{},
				log:           zap.NewNop(),
				metrics:       &metrics.ControllerMetrics{MetricsScope: testScope},
			}

			// add pools
			for _, pool := range test.pools {
				cache.addOrUpdate(infraCrds.ResourcePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: pool.name,
					},
					Spec: infraCrds.ResourcePoolSpec{
						OwningTeamID: pool.owningTeamUUID,
					},
					Status: infraCrds.ResourcePoolStatus{
						Path:          "/pool/path",
						IsSchedulable: true,
					},
				}, &cluster)
			}

			// setup uOwn mock
			g := gomock.NewController(t)
			uOwn := uownmock.NewMockGateway(g)

			// setup mock response from parent chain
			queryAsset := &uownpb.Asset{
				Uuid: test.queryUUID,
			}

			loopAsset := queryAsset
			for _, parentUUID := range test.parentUUIDChain {
				loopAsset.Parent = &uownpb.Asset{
					Uuid: parentUUID,
				}
				loopAsset = loopAsset.Parent
			}
			loopAsset.Parent = &uownpb.Asset{
				Name: _rootAssetName,
			}

			uOwn.EXPECT().GetUOwnAsset(gomock.Any(), test.queryUUID, false).
				Return(&uownpb.GetAssetResponse{Asset: queryAsset}, nil)
			cache.uOwn = uOwn

			assertionFunc := func(pools []*ResourcePoolInfo, expectedPools map[string]interface{}) {
				poolNames := map[string]interface{}{}
				for _, pool := range pools {
					poolNames[pool.Pool.Name] = ""
					require.Equal(t, cluster.Name, pool.ClusterName)
				}
				require.Equal(t, expectedPools, poolNames)
			}

			ownedPools, err := cache.GetOwnedResourcePools(test.queryUUID)
			require.NoError(t, err)
			assertionFunc(ownedPools, test.expectedOwnedPools)

			parentOwnedPools, err := cache.GetParentOwnedResourcePools(test.queryUUID)
			require.NoError(t, err)
			assertionFunc(parentOwnedPools, test.expectedParentOwnedPools)

			defaultPools, err := cache.GetDefaultResourcePools()
			require.NoError(t, err)
			assertionFunc(defaultPools, test.expectedDefaultPools)
		})
	}
}

func TestIsValidResourcePool(t *testing.T) {
	tt := []struct {
		poolInfo infraCrds.ResourcePool
		valid    bool
		msg      string
	}{
		{
			poolInfo: buildPoolInfo("/UberAI/Default", true),
			valid:    true,
			msg:      "resource pool path present",
		},
		{
			poolInfo: buildPoolInfo("", true),
			valid:    false,
			msg:      "resource pool path not present",
		},
		{
			poolInfo: buildPoolInfo("/UberAI/Default", false),
			valid:    false,
			msg:      "resource pool is not schedulable",
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			testScope := tally.NewTestScope("test", map[string]string{})

			r := resourcePoolCache{
				log:     zap.NewNop(),
				metrics: &metrics.ControllerMetrics{MetricsScope: testScope},
			}

			foundValid := r.isValidResourcePool(test.poolInfo)
			require.Equal(t, test.valid, foundValid)
		})
	}
}

func buildPoolInfo(path string, IsSchedulable bool) infraCrds.ResourcePool {
	return infraCrds.ResourcePool{
		Status: infraCrds.ResourcePoolStatus{
			Path:          path,
			IsSchedulable: IsSchedulable,
		},
	}
}

func TestAddOrUpdate(t *testing.T) {
	tt := []struct {
		msg           string
		beforePool    infraCrds.ResourcePool
		beforeCluster v2beta1pb.Cluster
		afterPool     infraCrds.ResourcePool
		afterCluster  v2beta1pb.Cluster
		assertFunc    func(t *testing.T, cache *resourcePoolCache)
	}{
		{
			msg: "increased cpu and added authorized uown in pool on same cluster",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
					Labels: map[string]string{
						constants.ResourcePoolEnvTest: "false",
					},
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID: "uuid1",
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			afterPool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
					Labels: map[string]string{
						constants.ResourcePoolEnvTest: "false",
					},
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(40, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			afterCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
				require.Equal(t, "cluster1", pools[0].ClusterName)
				require.Equal(t, "pool1", pools[0].Pool.Name)
				require.Equal(t, 1, len(pools[0].Pool.Spec.Resources))
				require.Equal(t, "cpu", pools[0].Pool.Spec.Resources[0].Kind)
				require.Equal(t, *resource.NewQuantity(40, resource.DecimalSI), pools[0].Pool.Spec.Resources[0].Reservation)

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
				require.Equal(t, "cluster1", pools[0].ClusterName)
				require.Equal(t, "pool1", pools[0].Pool.Name)
				require.Equal(t, 1, len(pools[0].Pool.Spec.Resources))
				require.Equal(t, "cpu", pools[0].Pool.Spec.Resources[0].Kind)
				require.Equal(t, *resource.NewQuantity(40, resource.DecimalSI), pools[0].Pool.Spec.Resources[0].Reservation)
			},
		},
		{
			msg: "same pool on another cluster but without the authorized uown",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			afterPool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID: "uuid1",
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(40, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			afterCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster2",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 2, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
			},
		},
		{
			msg: "switch owner and authorized uowns",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			afterPool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid2",
					AuthorizedIdentities: []string{"uuid1"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(40, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			afterCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
			},
		},
		{
			msg: "path not present after update",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			afterPool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID: "uuid1",
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(40, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{},
			},
			afterCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 0, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 0, len(pools))
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			testScope := tally.NewTestScope("test", map[string]string{})

			cache := resourcePoolCache{
				owned:         uOwnToPoolKeyMap{},
				authorized:    uOwnToPoolKeyMap{},
				resourcePools: map[resourcePoolKey]*ResourcePoolInfo{},
				log:           zap.NewNop(),
				metrics:       &metrics.ControllerMetrics{MetricsScope: testScope},
			}

			cache.addOrUpdate(test.beforePool, &test.beforeCluster)
			cache.addOrUpdate(test.afterPool, &test.afterCluster)
			test.assertFunc(t, &cache)
		})
	}
}

func TestDelete(t *testing.T) {
	tt := []struct {
		msg           string
		beforePool    infraCrds.ResourcePool
		beforeCluster v2beta1pb.Cluster
		assertFunc    func(t *testing.T, cache *resourcePoolCache, size int)
	}{
		{
			msg: "Delete pool on given cluster",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache, size int) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, size, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, size, len(pools))
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			testScope := tally.NewTestScope("test", map[string]string{})

			cache := resourcePoolCache{
				owned:         uOwnToPoolKeyMap{},
				authorized:    uOwnToPoolKeyMap{},
				resourcePools: map[resourcePoolKey]*ResourcePoolInfo{},
				log:           zap.NewNop(),
				metrics:       &metrics.ControllerMetrics{MetricsScope: testScope},
			}

			cache.addOrUpdate(test.beforePool, &test.beforeCluster)
			test.assertFunc(t, &cache, 1)
			cache.delete(test.beforePool, &test.beforeCluster)
			test.assertFunc(t, &cache, 0)
		})
	}
}

func TestCleanup(t *testing.T) {
	tt := []struct {
		msg           string
		beforePool    infraCrds.ResourcePool
		newPools      infraCrds.ResourcePoolList
		beforeCluster v2beta1pb.Cluster
		assertFunc    func(t *testing.T, cache *resourcePoolCache)
	}{
		{
			msg: "clean up existing pool which is not active",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			newPools: infraCrds.ResourcePoolList{
				Items: []infraCrds.ResourcePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "fresh-test-pool",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uuid1",
							AuthorizedIdentities: []string{"uuid3"},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path:          "/pool/path",
							IsSchedulable: true,
						},
					},
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				// test owning id pools
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
				require.Equal(t, "fresh-test-pool", pools[0].Pool.Name)

				// test authorized pool is set up after update
				pools, err = cache.GetAuthorizedResourcePools("uuid3")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
				require.Equal(t, "fresh-test-pool", pools[0].Pool.Name)

				// test old authorization is gone after update
				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Empty(t, pools)
			},
		},
		{
			msg: "no ops while clean up",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			newPools: infraCrds.ResourcePoolList{
				Items: []infraCrds.ResourcePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool2",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uuid1",
							AuthorizedIdentities: []string{"uuid2"},
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        "cpu",
									Reservation: *resource.NewQuantity(20, resource.DecimalSI),
								},
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path:          "path2",
							IsSchedulable: true,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool1",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uuid1",
							AuthorizedIdentities: []string{"uuid2"},
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        "cpu",
									Reservation: *resource.NewQuantity(20, resource.DecimalSI),
								},
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path:          "path1",
							IsSchedulable: true,
						},
					},
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 2, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 2, len(pools))
			},
		},
		{
			msg: "owning and authorized uOwns got changed",
			beforePool: infraCrds.ResourcePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pool1",
				},
				Spec: infraCrds.ResourcePoolSpec{
					OwningTeamID:         "uuid1",
					AuthorizedIdentities: []string{"uuid2"},
					Resources: []infraCrds.ResourceConfig{
						{
							Kind:        "cpu",
							Reservation: *resource.NewQuantity(20, resource.DecimalSI),
						},
					},
				},
				Status: infraCrds.ResourcePoolStatus{
					Path:          "path1",
					IsSchedulable: true,
				},
			},
			newPools: infraCrds.ResourcePoolList{
				Items: []infraCrds.ResourcePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool1",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID: "uuid3",
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        "cpu",
									Reservation: *resource.NewQuantity(20, resource.DecimalSI),
								},
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path:          "path1",
							IsSchedulable: true,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool2",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uuid1",
							AuthorizedIdentities: []string{"uuid2"},
							Resources: []infraCrds.ResourceConfig{
								{
									Kind:        "cpu",
									Reservation: *resource.NewQuantity(20, resource.DecimalSI),
								},
							},
						},
						Status: infraCrds.ResourcePoolStatus{
							Path:          "path2",
							IsSchedulable: true,
						},
					},
				},
			},
			beforeCluster: v2beta1pb.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			assertFunc: func(t *testing.T, cache *resourcePoolCache) {
				pools, err := cache.GetOwnedResourcePools("uuid1")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))

				pools, err = cache.GetAuthorizedResourcePools("uuid2")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))

				pools, err = cache.GetOwnedResourcePools("uuid3")
				require.NoError(t, err)
				require.Equal(t, 1, len(pools))
			},
		},
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			testScope := tally.NewTestScope("test", map[string]string{})

			cache := resourcePoolCache{
				owned:         uOwnToPoolKeyMap{},
				authorized:    uOwnToPoolKeyMap{},
				resourcePools: map[resourcePoolKey]*ResourcePoolInfo{},
				log:           zap.NewNop(),
				metrics:       &metrics.ControllerMetrics{MetricsScope: testScope},
			}

			cache.addOrUpdate(test.beforePool, &test.beforeCluster)
			cache.cleanup(&test.beforeCluster, test.newPools)
			for _, pool := range test.newPools.Items {
				cache.addOrUpdate(pool, &test.beforeCluster)
			}
			test.assertFunc(t, &cache)
		})
	}
}

func TestDeletePoolsByCluster(t *testing.T) {
	tests := []struct {
		msg               string
		owned             uOwnToPoolKeyMap
		authorized        uOwnToPoolKeyMap
		resourcePools     map[resourcePoolKey]*ResourcePoolInfo
		clusterToDelete   string
		wantOwned         uOwnToPoolKeyMap
		wantAuthorized    uOwnToPoolKeyMap
		wantResourcePools map[resourcePoolKey]*ResourcePoolInfo
	}{
		{
			msg: "do nothing when no matching pool with cluster",
			owned: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-1"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-2"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			authorized: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-3"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-4"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			resourcePools: map[resourcePoolKey]*ResourcePoolInfo{
				"pool-1-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-1",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-2-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-2",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-3-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-3",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-4-cluster-2": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-4",
						},
					},
					ClusterName: "cluster-2",
				},
			},
			clusterToDelete: "cluster-4",
			wantOwned: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-1"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-2"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			wantAuthorized: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-3"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-4"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			wantResourcePools: map[resourcePoolKey]*ResourcePoolInfo{
				"pool-1-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-1",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-2-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-2",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-3-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-3",
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-4-cluster-2": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-4",
						},
					},
					ClusterName: "cluster-2",
				},
			},
		},
		{
			msg: "delete respools when matching pool with cluster",
			owned: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-1"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-2"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			authorized: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-3"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-4"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
					"pool-4-cluster-2": {},
				},
			},
			resourcePools: map[resourcePoolKey]*ResourcePoolInfo{
				"pool-1-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-1",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-1",
							AuthorizedIdentities: []string{"uOwn-3"},
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-2-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-2",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-1",
							AuthorizedIdentities: []string{"uOwn-3"},
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-3-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-3",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-2",
							AuthorizedIdentities: []string{"uOwn-4"},
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-4-cluster-2": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-4",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-2",
							AuthorizedIdentities: []string{"uOwn-4"},
						},
					},
					ClusterName: "cluster-2",
				},
			},
			clusterToDelete: "cluster-2",
			wantOwned: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-1"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-2"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
				},
			},
			wantAuthorized: map[uOwnUUID]resourcePoolKeys{
				uOwnUUID("uOwn-3"): map[resourcePoolKey]struct{}{
					"pool-1-cluster-1": {},
					"pool-2-cluster-1": {},
				},
				uOwnUUID("uOwn-4"): map[resourcePoolKey]struct{}{
					"pool-3-cluster-1": {},
				},
			},
			wantResourcePools: map[resourcePoolKey]*ResourcePoolInfo{
				"pool-1-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-1",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-1",
							AuthorizedIdentities: []string{"uOwn-3"},
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-2-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-2",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-1",
							AuthorizedIdentities: []string{"uOwn-3"},
						},
					},
					ClusterName: "cluster-1",
				},
				"pool-3-cluster-1": {
					Pool: infraCrds.ResourcePool{
						ObjectMeta: metav1.ObjectMeta{
							Name: "pool-3",
						},
						Spec: infraCrds.ResourcePoolSpec{
							OwningTeamID:         "uOwn-2",
							AuthorizedIdentities: []string{"uOwn-4"},
						},
					},
					ClusterName: "cluster-1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			cache := resourcePoolCache{
				owned:         tt.owned,
				authorized:    tt.authorized,
				resourcePools: tt.resourcePools,
			}
			cluster := &v2beta1pb.Cluster{}
			cluster.SetName(tt.clusterToDelete)

			cache.deletePoolsByCluster(cluster)

			require.True(t, reflect.DeepEqual(cache.owned, tt.wantOwned))
			require.True(t, reflect.DeepEqual(cache.authorized, tt.wantAuthorized))
			require.True(t, reflect.DeepEqual(cache.resourcePools, tt.wantResourcePools))
		})
	}
}
