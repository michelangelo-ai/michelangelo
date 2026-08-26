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
}

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}
