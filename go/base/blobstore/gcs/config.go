package gcs

import "go.uber.org/config"

const configKey = "gcs"

// Config is the configuration for the Google Cloud Storage blob store client.
//
// Credentials resolve in this order:
//  1. CredentialsFile, when set (path to a service account JSON key).
//  2. Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS,
//     GKE workload identity, or the GCE metadata server) when no explicit
//     credentials are configured. This is the default, so workload identity
//     works without static keys.
//
// Anonymous disables authentication entirely (public buckets or emulators).
type Config struct {
	// CredentialsFile is an optional path to a service account JSON key.
	// Leave empty to use Application Default Credentials.
	CredentialsFile string `yaml:"credentialsFile"`
	// Endpoint optionally overrides the storage API endpoint, for example
	// for private Google access or a local GCS emulator.
	Endpoint string `yaml:"endpoint"`
	// Anonymous disables authentication. Only useful for public buckets
	// and emulators.
	Anonymous bool `yaml:"anonymous"`

	// configured records whether a "gcs" section was present in the
	// application config at all (even an empty one). Deployments that
	// declare the section get an eagerly constructed client so that
	// misconfiguration fails at startup; deployments without it keep
	// the lazy, zero-cost path. Unexported so it cannot be set from
	// YAML directly.
	configured bool
}

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	value := provider.Get(configKey)
	if err := value.Populate(&conf); err != nil {
		return conf, err
	}
	// HasValue is deprecated for populating defaults, but it is the only
	// way to distinguish "gcs section present" (opt in to eager
	// construction) from "gcs section absent" (stay lazy): an explicit
	// but empty section still counts as present.
	conf.configured = value.HasValue()
	return conf, nil
}
