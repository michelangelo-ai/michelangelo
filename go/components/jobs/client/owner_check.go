package client

import (
	"context"
	"errors"
	"fmt"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// ErrOwnedByAnotherJob marks a create that found an object of the same name
// owned by another job. Compute-side names drop the global namespace, so
// same-named jobs in two namespaces collide on a compute cluster.
var ErrOwnedByAnotherJob = errors.New("owned by another job")

// ComputeOwner is the owner that a compute-side object records in its
// ma/owner and ma/owner-uid annotations. Both are empty on an object created
// before owners were recorded.
type ComputeOwner struct {
	// Owner is the owning job's <namespace>/<name>.
	Owner string
	// UID is the owning job's UID.
	UID string
}

// ResolveAlreadyExists decides a create that got AlreadyExists, from the
// owner that the existing object records, as read back from the target with
// readErr (RFC 0f). It returns the create's class and, unless that is
// DispatchSuccess, the error to record:
//   - the job's UID, or no owner, as on objects created before owners were
//     recorded: DispatchSuccess, and the job adopts the object;
//   - another job's UID: DispatchPermanent, with ErrOwnedByAnotherJob naming
//     that job. The object is that job's, so nothing retries, re-places or
//     deletes it;
//   - a failed read: DispatchAmbiguous, since the object may be the job's.
//     The create is retried in place, and its next AlreadyExists is checked
//     again;
//   - no object, because it was deleted after the create: DispatchRejected,
//     an answer that created nothing, so the create is retried like any
//     transient failure.
func ResolveAlreadyExists(jobUID string, owner ComputeOwner, readErr error) (DispatchClass, error) {
	switch {
	case apierrors.IsNotFound(readErr):
		return DispatchRejected, fmt.Errorf("an object of the same name was gone when its owner was read: %w", readErr)
	case readErr != nil:
		return DispatchAmbiguous, fmt.Errorf("an object of the same name exists and its owner could not be read: %w", readErr)
	case owner.UID == "" || owner.UID == jobUID:
		return DispatchSuccess, nil
	default:
		return DispatchPermanent, fmt.Errorf(
			"an object of the same name on the compute cluster is %w %q (uid %s); compute-side names omit the namespace, so rename this job",
			ErrOwnedByAnotherJob, owner.Owner, owner.UID)
	}
}

// GetJobOwner reads the owner recorded on the RayJob's compute-side object,
// which resolves a create that got AlreadyExists (ResolveAlreadyExists).
func (c *Client) GetJobOwner(ctx context.Context, jobObject runtime.Object, kubeCluster *v2pb.Cluster) (ComputeOwner, error) {
	if _, ok := jobObject.(*v2pb.RayJob); !ok {
		return ComputeOwner{}, fmt.Errorf("unsupported job type: %T", jobObject)
	}
	return c.getComputeOwner(ctx, jobObject, kubeCluster, constants.KubeRayJobResource, "ray job", &rayv1.RayJob{})
}

// GetJobClusterOwner reads the owner recorded on the RayCluster's
// compute-side object, which resolves a create that got AlreadyExists
// (ResolveAlreadyExists).
func (c *Client) GetJobClusterOwner(ctx context.Context, jobClusterObject runtime.Object, kubeCluster *v2pb.Cluster) (ComputeOwner, error) {
	if _, ok := jobClusterObject.(*v2pb.RayCluster); !ok {
		return ComputeOwner{}, fmt.Errorf("unsupported cluster type: %T", jobClusterObject)
	}
	return c.getComputeOwner(ctx, jobClusterObject, kubeCluster, constants.KubeRayResource, "ray cluster", &rayv1.RayCluster{})
}

// computeObject is a compute-side object read back from a cluster.
type computeObject interface {
	runtime.Object
	GetAnnotations() map[string]string
}

// getComputeOwner reads globalObject's compute-side object, a resource of the
// given kind, into remote and returns the owner it records. It reads as
// DeleteJobClusterIfOwned does, under DispatchRequestTimeout; a missing
// object returns the NotFound error.
func (c *Client) getComputeOwner(ctx context.Context, globalObject runtime.Object, kubeCluster *v2pb.Cluster,
	resource, kind string, remote computeObject,
) (ComputeOwner, error) {
	ctx, cancel := context.WithTimeout(ctx, DispatchRequestTimeout)
	defer cancel()

	cs, err := c.factory.GetClientSetForCluster(kubeCluster)
	if err != nil {
		return ComputeOwner{}, fmt.Errorf("get client for cluster err:%w", markError(err, ErrClusterClientUnavailable))
	}

	localNamespace, localName := c.mapper.GetLocalName(globalObject)
	if err := c.helper.GetResource(ctx, cs.Ray, resource, localNamespace, localName, remote); err != nil {
		c.invalidateOnCredentialError(kubeCluster, err)
		return ComputeOwner{}, fmt.Errorf("get %s err:%w", kind, err)
	}
	annotations := remote.GetAnnotations()
	return ComputeOwner{
		Owner: annotations[constants.OwnerAnnotationKey],
		UID:   annotations[constants.OwnerUIDAnnotationKey],
	}, nil
}
