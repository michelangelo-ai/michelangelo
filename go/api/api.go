package api

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	/////////////////////////// K8s Finalizers ///////////////////////////

	// IngesterFinalizer is used as the pre-delete hook for ingester controller.
	// If Ingester finalizer is presented in a CRD object, ingester shall check if
	// all the pre-delete actions are completed before removing this finalizer.
	IngesterFinalizer = "michelangelo/Ingester"

	/////////////////////////// K8s Annotations ///////////////////////////

	// DeletingAnnotation is used to mark a CRD object is pending on deletion.
	// If this annotation is "true", ingester will delete this CRD in both k8s/ETCD and MySQL.
	DeletingAnnotation = "michelangelo/Deleting"

	// ImmutableAnnotation is used to mark a CRD object if the spec and status of the object
	// will no longer be updated .  If this annotation is set to "true", ingester will remove
	// this CRD ojbect from k8s/ETCD and only the annotation and label of the immutable CRD
	// object can be changed in MySQL later on.
	ImmutableAnnotation = "michelangelo/Immutable"

	/////////////////////////// K8s Labels ///////////////////////////

	// SpecUpdateTimestampLabel is used to record the last time the spec of the object is updated.
	// The time is stored in Unix microseconds.
	SpecUpdateTimestampLabel = "michelangelo/SpecUpdateTimestamp"

	// UpdateTimestampLabel is used to record the last time the object is updated.
	// The time is stored in Unix microseconds.
	UpdateTimestampLabel = "michelangelo/UpdateTimestamp"
)

// DefaultContextTimeout defines the default timeout for the context
const DefaultContextTimeout = 30 * time.Second

// K8sStatusReasonToGrpcError map tries to map the K8s api server's error code to
// gRPC's status error code.  As there is no exact one to one mapping between this
// two error systems, this map translates the mapping with the best effort.
// For unlisted StatusReason, surfaceGrpcError() will translate that into codes.Unknown.
// The api client shall use the detailed error message to determine the unknown error.
var K8sStatusReasonToGrpcError = map[metav1.StatusReason]codes.Code{
	metav1.StatusReasonUnknown:               codes.Unknown,
	metav1.StatusReasonUnauthorized:          codes.Unauthenticated,
	metav1.StatusReasonForbidden:             codes.PermissionDenied,
	metav1.StatusReasonNotFound:              codes.NotFound,
	metav1.StatusReasonAlreadyExists:         codes.AlreadyExists,
	metav1.StatusReasonConflict:              codes.FailedPrecondition,
	metav1.StatusReasonGone:                  codes.NotFound,
	metav1.StatusReasonInvalid:               codes.InvalidArgument,
	metav1.StatusReasonServerTimeout:         codes.DeadlineExceeded,
	metav1.StatusReasonTimeout:               codes.DeadlineExceeded,
	metav1.StatusReasonTooManyRequests:       codes.ResourceExhausted,
	metav1.StatusReasonBadRequest:            codes.InvalidArgument,
	metav1.StatusReasonMethodNotAllowed:      codes.InvalidArgument,
	metav1.StatusReasonNotAcceptable:         codes.InvalidArgument,
	metav1.StatusReasonRequestEntityTooLarge: codes.InvalidArgument,
	metav1.StatusReasonUnsupportedMediaType:  codes.InvalidArgument,
	metav1.StatusReasonInternalError:         codes.Internal,
	metav1.StatusReasonExpired:               codes.NotFound,
	metav1.StatusReasonServiceUnavailable:    codes.Unavailable,
}

// GetGrpcStatusCode translates Kubernetes error to GRPC status code
func GetGrpcStatusCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if statusErr, ok := err.(*apiErrors.StatusError); ok {
		if statusErr == nil {
			return codes.OK
		}

		grpcErrCode, found := K8sStatusReasonToGrpcError[statusErr.Status().Reason]
		if !found {
			grpcErrCode = codes.Unknown
		}
		return grpcErrCode
	}

	return codes.Unknown
}

// K8sError2GrpcError converts K8s error to GRPC error
func K8sError2GrpcError(err error, msg string) error {
	if err == nil {
		return nil
	}
	statusCode := GetGrpcStatusCode(err)

	return status.Errorf(statusCode, "%s: %v", msg, err)
}

type validation interface {
	Validate(prefix string) error
}

// Validate an input message, if the message has Validate(string) error function
func Validate(obj interface{}) error {
	v, ok := obj.(validation)
	if ok {
		return v.Validate("")
	}
	return nil
}
