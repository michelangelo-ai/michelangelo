package gcs

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// BlobStoreClientOut is the output of the gcs module.
type BlobStoreClientOut struct {
	fx.Out
	BlobStoreClient blobstore.BlobStoreClient `group:"blobstore_clients"`
}

// Module provides the Google Cloud Storage blob store client.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(newClient),
)

// newClient creates the Google Cloud Storage blob store client.
//
// When the application config contains a gcs section, the underlying
// storage client is constructed immediately so that misconfiguration
// (for example an unreadable credentials file) fails at startup rather
// than on the first gs:// read. Without a gcs section the storage
// client is constructed lazily on first use (see gcsBlobClient), so
// providing this module never fails startup for deployments that do
// not use GCS.
func newClient(config Config) (BlobStoreClientOut, error) {
	client := newGcsBlobClient(config)
	if config.configured {
		if _, err := client.ensureClient(); err != nil {
			return BlobStoreClientOut{}, err
		}
	}
	return BlobStoreClientOut{BlobStoreClient: client}, nil
}
