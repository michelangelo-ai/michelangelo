package common

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// s3Scheme is the only artifact URI scheme the model-sync sidecar understands. The
// sidecar shells out to the AWS CLI, so gs:// and abfss:// URIs would only fail once
// the sync ran; rejecting them here surfaces the problem on the Deployment instead.
const s3Scheme = "s3://"

// Condition reasons reported when a Deployment's model cannot be resolved to a
// storage path. These are surfaced verbatim on the AssetsPrepared condition.
const (
	ReasonModelNotFound             = "ModelNotFound"
	ReasonModelPackageTypeMismatch  = "ModelPackageTypeMismatch"
	ReasonNoDeployableArtifact      = "NoDeployableArtifact"
	ReasonUnsupportedArtifactScheme = "UnsupportedArtifactScheme"
)

// ModelResolutionError reports that a Deployment's desired model could not be resolved
// to a usable storage path. Reason is a stable identifier suitable for a condition
// reason; callers that do not recognise it should fall back to a generic reason.
type ModelResolutionError struct {
	Reason  string
	Message string
}

// Error implements the error interface.
func (e *ModelResolutionError) Error() string {
	return e.Message
}

// FetchModel loads the Model named by the Deployment's desired revision. The reference's
// namespace is optional and defaults to the Deployment's own namespace, matching how the
// Deployment's other references are resolved.
//
// The read goes through the API handler rather than a Kubernetes client because Model is
// an immutable kind: the ingester deletes immutable objects from etcd once they have been
// written to metadata storage, so a direct etcd read would report them as missing.
func FetchModel(ctx context.Context, apiHandler goapi.Handler, deployment *v2pb.Deployment) (*v2pb.Model, error) {
	ref := deployment.Spec.GetDesiredRevision()
	if ref.GetName() == "" {
		return nil, &ModelResolutionError{
			Reason:  ReasonModelNotFound,
			Message: fmt.Sprintf("deployment %s/%s has no desired revision name", deployment.Namespace, deployment.Name),
		}
	}

	namespace := ref.GetNamespace()
	if namespace == "" {
		namespace = deployment.Namespace
	}

	model := &v2pb.Model{}
	if err := apiHandler.Get(ctx, namespace, ref.GetName(), &metav1.GetOptions{}, model); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, &ModelResolutionError{
				Reason:  ReasonModelNotFound,
				Message: fmt.Sprintf("model %s/%s not found", namespace, ref.GetName()),
			}
		}
		return nil, fmt.Errorf("get model %s/%s: %w", namespace, ref.GetName(), err)
	}
	return model, nil
}

// ResolveModelStoragePath returns the storage prefix holding the model's Triton-packaged
// artifacts. The result always ends in a slash: the sidecar syncs a prefix rather than a
// single object, so a bare object key would sync nothing.
func ResolveModelStoragePath(model *v2pb.Model) (string, error) {
	// The registration path does not populate package_type yet, so an unset value is
	// taken to mean Triton. An explicitly different type is still a hard error, which
	// keeps a Spark or mobile package from being handed to a Triton server.
	packageType := model.Spec.GetPackageType()
	if packageType != v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON &&
		packageType != v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_INVALID {
		return "", &ModelResolutionError{
			Reason: ReasonModelPackageTypeMismatch,
			Message: fmt.Sprintf("model %s/%s has package type %s, want %s",
				model.Namespace, model.Name, packageType, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON),
		}
	}

	uris := model.Spec.GetDeployableArtifactUri()
	if len(uris) == 0 {
		return "", &ModelResolutionError{
			Reason:  ReasonNoDeployableArtifact,
			Message: fmt.Sprintf("model %s/%s has no deployable artifact URI", model.Namespace, model.Name),
		}
	}

	uri := uris[0]
	if !strings.HasPrefix(uri, s3Scheme) {
		return "", &ModelResolutionError{
			Reason: ReasonUnsupportedArtifactScheme,
			Message: fmt.Sprintf("model %s/%s artifact URI %q is not supported, only %s is",
				model.Namespace, model.Name, uri, s3Scheme),
		}
	}
	// A tar artifact is a single object that the sidecar downloads and unpacks, so it must
	// keep its exact key. Everything else is a prefix, and the trailing slash is what marks
	// it as one for the sidecar's recursive sync.
	if !isTarArtifact(uri) && !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	return uri, nil
}

// isTarArtifact reports whether the URI points at a tar archive rather than a prefix.
// The packaging step archives directory artifacts before upload, so Triton models pushed
// by a pipeline arrive as a single .tar object.
func isTarArtifact(uri string) bool {
	return strings.HasSuffix(uri, ".tar")
}

// ResolveDeploymentModelStoragePath fetches the Deployment's Model and resolves the
// storage prefix its artifacts live under.
func ResolveDeploymentModelStoragePath(ctx context.Context, apiHandler goapi.Handler, deployment *v2pb.Deployment) (string, error) {
	model, err := FetchModel(ctx, apiHandler, deployment)
	if err != nil {
		return "", err
	}
	return ResolveModelStoragePath(model)
}
