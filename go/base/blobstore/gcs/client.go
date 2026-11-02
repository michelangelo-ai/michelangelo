package gcs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// maxRedirects matches net/http.Client's built-in redirect cap. Installing a
// custom CheckRedirect (below) disables that built-in cap, so it has to be
// reproduced explicitly or a misbehaving endpoint could redirect forever.
const maxRedirects = 10

var _ blobstore.BlobStoreClient = (*gcsBlobClient)(nil)

// gcsBlobClient is a client for Google Cloud Storage.
//
// The underlying storage.Client is created lazily on the first Get call
// rather than at construction time: storage.NewClient resolves credentials
// (Application Default Credentials unless configured otherwise) eagerly, so
// eager construction would fail application startup on deployments that
// have no GCP credentials and never read gs:// URIs. Lazy construction
// keeps the gs scheme registered everywhere at no cost to non-GCS users.
type gcsBlobClient struct {
	config Config
	scheme string

	once    sync.Once
	client  *storage.Client
	initErr error
}

func newGcsBlobClient(config Config) *gcsBlobClient {
	return &gcsBlobClient{config: config, scheme: "gs"}
}

// ensureClient creates the storage client on first use. The client is
// created with a background context because it outlives the request that
// triggers its construction.
func (c *gcsBlobClient) ensureClient() (*storage.Client, error) {
	c.once.Do(func() {
		var opts []option.ClientOption
		if c.config.CredentialsFile != "" {
			opts = append(opts, option.WithCredentialsFile(c.config.CredentialsFile))
		}
		if c.config.Anonymous {
			opts = append(opts, option.WithoutAuthentication())
		}
		if c.config.Endpoint != "" {
			opts = append(opts, option.WithEndpoint(c.config.Endpoint))
		}

		// Build the authenticated HTTP client ourselves, the same way
		// storage.NewClient would internally, so a CheckRedirect policy can be
		// installed on it. Without this, a configured endpoint that answers a
		// read with an https -> http redirect would have the bearer token
		// replayed onto the plaintext hop: net/http's default client re-invokes
		// the same RoundTripper (which unconditionally sets the Authorization
		// header) on every redirect it follows, with no scheme check of its own.
		hc, ep, err := htransport.NewClient(context.Background(), opts...)
		if err != nil {
			c.initErr = fmt.Errorf("failed to create authenticated http client: %w", err)
			return
		}
		hc.CheckRedirect = refuseInsecureRedirect

		clientOpts := []option.ClientOption{option.WithHTTPClient(hc)}
		if ep != "" {
			// WithHTTPClient supplies the transport only, not the base URL storage
			// requests are built against — ep is whatever opts (including
			// WithEndpoint, if the caller set one) resolved to, so a configured
			// endpoint still reaches storage.NewClient. When no endpoint was
			// configured, ep comes back empty and storage.NewClient falls back to
			// its own default (the real GCS API), which is what config.Endpoint =
			// "" is supposed to mean.
			clientOpts = append(clientOpts, option.WithEndpoint(ep))
		}

		c.client, c.initErr = storage.NewClient(context.Background(), clientOpts...)
	})
	if c.initErr != nil {
		return nil, fmt.Errorf("failed to create gcs client: %w", c.initErr)
	}
	return c.client, nil
}

// refuseInsecureRedirect is an http.Client.CheckRedirect policy that follows
// redirects the same way net/http's default (a nil CheckRedirect) does, except
// it refuses a hop from https to http — the one case where this client's bearer
// token would otherwise be sent in cleartext.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	last := via[len(via)-1]
	if last.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing to follow %s -> %s redirect from %s to %s: credentials would be sent in cleartext",
			last.URL.Scheme, req.URL.Scheme, last.URL, req.URL)
	}
	return nil
}

// Get retrieves the content of a blob from Google Cloud Storage.
// The blobURI is expected to be in the format "gs://bucket/path/to/object".
func (c *gcsBlobClient) Get(ctx context.Context, blobURI string) ([]byte, error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	if parsedURL.Scheme != c.scheme {
		return nil, fmt.Errorf("scheme %s is not supported by gcs client", parsedURL.Scheme)
	}
	bucket := parsedURL.Host
	if bucket == "" {
		return nil, fmt.Errorf("no bucket in uri %s", blobURI)
	}
	object := strings.TrimPrefix(parsedURL.Path, "/")
	if object == "" {
		return nil, fmt.Errorf("no object path in uri %s", blobURI)
	}

	client, err := c.ensureClient()
	if err != nil {
		return nil, err
	}
	reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("failed to read object: %w", err)
	}
	if err = reader.Close(); err != nil {
		return nil, fmt.Errorf("failed to close object: %w", err)
	}
	return data, nil
}

// Scheme returns the scheme of the blob store client.
func (c *gcsBlobClient) Scheme() string {
	return c.scheme
}
