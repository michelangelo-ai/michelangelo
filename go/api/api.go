// Copyright (c) 2022 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// IngesterFinalizer is used as the pre-delete hook for ingester controller.
	// If Ingester finalizer is presented in a CRD object, ingester shall check if
	// all the pre-delete actions are completed before removing this finalizer.
	IngesterFinalizer = "michelangelo/Ingester"

	// DeletingAnnotation is used to mark a CRD object is pending on deletion.
	// If this annotation is "true", ingester will delete this CRD in both k8s/ETCD and MySQL.
	DeletingAnnotation = "michelangelo/Deleting"

	// ImmutableAnnotation is used to mark a CRD object if the spec and status of the object
	// will no longer be updated .  If this annotation is set to "true", ingester will remove
	// this CRD ojbect from k8s/ETCD and only the annotation and label of the immutable CRD
	// object can be changed in MySQL later on.
	ImmutableAnnotation = "michelangelo/Immutable"
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
	if err != nil {
		if statusErr, ok := err.(*apiErrors.StatusError); ok {
			if statusErr == nil {
				return codes.OK
			}

			grpcErrCode, found := K8sStatusReasonToGrpcError[statusErr.Status().Reason]
			if found == false {
				grpcErrCode = codes.Unknown
			}
			return grpcErrCode
		}

		return codes.Unknown
	}

	return codes.OK
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
