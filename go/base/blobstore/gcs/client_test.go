package gcs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGcsClient_Scheme(t *testing.T) {
	client := newGcsBlobClient(Config{})
	if client.Scheme() != "gs" {
		t.Errorf("expected scheme gs, got %s", client.Scheme())
	}
}

func TestGcsClient_Get_InvalidURL(t *testing.T) {
	client := newGcsBlobClient(Config{})
	_, err := client.Get(context.Background(), "://invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

func TestGcsClient_Get_WrongScheme(t *testing.T) {
	client := newGcsBlobClient(Config{})
	_, err := client.Get(context.Background(), "s3://bucket/path/to/object")
	if err == nil {
		t.Fatal("expected error for wrong scheme, got nil")
	}
	expected := "scheme s3 is not supported by gcs client"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestGcsClient_Get_NoBucket(t *testing.T) {
	client := newGcsBlobClient(Config{})
	_, err := client.Get(context.Background(), "gs:///path/to/object")
	if err == nil {
		t.Error("expected error for missing bucket, got nil")
	}
}

func TestGcsClient_Get_NoObject(t *testing.T) {
	client := newGcsBlobClient(Config{})
	for _, uri := range []string{"gs://bucket", "gs://bucket/"} {
		if _, err := client.Get(context.Background(), uri); err == nil {
			t.Errorf("expected error for missing object path in %s, got nil", uri)
		}
	}
}

// writeFakeServiceAccountKey writes a syntactically valid service account
// JSON key (with a freshly generated throwaway RSA key) so that the
// credentials-file path can be exercised without real GCP credentials.
// Credential material is only parsed at client construction; no token is
// fetched until a request is made, so this stays offline.
func writeFakeServiceAccountKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	saKey := map[string]string{
		"type":         "service_account",
		"project_id":   "test-project",
		"private_key":  string(pemKey),
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	data, err := json.Marshal(saKey)
	if err != nil {
		t.Fatalf("failed to marshal service account key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write credentials file: %v", err)
	}
	return path
}

func TestCredentialSelection(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		expectErr bool
	}{
		{name: "anonymous", config: Config{Anonymous: true}, expectErr: false},
		{name: "anonymous with endpoint", config: Config{Anonymous: true, Endpoint: "http://127.0.0.1:1"}, expectErr: false},
		{name: "credentials file", config: Config{}, expectErr: false}, // CredentialsFile filled in below
		{name: "missing credentials file", config: Config{CredentialsFile: "/nonexistent/credentials.json"}, expectErr: true},
	}
	tests[2].config.CredentialsFile = writeFakeServiceAccountKey(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newGcsBlobClient(tt.config)
			storageClient, err := client.ensureClient()
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error=%v, got %v", tt.expectErr, err)
			}
			if !tt.expectErr && storageClient == nil {
				t.Error("expected storage client to be set, got nil")
			}
		})
	}
}

func TestGcsClient_Get_PropagatesConstructionError(t *testing.T) {
	client := newGcsBlobClient(Config{CredentialsFile: "/nonexistent/credentials.json"})
	_, err := client.Get(context.Background(), "gs://bucket/path/to/object")
	if err == nil {
		t.Fatal("expected construction error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create gcs client") {
		t.Errorf("expected wrapped construction error, got %q", err.Error())
	}
}

// newFakeGCSServer serves object content for any request whose path ends in
// the object name, regardless of the exact API surface (JSON metadata vs
// media download) the storage SDK chooses, and 404s for anything else.
func newFakeGCSServer(t *testing.T, objectName string, content []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/"+objectName) || strings.HasSuffix(r.URL.Path, "%2F"+objectName) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGcsClient_Get_RoundTripAgainstFake(t *testing.T) {
	content := []byte("fake gcs object content")
	server := newFakeGCSServer(t, "model.json", content)

	client := newGcsBlobClient(Config{Anonymous: true, Endpoint: server.URL})
	data, err := client.Get(context.Background(), "gs://test-bucket/path/model.json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", content, data)
	}
}

func TestGcsClient_Get_NotFoundAgainstFake(t *testing.T) {
	server := newFakeGCSServer(t, "exists.json", []byte("x"))

	client := newGcsBlobClient(Config{Anonymous: true, Endpoint: server.URL})
	_, err := client.Get(context.Background(), "gs://test-bucket/missing.json")
	if err == nil {
		t.Fatal("expected error for missing object, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get object") {
		t.Errorf("expected wrapped get-object error, got %q", err.Error())
	}
}

func TestNewClient_ProvidesBlobStoreClient(t *testing.T) {
	out := newClient(Config{Anonymous: true})
	if out.BlobStoreClient == nil {
		t.Fatal("expected BlobStoreClient to be set, got nil")
	}
	if out.BlobStoreClient.Scheme() != "gs" {
		t.Errorf("expected scheme gs, got %s", out.BlobStoreClient.Scheme())
	}
}
