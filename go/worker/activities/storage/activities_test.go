package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/yarpc/yarpcerrors"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

var act *activities
var fake *fakeStorage

type Suite struct {
	suite.Suite
	activitySuite types.StarTestActivitySuite
}

func TestITCadence(t *testing.T) {
	suite.Run(t, &Suite{activitySuite: service.NewCadTestActivitySuite()})
}

func TestITTemporal(t *testing.T) {
	suite.Run(t, &Suite{activitySuite: service.NewTempTestActivitySuite()})
}

func (r *Suite) SetupSuite() {
	expected := map[string]string{"key": "value"}
	fake = &fakeStorage{
		scheme: "test",
		readFn: func(ctx context.Context, path string) ([]byte, error) {
			return jsoniter.Marshal(expected)
		},
	}
	blobStore := blobstore.BlobStore{}
	blobStore.RegisterClient(fake)
	act = &activities{
		blobStore: &blobStore,
	}
	r.activitySuite.RegisterActivity(act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {

}

// fakeStorage is a mock implementation of the Storage interface for testing.
type fakeStorage struct {
	scheme string
	readFn func(ctx context.Context, path string) ([]byte, error)
}

// Read calls the fake read function.
func (fs *fakeStorage) Get(ctx context.Context, path string) ([]byte, error) {
	return fs.readFn(ctx, path)
}

// Scheme returns the scheme identifier of the fake storage.
func (fs *fakeStorage) Scheme() string {
	return fs.scheme
}

// TestActivities_Read_Success verifies that activities.Read returns the expected result
// when the Storage implementation successfully reads the data.
func (r *Suite) TestActivities_Read_Success() {
	expected := map[string]string{"key": "value"}
	fake.readFn = func(ctx context.Context, path string) ([]byte, error) {
		return jsoniter.Marshal(expected)
	}
	result, err := r.activitySuite.ExecuteActivity(Activities.Read, "test://dummyPath")

	r.Require().NoError(err)

	var res map[string]string
	result.Get(&res)
	r.Require().Equal(res, expected)
}

// TestActivities_Read_Error verifies that activities.Read properly wraps errors
// returned by the Storage implementation using cadence.CustomError.
func (r *Suite) TestActivities_Read_Error() {
	expectedErr := errors.New("read error")
	fake.readFn = func(ctx context.Context, path string) ([]byte, error) {
		return nil, expectedErr
	}
	_, err := r.activitySuite.ExecuteActivity(Activities.Read, "test://dummyPath")

	// Verify that the returned error message contains the original error message.
	r.Require().Error(err)

	// Verify that the error message includes the YARPC error code.
	yarpcCode := yarpcerrors.FromError(expectedErr).Code().String()
	if !strings.Contains(err.Error(), yarpcCode) {
		r.Require().Fail(fmt.Sprintf("expected error message to contain YARPC code %q, got %q", yarpcCode, err.Error()))
	}
}

// TestActivities_Read_UnsupportedProtocol verifies that activities.Read returns an error
// when an unsupported protocol is provided.
func (r *Suite) TestActivities_Read_UnsupportedProtocol() {
	result, err := r.activitySuite.ExecuteActivity(Activities.Read, "test2://dummyPath")
	if result != nil {
		r.Require().Fail(fmt.Sprintf("expected nil result for unsupported protocol, got %s", result))
	}
	if err == nil {
		r.Require().Fail("expected error for unsupported protocol, got nil")
	}

	// Verify the error message indicates the protocol is not supported.
	expectedMsg := fmt.Sprintf("scheme %s is not supported", "test2")
	r.Require().Contains(err.Error(), expectedMsg)
}
