package oidc

import "time"

const (
	defaultUsernameClaim = "email"
	defaultGroupsClaim   = "groups"
	defaultClockSkew     = 30 * time.Second
	// maxClockSkewLeeway bounds ClockSkewLeeway so a misconfiguration can't
	// silently widen the token validity window by an unreasonable amount.
	maxClockSkewLeeway = 5 * time.Minute
)

// defaultSupportedSigningAlgs is the operator-controlled signing algorithm
// allowlist used when Config.SupportedSigningAlgs is unset, rather than
// implicitly trusting whatever algorithms the issuer's discovery document
// advertises.
var defaultSupportedSigningAlgs = []string{"RS256", "ES256"}

// Config configures OIDC ID-token verification.
type Config struct {
	// IssuerURL is the OIDC issuer. The discovery document is fetched from
	// its /.well-known/openid-configuration endpoint, and the token's iss
	// claim must match it exactly. Must be an https:// URL; plain http is
	// only accepted for a loopback host (so local/test issuers work).
	IssuerURL string `yaml:"issuerUrl"`
	// CABundleFile, if set, is a PEM file of additional CA certificates
	// trusted for the issuer's TLS connection (discovery and JWKS fetches),
	// on top of the system root pool. Useful for a private IdP (e.g. Dex or
	// Keycloak) fronted by an internal CA.
	CABundleFile string `yaml:"caBundleFile"`
	// Audiences lists the accepted values of the token's aud claim. A token
	// is accepted when its audience contains at least one of them. No
	// element may be empty.
	Audiences []string `yaml:"audiences"`
	// UsernameClaim is the claim used as the caller's username. Defaults to
	// "email" -- which is only as trustworthy as the IdP's email_verified
	// claim, so AuthenticateToken requires it to be true unless
	// AllowUnverifiedEmail is set. A sub-based setup (stable, IdP-assigned,
	// but opaque) should set this to "sub" explicitly.
	UsernameClaim string `yaml:"usernameClaim"`
	// AllowUnverifiedEmail opts out of the email_verified requirement that
	// otherwise applies when UsernameClaim is "email". Has no effect for
	// any other UsernameClaim.
	AllowUnverifiedEmail bool `yaml:"allowUnverifiedEmail"`
	// UsernamePrefix, if set, is prepended to the username claim's value
	// before it becomes the caller's identity. When unset, a username
	// claim value starting with "system:" is rejected instead, so an
	// IdP-supplied string can't collide with a Kubernetes built-in identity
	// once it reaches RBAC.
	UsernamePrefix string `yaml:"usernamePrefix"`
	// GroupsClaim is the claim carrying the caller's group memberships.
	// Defaults to "groups". A missing groups claim yields no groups rather
	// than an error.
	GroupsClaim string `yaml:"groupsClaim"`
	// GroupsPrefix, if set, is prepended to every group claim value. When
	// unset, a group value starting with "system:" is rejected, for the
	// same reason as UsernamePrefix.
	GroupsPrefix string `yaml:"groupsPrefix"`
	// SupportedSigningAlgs restricts which JWT signing algorithms are
	// accepted. Defaults to {"RS256", "ES256"} rather than inheriting
	// whatever the issuer's discovery document advertises.
	SupportedSigningAlgs []string `yaml:"supportedSigningAlgs"`
	// ClockSkewLeeway is the leeway applied when validating time-based
	// claims (exp, nbf). Defaults to 30s when unset (zero); must not be
	// negative or exceed 5 minutes.
	ClockSkewLeeway time.Duration `yaml:"clockSkewLeeway"`
}

func (c Config) withDefaults() Config {
	if c.UsernameClaim == "" {
		c.UsernameClaim = defaultUsernameClaim
	}
	if c.GroupsClaim == "" {
		c.GroupsClaim = defaultGroupsClaim
	}
	if c.ClockSkewLeeway == 0 {
		c.ClockSkewLeeway = defaultClockSkew
	}
	if len(c.SupportedSigningAlgs) == 0 {
		c.SupportedSigningAlgs = defaultSupportedSigningAlgs
	}
	return c
}
