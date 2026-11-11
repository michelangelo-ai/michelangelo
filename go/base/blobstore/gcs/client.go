package gcs

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

var _ blobstore.BlobStoreClient = (*gcsBlobClient)(nil)

// gcsBlobClient is a client for Google Cloud Storage.
//
// By default the underlying storage.Client is created lazily on the first
// Get call rather than at construction time: storage.NewClient resolves
// credentials (Application Default Credentials unless configured otherwise)
// eagerly, so eager construction would fail application startup on
// deployments that have no GCP credentials and never read gs:// URIs. Lazy
// construction keeps the gs scheme registered everywhere at no cost to
// non-GCS users. Deployments that declare a gcs config section opt in to
// eager construction at startup instead (see newClient in module.go).
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
		c.client, c.initErr = storage.NewClient(context.Background(), opts...)
	})
	if c.initErr != nil {
		return nil, fmt.Errorf("failed to create gcs client: %w", c.initErr)
	}
	return c.client, nil
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
