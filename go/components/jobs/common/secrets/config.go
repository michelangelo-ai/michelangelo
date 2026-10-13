package secrets

// Provider selection values for Config.Provider.
const (
	// ProviderSample selects the sample implementation that reads secrets
	// from the control-plane cluster. This is the default.
	ProviderSample = "sample"
	// ProviderESO selects the External Secrets Operator backed
	// implementation.
	ProviderESO = "eso"
)

// configKey is the YAML key the secrets configuration is read from. The
// inference-server client factory reads the same key, so one config block
// selects the secret provider for both consumers.
const configKey = "secrets"

// Config selects and configures the SecretProvider implementation.
type Config struct {
	// Provider is "sample" (default) or "eso".
	Provider string `yaml:"provider"`
	// ESO configures the External Secrets Operator backed provider.
	ESO ESOConfig `yaml:"eso"`
}

// ESOConfig configures the External Secrets Operator backed provider.
type ESOConfig struct {
	// Namespace is where the operator-synced credential Secrets live.
	// Defaults to "default", matching the sample provider.
	Namespace string `yaml:"namespace"`
}
