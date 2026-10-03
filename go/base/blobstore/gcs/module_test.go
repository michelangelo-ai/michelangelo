package gcs

import (
	"strings"
	"testing"

	"go.uber.org/config"
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
)

// blobStoreClientsIn consumes the blobstore_clients group the same way the
// worker and controller manager binaries do, forcing fx to run newClient.
type blobStoreClientsIn struct {
	fx.In
	Clients []blobstore.BlobStoreClient `group:"blobstore_clients"`
}

func newAppFromYAML(t *testing.T, yamlContent string) *fx.App {
	t.Helper()
	return fx.New(
		fx.NopLogger,
		fx.Provide(func() (config.Provider, error) {
			return config.NewYAMLProviderFromBytes([]byte(yamlContent))
		}),
		Module,
		fx.Invoke(func(in blobStoreClientsIn) {}),
	)
}

func TestModule_StartsWithoutGcsSection(t *testing.T) {
	app := newAppFromYAML(t, `
otherKey:
  value: something
`)
	if err := app.Err(); err != nil {
		t.Fatalf("expected app construction to succeed without a gcs section, got %v", err)
	}
}

func TestModule_FailsFastWithBrokenGcsSection(t *testing.T) {
	app := newAppFromYAML(t, `
gcs:
  credentialsFile: /nonexistent/credentials.json
`)
	err := app.Err()
	if err == nil {
		t.Fatal("expected app construction to fail for a broken gcs section, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create gcs client") {
		t.Errorf("expected wrapped gcs construction error, got %q", err.Error())
	}
}
