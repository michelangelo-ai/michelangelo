package gcs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
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
	return writeFakeServiceAccountKeyWithTokenURI(t, "https://oauth2.googleapis.com/token")
}

// writeFakeServiceAccountKeyWithTokenURI is writeFakeServiceAccountKey with a
// caller-chosen token_uri, so a test can point the OAuth2 JWT-bearer grant at
// a local fake token endpoint and inspect what is requested.
func writeFakeServiceAccountKeyWithTokenURI(t *testing.T, tokenURI string) string {
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
		"token_uri":    tokenURI,
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

func TestRefuseInsecureRedirect(t *testing.T) {
	mustURL := func(t *testing.T, raw string) *http.Request {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatalf("failed to build request for %s: %v", raw, err)
		}
		return req
	}

	tests := []struct {
		name      string
		via       []string // URLs of every request so far, oldest first
		next      string   // URL of the proposed redirect target
		expectErr bool
	}{
		{name: "https to https is allowed", via: []string{"https://a.example.com"}, next: "https://b.example.com", expectErr: false},
		{name: "http to http is allowed (nothing to downgrade)", via: []string{"http://a.example.com"}, next: "http://b.example.com", expectErr: false},
		{name: "http to https is allowed (upgrade)", via: []string{"http://a.example.com"}, next: "https://b.example.com", expectErr: false},
		{name: "https to http is refused", via: []string{"https://a.example.com"}, next: "http://b.example.com", expectErr: true},
		{
			name:      "https to http is refused even after an intermediate https hop",
			via:       []string{"https://a.example.com", "https://b.example.com"},
			next:      "http://c.example.com",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			via := make([]*http.Request, len(tt.via))
			for i, u := range tt.via {
				via[i] = mustURL(t, u)
			}
			err := refuseInsecureRedirect(mustURL(t, tt.next), via)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error=%v, got %v", tt.expectErr, err)
			}
		})
	}
}

func TestRefuseInsecureRedirect_CapsRedirectCount(t *testing.T) {
	via := make([]*http.Request, maxRedirects)
	for i := range via {
		req, err := http.NewRequest(http.MethodGet, "https://a.example.com", nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		via[i] = req
	}
	next, err := http.NewRequest(http.MethodGet, "https://a.example.com", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if err := refuseInsecureRedirect(next, via); err == nil {
		t.Error("expected an error after maxRedirects same-scheme hops, got nil")
	}
}

// newRedirectingFakeGCSServer serves object content for requests whose path
// ends in objectName, exactly like newFakeGCSServer, except the first request
// to any path is answered with a 302 redirect to redirectTo instead of the
// object content — simulating a storage endpoint that redirects reads
// elsewhere. Only the first request redirects, so a legitimate one-hop
// redirect still succeeds; a client that followed every GET as a redirect
// loop would fail with "stopped after N redirects" instead.
func newRedirectingFakeGCSServer(t *testing.T, objectName string, content []byte, redirectTo string) *httptest.Server {
	t.Helper()
	redirected := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !redirected {
			redirected = true
			// Preserve the original request's path and query on the redirect target,
			// the same way a real storage endpoint would redirect a specific object
			// read rather than bouncing every request to the target's root.
			http.Redirect(w, r, redirectTo+r.URL.RequestURI(), http.StatusFound)
			return
		}
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

// TestGcsClient_Get_FollowsNonDowngradingRedirect proves the CheckRedirect
// policy installed in ensureClient doesn't just refuse downgrades in
// isolation (see TestRefuseInsecureRedirect) but is actually wired onto the
// *http.Client storage.NewClient ends up using for real requests: a redirect
// that doesn't downgrade the scheme must still be followed successfully.
//
// This only exercises the http -> http case: both fake servers here are
// plain httptest.NewServer (no TLS), so this cannot reproduce the https ->
// http downgrade itself end-to-end against the real storage/oauth2 stack —
// doing so would need the client to trust an httptest.NewTLSServer
// certificate, which the production code has no seam to inject. The refusal
// behavior itself is covered directly, and exhaustively, by
// TestRefuseInsecureRedirect above.
func TestGcsClient_Get_FollowsNonDowngradingRedirect(t *testing.T) {
	content := []byte("fake gcs object content, reached via redirect")
	target := newFakeGCSServer(t, "model.json", content)
	source := newRedirectingFakeGCSServer(t, "model.json", content, target.URL)

	client := newGcsBlobClient(Config{Anonymous: true, Endpoint: source.URL})
	data, err := client.Get(context.Background(), "gs://test-bucket/path/model.json")
	if err != nil {
		t.Fatalf("expected the non-downgrading redirect to be followed, got error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", content, data)
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

// TestGcsClient_Get_RequestsStorageReadWriteScope proves the authenticated
// HTTP client built in ensureClient requests an OAuth scope. storage.NewClient
// adds default scopes itself, but ensureClient builds the transport with
// htransport.NewClient directly, which adds none: without an explicit scope a
// service account key sends a JWT-bearer grant with no scope claim, which
// Google's token endpoint rejects. A fake token endpoint (via the key's
// token_uri) captures the grant so its scope claim can be checked offline.
func TestGcsClient_Get_RequestsStorageReadWriteScope(t *testing.T) {
	const accessToken = "fake-access-token"
	var gotScope string
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// The assertion is a JWT: header.claims.signature, base64url-encoded.
		parts := strings.Split(r.PostForm.Get("assertion"), ".")
		if len(parts) != 3 {
			http.Error(w, "malformed assertion", http.StatusBadRequest)
			return
		}
		claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var claims struct {
			Scope string `json:"scope"`
		}
		if err := json.Unmarshal(claimsJSON, &claims); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		gotScope = claims.Scope
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, accessToken)
	}))
	t.Cleanup(tokenServer.Close)

	content := []byte("fake gcs object content, read with a scoped token")
	var gotAuth string
	gcsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		_, _ = w.Write(content)
	}))
	t.Cleanup(gcsServer.Close)

	client := newGcsBlobClient(Config{
		CredentialsFile: writeFakeServiceAccountKeyWithTokenURI(t, tokenServer.URL),
		Endpoint:        gcsServer.URL,
	})
	data, err := client.Get(context.Background(), "gs://test-bucket/path/model.json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", content, data)
	}
	if gotScope != storage.ScopeReadWrite {
		t.Errorf("expected JWT grant scope %q, got %q", storage.ScopeReadWrite, gotScope)
	}
	if gotAuth != "Bearer "+accessToken {
		t.Errorf("expected Authorization %q on the storage request, got %q", "Bearer "+accessToken, gotAuth)
	}
}

func TestGcsClient_GetLimited_StopsAfterLimit(t *testing.T) {
	server := newFakeGCSServer(t, "model.json", []byte("0123456789"))

	client := newGcsBlobClient(Config{Anonymous: true, Endpoint: server.URL})
	data, err := client.GetLimited(context.Background(), "gs://test-bucket/path/model.json", 4)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(data) != "01234" {
		t.Errorf("expected %q, got %q", "01234", data)
	}
}
