package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8sError "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestK8sError2GrpError(t *testing.T) {
	type testCase struct {
		k8sError  error
		grpcError codes.Code
	}

	testCases := []testCase{
		{
			k8sError: &k8sError.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonNotFound,
				},
			},
			grpcError: codes.NotFound,
		},
		{
			k8sError: &k8sError.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonRequestEntityTooLarge,
				},
			},
			grpcError: codes.InvalidArgument,
		},
		{
			k8sError:  nil,
			grpcError: codes.OK,
		},
		{
			k8sError:  fmt.Errorf("this is not a k8s status error"),
			grpcError: codes.Unknown,
		},
	}

	for _, test := range testCases {
		err := K8sError2GrpcError(test.k8sError, "test")
		checkGrpcStatusCode(t, test.grpcError, err)
	}
}

func checkGrpcStatusCode(t *testing.T, expectedCode codes.Code, err error) {
	grpcStatus, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, expectedCode, grpcStatus.Code())
}
