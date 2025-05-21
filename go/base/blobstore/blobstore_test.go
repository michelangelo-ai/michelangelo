package blobstore

import (
	"context"
	"errors"
	"testing"
)

type mockBlobStoreClient struct {
	scheme string
	readFn func(ctx context.Context, path string) (any, error)
}


func (m *mockBlobStoreClient) Get(ctx context.Context, path string) (any, error) {
	return m.readFn(ctx, path)
}

func (m *mockBlobStoreClient) Scheme() string {
	return m.scheme
}

func TestBlobStore_Get(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, path string) (any, error) {
		return "test", nil
	}})
	result, err := bs.Get(context.Background(), "test://test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "test" {
		t.Fatalf("expected test, got %v", result)
	}
}

func TestBlobStore_Get_Error(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, path string) (any, error) {
		return nil, errors.New("test error")
	}})
	result, err := bs.Get(context.Background(), "test://test")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestBlobStore_Get_UnsupportedScheme(t *testing.T) {
	bs := BlobStore{}
	result, err := bs.Get(context.Background(), "unsupported://test")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}
