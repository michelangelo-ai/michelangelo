package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAzureClient_GetLimited_StopsAfterLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/container/path/blob.tar.gz" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("0123456789"))
	}))
	t.Cleanup(server.Close)
	client := newAzureBlobClient("account", "sig=x", server.URL)
	uri := "abfss://container@account.blob.core.windows.net/path/blob.tar.gz"

	data, err := client.GetLimited(context.Background(), uri, 4)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != "01234" {
		t.Errorf("expected %q, got %q", "01234", data)
	}
	data, err = client.Get(context.Background(), uri)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != "0123456789" {
		t.Errorf("expected %q, got %q", "0123456789", data)
	}
}
