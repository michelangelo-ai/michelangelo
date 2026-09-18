package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewClient_UnconfiguredDoesNotFail verifies that the module constructor
// succeeds with an empty config, so providing the module never fails startup
// for deployments that do not use Azure Blob Storage.
func TestNewClient_UnconfiguredDoesNotFail(t *testing.T) {
	out := newClient(Config{})
	if out.BlobStoreClient == nil {
		t.Fatal("newClient returned a nil client for empty config")
	}
	if got := out.BlobStoreClient.Scheme(); got != "abfss" {
		t.Fatalf("Scheme() = %q, want %q", got, "abfss")
	}
}

// TestGet_Unconfigured verifies that an unconfigured client fails at first
// use with a message pointing at the missing settings.
func TestGet_Unconfigured(t *testing.T) {
	client := newAzureBlobClient("", "", "")
	_, err := client.Get(context.Background(), "abfss://container@acct.blob.core.windows.net/path/blob.bin")
	if err == nil {
		t.Fatal("Get on an unconfigured client succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Get error = %q, want it to mention 'not configured'", err.Error())
	}
}

// TestGet_WrongScheme verifies that a configured client still rejects URIs
// with a scheme it does not own.
func TestGet_WrongScheme(t *testing.T) {
	client := newAzureBlobClient("acct", "sig=token", "")
	_, err := client.Get(context.Background(), "s3://bucket/path")
	if err == nil {
		t.Fatal("Get with s3 scheme succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Get error = %q, want it to mention 'not supported'", err.Error())
	}
}

// TestGet_RoundTrip verifies the request path a configured client builds and
// that the blob body is returned as-is.
func TestGet_RoundTrip(t *testing.T) {
	const body = "blob-bytes"
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := newAzureBlobClient("acct", "sig=token", server.URL)
	data, err := client.Get(context.Background(), "abfss://container@acct.blob.core.windows.net/dir/blob.bin")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(data) != body {
		t.Fatalf("Get body = %q, want %q", string(data), body)
	}
	if gotPath != "/container/dir/blob.bin" {
		t.Fatalf("request path = %q, want %q", gotPath, "/container/dir/blob.bin")
	}
	if gotQuery != "sig=token" {
		t.Fatalf("request query = %q, want the SAS token", gotQuery)
	}
}

// TestGet_HTTPError verifies that non-200 responses surface as errors with
// the status code included.
func TestGet_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such blob", http.StatusNotFound)
	}))
	defer server.Close()

	client := newAzureBlobClient("acct", "sig=token", server.URL)
	_, err := client.Get(context.Background(), "abfss://container@acct.blob.core.windows.net/missing")
	if err == nil {
		t.Fatal("Get on a 404 succeeded, want error")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("Get error = %q, want it to mention HTTP 404", err.Error())
	}
}
