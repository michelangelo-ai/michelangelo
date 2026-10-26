package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestResolveAlreadyExists(t *testing.T) {
	unavailable := apierrors.NewServiceUnavailable("unavailable")
	gone := fmt.Errorf("get ray cluster err:%w", apierrors.NewNotFound(rayClustersResource, "rc"))
	tests := []struct {
		name    string
		owner   ComputeOwner
		readErr error
		want    DispatchClass
		wantErr string
		wantIs  error
	}{
		{
			name:  "own UID: adopt",
			owner: ComputeOwner{Owner: "ns1/rc", UID: "job-uid"},
			want:  DispatchSuccess,
		},
		{
			name: "no owner, as on objects created before owners were recorded: adopt",
			want: DispatchSuccess,
		},
		{
			// ns2/rc maps to the same compute-side name.
			name:    "another job's UID: permanent, naming that job",
			owner:   ComputeOwner{Owner: "ns2/rc", UID: "other-uid"},
			want:    DispatchPermanent,
			wantErr: `an object of the same name on the compute cluster is owned by another job "ns2/rc" (uid other-uid); compute-side names omit the namespace, so rename this job`,
			wantIs:  ErrOwnedByAnotherJob,
		},
		{
			name:    "read fails: ambiguous",
			readErr: unavailable,
			want:    DispatchAmbiguous,
			wantErr: "an object of the same name exists and its owner could not be read: unavailable",
			wantIs:  unavailable,
		},
		{
			name:    "object gone before the read: rejected, so the create is retried",
			readErr: gone,
			want:    DispatchRejected,
			wantErr: `an object of the same name was gone when its owner was read: get ray cluster err:rayclusters.ray.io "rc" not found`,
			wantIs:  gone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, err := ResolveAlreadyExists("job-uid", tt.owner, tt.readErr)

			assert.Equal(t, tt.want, class)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
			assert.ErrorIs(t, err, tt.wantIs)
		})
	}
}

// ownerReadHelper serves one compute-side object of resource.
type ownerReadHelper struct {
	Helper
	resource string
	remote   runtime.Object
	getErr   error
}

func (h *ownerReadHelper) GetResource(_ context.Context, _ rest.Interface, resource, namespace, name string, result runtime.Object) error {
	if h.getErr != nil {
		return h.getErr
	}
	remote := h.remote.(metav1.Object)
	if resource != h.resource || namespace != remote.GetNamespace() || name != remote.GetName() {
		return apierrors.NewNotFound(rayClustersResource, name)
	}
	switch remote := h.remote.(type) {
	case *rayv1.RayCluster:
		remote.DeepCopyInto(result.(*rayv1.RayCluster))
	case *rayv1.RayJob:
		remote.DeepCopyInto(result.(*rayv1.RayJob))
	}
	return nil
}

func TestGetComputeOwner(t *testing.T) {
	reads := []struct {
		kind     string
		resource string
		remote   func(annotations map[string]string) runtime.Object
		read     func(ctx context.Context, c *Client) (ComputeOwner, error)
	}{
		{
			kind:     "ray cluster",
			resource: constants.KubeRayResource,
			remote: func(annotations map[string]string) runtime.Object {
				return &rayv1.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", Annotations: annotations}}
			},
			read: func(ctx context.Context, c *Client) (ComputeOwner, error) {
				job := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1", UID: "job-uid"}}
				return c.GetJobClusterOwner(ctx, job, dispatchTestCluster)
			},
		},
		{
			kind:     "ray job",
			resource: constants.KubeRayJobResource,
			remote: func(annotations map[string]string) runtime.Object {
				return &rayv1.RayJob{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default", Annotations: annotations}}
			},
			read: func(ctx context.Context, c *Client) (ComputeOwner, error) {
				job := &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1", UID: "job-uid"}}
				return c.GetJobOwner(ctx, job, dispatchTestCluster)
			},
		},
	}
	tests := []struct {
		name         string
		annotations  map[string]string
		getErr       error
		invalidate   bool
		want         ComputeOwner
		wantNotFound bool
	}{
		{
			name:        "the job's",
			annotations: map[string]string{constants.OwnerAnnotationKey: "ns1/foo", constants.OwnerUIDAnnotationKey: "job-uid"},
			want:        ComputeOwner{Owner: "ns1/foo", UID: "job-uid"},
		},
		{
			name:        "another job's",
			annotations: map[string]string{constants.OwnerAnnotationKey: "ns2/foo", constants.OwnerUIDAnnotationKey: "other-uid"},
			want:        ComputeOwner{Owner: "ns2/foo", UID: "other-uid"},
		},
		{
			name: "no owner annotations",
		},
		{
			name:         "gone",
			getErr:       apierrors.NewNotFound(rayClustersResource, "foo"),
			wantNotFound: true,
		},
		{
			name:   "GET fails",
			getErr: apierrors.NewServiceUnavailable("unavailable"),
		},
		{
			name:       "GET 401 invalidates the client",
			getErr:     apierrors.NewUnauthorized("expired"),
			invalidate: true,
		},
	}
	for _, read := range reads {
		for _, tt := range tests {
			t.Run(read.kind+": "+tt.name, func(t *testing.T) {
				factory := computemocks.NewMockFactory(gomock.NewController(t))
				factory.EXPECT().GetClientSetForCluster(dispatchTestCluster).Return(&compute.ClientSet{}, nil)
				if tt.invalidate {
					factory.EXPECT().Invalidate(&dispatchTestCluster.Spec)
				}
				helper := &ownerReadHelper{resource: read.resource, remote: read.remote(tt.annotations), getErr: tt.getErr}
				c := &Client{factory: factory, helper: helper, mapper: permissiveMapper(t), logger: zap.NewNop()}

				owner, err := read.read(context.Background(), c)

				if tt.getErr != nil {
					require.EqualError(t, err, "get "+read.kind+" err:"+tt.getErr.Error())
					assert.Equal(t, tt.wantNotFound, apierrors.IsNotFound(err))
					assert.Equal(t, ComputeOwner{}, owner)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tt.want, owner)
			})
		}
	}
}

func TestGetComputeOwner_ClientUnavailable(t *testing.T) {
	factory := computemocks.NewMockFactory(gomock.NewController(t))
	factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, errors.New("secret not found")).Times(2)
	c := &Client{factory: factory, helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}

	_, err := c.GetJobClusterOwner(context.Background(), &v2pb.RayCluster{}, dispatchTestCluster)
	require.EqualError(t, err, "get client for cluster err:secret not found")
	assert.ErrorIs(t, err, ErrClusterClientUnavailable)

	_, err = c.GetJobOwner(context.Background(), &v2pb.RayJob{}, dispatchTestCluster)
	require.EqualError(t, err, "get client for cluster err:secret not found")
	assert.ErrorIs(t, err, ErrClusterClientUnavailable)
}

func TestGetComputeOwner_UnsupportedType(t *testing.T) {
	c := &Client{helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}

	_, err := c.GetJobClusterOwner(context.Background(), &v2pb.RayJob{}, dispatchTestCluster)
	require.EqualError(t, err, "unsupported cluster type: *v2.RayJob")

	_, err = c.GetJobOwner(context.Background(), &v2pb.RayCluster{}, dispatchTestCluster)
	require.EqualError(t, err, "unsupported job type: *v2.RayCluster")
}
