package oidc

import "time"

const (
	defaultUsernameClaim = "email"
	defaultGroupsClaim   = "groups"
	defaultClockSkew     = 30 * time.Second
)

// Config configures OIDC ID-token verification.
type Config struct {
	// IssuerURL is the OIDC issuer. The discovery document is fetched from
	// its /.well-known/openid-configuration endpoint, and the token's iss
	// claim must match it exactly.
	IssuerURL string `yaml:"issuerUrl"`
	// Audiences lists the accepted values of the token's aud claim. A token
	// is accepted when its audience contains at least one of them.
	Audiences []string `yaml:"audiences"`
	// UsernameClaim is the claim used as the caller's username.
	// Defaults to "email".
	UsernameClaim string `yaml:"usernameClaim"`
	// GroupsClaim is the claim carrying the caller's group memberships.
	// Defaults to "groups". A missing groups claim yields no groups rather
	// than an error.
	GroupsClaim string `yaml:"groupsClaim"`
	// ClockSkewLeeway is the leeway applied when validating time-based
	// claims (exp, nbf). Defaults to 30s.
	ClockSkewLeeway time.Duration `yaml:"clockSkewLeeway"`
}

func (c Config) withDefaults() Config {
	if c.UsernameClaim == "" {
		c.UsernameClaim = defaultUsernameClaim
	}
	if c.GroupsClaim == "" {
		c.GroupsClaim = defaultGroupsClaim
	}
	if c.ClockSkewLeeway <= 0 {
		c.ClockSkewLeeway = defaultClockSkew
	}
	return c
}
