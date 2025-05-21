package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/yarpc/yarpcerrors"

	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"
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
	expected := "hello world"
	fake = &fakeStorage{
		protocol: "test",
		readFn: func(ctx context.Context, path string) (any, error) {
			return expected, nil
		},
	}
	act = &activities{
		impls: map[string]intf.Storage{
			"test": fake,
		},
	}
	r.activitySuite.RegisterActivity(act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {

}

// fakeStorage is a mock implementation of the Storage interface for testing.
type fakeStorage struct {
	protocol   string
	readFn     func(ctx context.Context, path string) (any, error)
	isNotFound bool
}

// Read calls the fake read function.
func (fs *fakeStorage) Read(ctx context.Context, path string) (any, error) {
	return fs.readFn(ctx, path)
}

// Protocol returns the protocol identifier of the fake storage.
func (fs *fakeStorage) Protocol() string {
	return fs.protocol
}

func (fs *fakeStorage) IsNotFoundError(err error) bool {
	return fs.isNotFound
}

// TestActivities_Read_Success verifies that activities.Read returns the expected result
// when the Storage implementation successfully reads the data.
func (r *Suite) TestActivities_Read_Success() {
	expected := "hello world"
	fake.readFn = func(ctx context.Context, path string) (any, error) {
		return expected, nil
	}
	act.impls = map[string]intf.Storage{"test": fake}
	result, err := r.activitySuite.ExecuteActivity(Activities.Read, "test", "dummyPath")

	r.Require().NoError(err)

	var res string
	result.Get(&res)
	r.Require().Equal(res, expected)
}

// TestActivities_Read_Notfound verifies that activities.Read returns the expected result not found
func (r *Suite) TestActivities_Read_Notfound() {
	fake.isNotFound = true
	fake.readFn = func(ctx context.Context, path string) (any, error) {
		return nil, errors.New("error")
	}
	act.impls = map[string]intf.Storage{"test": fake}
	result, err := r.activitySuite.ExecuteActivity(Activities.Read, "test", "dummyPath")

	r.Require().NoError(err)
	var res string
	result.Get(&res)
	r.Require().Equal(res, "")
}

// TestActivities_Read_Error verifies that activities.Read properly wraps errors
// returned by the Storage implementation using cadence.CustomError.
func (r *Suite) TestActivities_Read_Error() {
	expectedErr := errors.New("read error")
	fake.readFn = func(ctx context.Context, path string) (any, error) {
		return "", expectedErr
	}
	act.impls = map[string]intf.Storage{"test": fake}
	_, err := r.activitySuite.ExecuteActivity(Activities.Read, "test", "dummyPath")

	// Verify that the returned error message contains the original error message.
	r.Require().Error(err)

	// Verify that the error message includes the YARPC error code.
	yarpcCode := yarpcerrors.FromError(expectedErr).Code().String()
	if !strings.Contains(err.Error(), yarpcCode) {
		r.Require().Fail("expected error message to contain YARPC code %q, got %q", yarpcCode, err.Error())
	}
}

// TestActivities_Read_UnsupportedProtocol verifies that activities.Read returns an error
// when an unsupported protocol is provided.
func (r *Suite) TestActivities_Read_UnsupportedProtocol() {
	// Initialize activities with an empty storage implementation map.
	act.impls = map[string]intf.Storage{}
	result, err := r.activitySuite.ExecuteActivity(Activities.Read, "test", "dummyPath")
	if result != nil {
		r.Require().Fail("expected nil result for unsupported protocol, got %v", result)
	}
	if err == nil {
		r.Require().Fail("expected error for unsupported protocol, got nil")
	}

	// Verify the error message indicates the protocol is not supported.
	expectedMsg := fmt.Sprintf("protocol %s is not supported", "test")
	r.Require().Contains(err.Error(), expectedMsg)
}
