package minio

import (
	"testing"
)

func TestMinioClient_ProviderKey(t *testing.T) {
	client := &minioClient{
		providerKey: "test-provider",
		scheme:      "s3",
	}

	if client.ProviderKey() != "test-provider" {
		t.Errorf("expected provider key 'test-provider', got %q", client.ProviderKey())
	}
}

func TestMinioClient_Scheme(t *testing.T) {
	client := &minioClient{
		scheme: "s3",
	}

	if client.Scheme() != "s3" {
		t.Errorf("expected scheme 's3', got %q", client.Scheme())
	}
}
