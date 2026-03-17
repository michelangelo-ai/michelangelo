package blobstore

import (
	"context"
	"errors"
	"testing"
)

type mockBlobStoreClient struct {
	scheme string
	readFn func(ctx context.Context, path string) ([]byte, error)
	putFn  func(ctx context.Context, path string, data []byte) error
	delFn  func(ctx context.Context, path string) error
}

func (m *mockBlobStoreClient) Get(ctx context.Context, path string) ([]byte, error) {
	return m.readFn(ctx, path)
}

func (m *mockBlobStoreClient) Put(ctx context.Context, path string, data []byte) error {
	if m.putFn != nil {
		return m.putFn(ctx, path, data)
	}
	return nil
}

func (m *mockBlobStoreClient) Delete(ctx context.Context, path string) error {
	if m.delFn != nil {
		return m.delFn(ctx, path)
	}
	return nil
}

func (m *mockBlobStoreClient) Scheme() string {
	return m.scheme
}

func TestBlobStore_Get(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
		return []byte("test"), nil
	}})
	result, err := bs.Get(context.Background(), "test://test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "test" {
		t.Fatalf("expected test, got %v", result)
	}
}

func TestBlobStore_Get_Error(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(ctx context.Context, blobURI string) ([]byte, error) {
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

func TestBlobStore_Put(t *testing.T) {
	called := false
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{
		scheme: "test",
		readFn: func(_ context.Context, _ string) ([]byte, error) { return nil, nil },
		putFn: func(_ context.Context, _ string, data []byte) error {
			called = true
			if string(data) != "payload" {
				t.Fatalf("unexpected data: %s", data)
			}
			return nil
		},
	})
	if err := bs.Put(context.Background(), "test://bucket/key", []byte("payload")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected Put to be called on client")
	}
}

func TestBlobStore_Delete(t *testing.T) {
	called := false
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{
		scheme: "test",
		readFn: func(_ context.Context, _ string) ([]byte, error) { return nil, nil },
		delFn: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	})
	if err := bs.Delete(context.Background(), "test://bucket/key"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected Delete to be called on client")
	}
}

