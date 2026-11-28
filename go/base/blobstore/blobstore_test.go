package blobstore

import (
	"context"
	"errors"
	"testing"
)

type mockBlobStoreClient struct {
	scheme string
	readFn func(ctx context.Context, path string) ([]byte, error)
}

func (m *mockBlobStoreClient) Get(ctx context.Context, path string) ([]byte, error) {
	return m.readFn(ctx, path)
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

type limitedClient struct {
	mockBlobStoreClient
	limits []int64
}

func (m *limitedClient) GetLimited(_ context.Context, _ string, limit int64) ([]byte, error) {
	m.limits = append(m.limits, limit)
	return []byte("12345"), nil
}

func TestBlobStore_GetLimited_UsesLimitedGetter(t *testing.T) {
	client := &limitedClient{mockBlobStoreClient: mockBlobStoreClient{scheme: "test", readFn: func(context.Context, string) ([]byte, error) {
		t.Fatal("Get must not be called on a LimitedGetter")
		return nil, nil
	}}}
	bs := BlobStore{}
	bs.RegisterClient(client)
	result, err := bs.GetLimited(context.Background(), "test://blob", 4)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result) != "12345" {
		t.Fatalf("expected 12345, got %q", result)
	}
	if len(client.limits) != 1 || client.limits[0] != 4 {
		t.Fatalf("expected one call with limit 4, got %v", client.limits)
	}
}

func TestBlobStore_GetLimited_FallsBackToGet(t *testing.T) {
	bs := BlobStore{}
	bs.RegisterClient(&mockBlobStoreClient{scheme: "test", readFn: func(context.Context, string) ([]byte, error) {
		return []byte("1234567"), nil
	}})
	for _, tc := range []struct {
		limit int64
		want  string
	}{{limit: 4, want: "12345"}, {limit: 7, want: "1234567"}, {limit: 10, want: "1234567"}} {
		result, err := bs.GetLimited(context.Background(), "test://blob", tc.limit)
		if err != nil {
			t.Fatalf("limit %d: expected no error, got %v", tc.limit, err)
		}
		if string(result) != tc.want {
			t.Fatalf("limit %d: expected %q, got %q", tc.limit, tc.want, result)
		}
	}
}
