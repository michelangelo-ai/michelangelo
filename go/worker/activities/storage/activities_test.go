package storage

import (
	"context"
	"errors"
	"fmt"
	intf "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage/interface"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"go.uber.org/yarpc/yarpcerrors"
)

// fakeStorage is a mock implementation of the Storage interface for testing.
type fakeStorage struct {
	protocol string
	readFn   func(ctx context.Context, path string) (any, error)
}

// Read calls the fake read function.
func (fs *fakeStorage) Read(ctx context.Context, path string) (any, error) {
	return fs.readFn(ctx, path)
}

// Protocol returns the protocol identifier of the fake storage.
func (fs *fakeStorage) Protocol() string {
	return fs.protocol
}

// TestActivities_Read_Success verifies that activities.Read returns the expected result
// when the Storage implementation successfully reads the data.
func TestActivities_Read_Success(t *testing.T) {
	expected := "hello world"

	// Create a fake storage that returns the expected result.
	fake := &fakeStorage{
		protocol: "test",
		readFn: func(ctx context.Context, path string) (any, error) {
			return expected, nil
		},
	}

	// Initialize activities with the fake storage implementation.
	acts := &activities{
		impls: map[string]intf.Storage{
			"test": fake,
		},
	}

	ctx := context.Background()
	result, err := acts.Read(ctx, "test", "dummyPath")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != expected {
		t.Errorf("expected result %v, got %v", expected, result)
	}
}

// TestActivities_Read_Error verifies that activities.Read properly wraps errors
// returned by the Storage implementation using cadence.CustomError.
func TestActivities_Read_Error(t *testing.T) {
	expectedErr := errors.New("read error")

	// Create a fake storage that returns an error.
	fake := &fakeStorage{
		protocol: "test",
		readFn: func(ctx context.Context, path string) (any, error) {
			return nil, expectedErr
		},
	}

	acts := &activities{
		impls: map[string]intf.Storage{
			"test": fake,
		},
	}
	ctx := context.Background()
	result, err := acts.Read(ctx, "test", "dummyPath")
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify that the returned error message contains the original error message.
	errMsg := err.Error()
	assert.Equal(t, errMsg, "unknown")

	// Verify that the error message includes the YARPC error code.
	yarpcCode := yarpcerrors.FromError(expectedErr).Code().String()
	if !strings.Contains(errMsg, yarpcCode) {
		t.Errorf("expected error message to contain YARPC code %q, got %q", yarpcCode, errMsg)
	}
}

// TestActivities_Read_UnsupportedProtocol verifies that activities.Read returns an error
// when an unsupported protocol is provided.
func TestActivities_Read_UnsupportedProtocol(t *testing.T) {
	// Initialize activities with an empty storage implementation map.
	acts := &activities{
		impls: map[string]intf.Storage{},
	}
	ctx := context.Background()
	result, err := acts.Read(ctx, "unsupported", "dummyPath")
	if result != nil {
		t.Errorf("expected nil result for unsupported protocol, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error for unsupported protocol, got nil")
	}

	// Verify the error message indicates the protocol is not supported.
	expectedMsg := fmt.Sprintf("protocol %s is not supported", "unsupported")
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error message to contain %q, got %q", expectedMsg, err.Error())
	}
}
