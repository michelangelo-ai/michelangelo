package client

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ownerCheckHelper serves one compute-side RayCluster and records deletes.
type ownerCheckHelper struct {
	Helper
	remote    *rayv1.RayCluster
	getErr    error
	deleteErr error
	deleted   []string
	deletes   []metav1.DeleteOptions
}

func (h *ownerCheckHelper) GetResource(_ context.Context, _ rest.Interface, resource, namespace, name string, result runtime.Object) error {
	if h.getErr != nil {
		return h.getErr
	}
	if resource != constants.KubeRayResource || namespace != h.remote.Namespace || name != h.remote.Name {
		return apierrors.NewNotFound(rayClustersResource, name)
	}
	h.remote.DeepCopyInto(result.(*rayv1.RayCluster))
	return nil
}

func (h *ownerCheckHelper) DeleteResource(_ context.Context, _ rest.Interface, resource, namespace, name string, opts metav1.DeleteOptions) error {
	h.deleted = append(h.deleted, resource+":"+namespace+"/"+name)
	h.deletes = append(h.deletes, opts)
	return h.deleteErr
}

func TestDeleteJobClusterIfOwned(t *testing.T) {
	owner := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "ns1", UID: "owner-uid"}}
	remote := func(annotations map[string]string) *rayv1.RayCluster {
		return &rayv1.RayCluster{ObjectMeta: metav1.ObjectMeta{
			Name: "rc", Namespace: "default", UID: "remote-uid", Annotations: annotations,
		}}
	}
	ours := map[string]string{constants.OwnerAnnotationKey: "ns1/rc", constants.OwnerUIDAnnotationKey: "owner-uid"}

	tests := []struct {
		name        string
		owner       *v2pb.RayCluster
		helper      *ownerCheckHelper
		invalidate  bool
		wantErr     string
		wantDeleted bool
	}{
		{
			name:   "not found: done",
			owner:  owner,
			helper: &ownerCheckHelper{getErr: apierrors.NewNotFound(rayClustersResource, "rc")},
		},
		{
			// ns2/rc maps to the same compute-side name.
			name:  "foreign owner: left alone",
			owner: owner,
			helper: &ownerCheckHelper{remote: remote(map[string]string{
				constants.OwnerAnnotationKey: "ns2/rc", constants.OwnerUIDAnnotationKey: "other-uid",
			})},
		},
		{
			name:   "legacy object without annotations: left alone",
			owner:  owner,
			helper: &ownerCheckHelper{remote: remote(nil)},
		},
		{
			name:   "owner without a UID never matches",
			owner:  &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "ns1"}},
			helper: &ownerCheckHelper{remote: remote(nil)},
		},
		{
			name:        "ours: deleted with a UID precondition",
			owner:       owner,
			helper:      &ownerCheckHelper{remote: remote(ours)},
			wantDeleted: true,
		},
		{
			name:        "ours, gone before the delete: done",
			owner:       owner,
			helper:      &ownerCheckHelper{remote: remote(ours), deleteErr: apierrors.NewNotFound(rayClustersResource, "rc")},
			wantDeleted: true,
		},
		{
			name:        "ours, delete fails",
			owner:       owner,
			helper:      &ownerCheckHelper{remote: remote(ours), deleteErr: apierrors.NewInternalError(errors.New("etcd"))},
			wantErr:     "delete ray cluster err:Internal error occurred: etcd",
			wantDeleted: true,
		},
		{
			name:    "GET fails",
			owner:   owner,
			helper:  &ownerCheckHelper{getErr: apierrors.NewServiceUnavailable("unavailable")},
			wantErr: "get ray cluster err:unavailable",
		},
		{
			name:       "GET 401 invalidates the client",
			owner:      owner,
			helper:     &ownerCheckHelper{getErr: apierrors.NewUnauthorized("expired")},
			invalidate: true,
			wantErr:    "get ray cluster err:expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := computemocks.NewMockFactory(gomock.NewController(t))
			factory.EXPECT().GetClientSetForCluster(dispatchTestCluster).Return(&compute.ClientSet{}, nil)
			if tt.invalidate {
				factory.EXPECT().Invalidate(&dispatchTestCluster.Spec)
			}
			c := &Client{factory: factory, helper: tt.helper, mapper: permissiveMapper(t), logger: zap.NewNop()}

			err := c.DeleteJobClusterIfOwned(context.Background(), tt.owner, dispatchTestCluster)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if !tt.wantDeleted {
				assert.Empty(t, tt.helper.deleted)
				return
			}
			require.Equal(t, []string{constants.KubeRayResource + ":default/rc"}, tt.helper.deleted)
			require.NotNil(t, tt.helper.deletes[0].Preconditions)
			assert.Equal(t, types.UID("remote-uid"), *tt.helper.deletes[0].Preconditions.UID)
		})
	}
}

func TestDeleteJobClusterIfOwned_ClientUnavailable(t *testing.T) {
	factory := computemocks.NewMockFactory(gomock.NewController(t))
	factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, errors.New("secret not found"))
	c := &Client{factory: factory, helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}

	err := c.DeleteJobClusterIfOwned(context.Background(), &v2pb.RayCluster{}, dispatchTestCluster)

	require.EqualError(t, err, "get client for cluster err:secret not found")
	assert.ErrorIs(t, err, ErrClusterClientUnavailable)
}

func TestDeleteJobClusterIfOwned_UnsupportedType(t *testing.T) {
	c := &Client{helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}
	err := c.DeleteJobClusterIfOwned(context.Background(), &v2pb.RayJob{}, dispatchTestCluster)
	require.EqualError(t, err, "unsupported cluster type: *v2.RayJob")
}
