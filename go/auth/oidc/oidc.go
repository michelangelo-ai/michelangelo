// Package oidc authenticates bearer tokens presented to the Michelangelo
// API server as OIDC ID tokens verified against a configured issuer,
// implementing the k8s.io/apiserver authenticator.Token contract.
package oidc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// httpClientTimeout bounds every HTTP request this package makes to the
// issuer (discovery and JWKS fetches), so an unresponsive or malicious
// issuer can't hang a caller indefinitely.
const httpClientTimeout = 10 * time.Second

// systemPrefix identifies Kubernetes' own reserved identity namespace. An
// IdP-supplied username or group claim value starting with it is rejected
// unless a UsernamePrefix/GroupsPrefix is configured, so it can't collide
// with a Kubernetes built-in identity once it reaches RBAC.
const systemPrefix = "system:"

// Authenticator verifies OIDC ID tokens issued by a single configured
// issuer. Signature and issuer checks are delegated to the go-oidc verifier,
// which caches the issuer's JWKS and refreshes it when it sees an unknown
// key ID. Audience and time-based claims are validated here instead so that
// multiple accepted audiences (any-of) and clock-skew leeway are supported.
type Authenticator struct {
	verifier             *gooidc.IDTokenVerifier
	audiences            []string
	usernameClaim        string
	usernamePrefix       string
	requireEmailVerified bool
	groupsClaim          string
	groupsPrefix         string
	clockSkew            time.Duration
	// now is the clock used for exp/nbf validation; overridable in tests.
	now func() time.Time
}

var _ authenticator.Token = (*Authenticator)(nil)

// New fetches the issuer's discovery document and returns an Authenticator
// for it. It fails fast when the configuration is incomplete or invalid, or
// the issuer is unreachable, so a misconfigured deployment stops at startup
// instead of denying every request at runtime (or, worse, succeeding with a
// weaker guarantee than intended).
func New(ctx context.Context, config Config) (*Authenticator, error) {
	config = config.withDefaults()
	if config.IssuerURL == "" {
		return nil, errors.New("oidc: issuerUrl is required")
	}
	if err := validateIssuerURL(config.IssuerURL); err != nil {
		return nil, err
	}
	if len(config.Audiences) == 0 {
		return nil, errors.New("oidc: at least one audience is required")
	}
	for _, audience := range config.Audiences {
		if audience == "" {
			return nil, errors.New("oidc: audiences must not contain an empty string")
		}
	}
	if config.ClockSkewLeeway < 0 {
		return nil, errors.New("oidc: clockSkewLeeway must not be negative")
	}
	if config.ClockSkewLeeway > maxClockSkewLeeway {
		return nil, fmt.Errorf("oidc: clockSkewLeeway must not exceed %s", maxClockSkewLeeway)
	}

	client, err := newHTTPClient(config.CABundleFile)
	if err != nil {
		return nil, fmt.Errorf("oidc: cannot build HTTP client: %w", err)
	}
	ctx = gooidc.ClientContext(ctx, client)

	provider, err := gooidc.NewProvider(ctx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: issuer discovery failed for %q: %w", config.IssuerURL, err)
	}
	verifier := provider.Verifier(&gooidc.Config{
		// An explicit, operator-controlled allowlist rather than
		// inheriting whatever algorithms the discovery document
		// advertises.
		SupportedSigningAlgs: config.SupportedSigningAlgs,
		// The audience is validated in AuthenticateToken to support any-of
		// matching across multiple accepted audiences -- go-oidc's own
		// check only supports a single ClientID, which doesn't fit that.
		SkipClientIDCheck: true,
		// exp/nbf are validated in AuthenticateToken with clock-skew
		// leeway, which go-oidc's own check does not support.
		SkipExpiryCheck: true,
	})
	return &Authenticator{
		verifier:             verifier,
		audiences:            config.Audiences,
		usernameClaim:        config.UsernameClaim,
		usernamePrefix:       config.UsernamePrefix,
		requireEmailVerified: config.UsernameClaim == defaultUsernameClaim && !config.AllowUnverifiedEmail,
		groupsClaim:          config.GroupsClaim,
		groupsPrefix:         config.GroupsPrefix,
		clockSkew:            config.ClockSkewLeeway,
		now:                  time.Now,
	}, nil
}

// validateIssuerURL requires an absolute https:// URL, allowing http://
// only for a loopback host so local and test issuers keep working.
// Discovery and JWKS fetches over plain HTTP to anything else would not be
// trustworthy: nothing would stop a network attacker from supplying a
// forged discovery document or key set.
func validateIssuerURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("oidc: invalid issuerUrl %q: %w", raw, err)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("oidc: issuerUrl %q must use https (plain http is only allowed for a loopback host)", raw)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newHTTPClient builds the HTTP client used for the issuer's discovery and
// JWKS fetches: a bounded timeout (http.DefaultClient has none), and the
// system root CA pool plus caBundleFile's certificates if set -- so a
// private IdP behind an internal CA (e.g. Dex or Keycloak) can be trusted
// without disabling TLS verification.
func newHTTPClient(caBundleFile string) (*http.Client, error) {
	if caBundleFile == "" {
		return &http.Client{Timeout: httpClientTimeout}, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pem, err := os.ReadFile(caBundleFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read CA bundle %q: %w", caBundleFile, err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA bundle %q contains no usable PEM-encoded certificates", caBundleFile)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	return &http.Client{Timeout: httpClientTimeout, Transport: transport}, nil
}

// AuthenticateToken verifies the raw ID token's signature, issuer, audience,
// and time-based claims, then extracts the caller's identity from the
// configured username and groups claims. It fails closed: any verification
// problem returns a nil response and a non-nil error.
func (a *Authenticator) AuthenticateToken(ctx context.Context, rawToken string) (*authenticator.Response, bool, error) {
	idToken, err := a.verifier.Verify(ctx, rawToken)
	if err != nil {
		// go-oidc's error can embed values taken from the token or the
		// IdP's response; a fixed message here keeps that out of logs
		// (the caller only ever sees a generic message regardless, via
		// NewInboundMiddleware).
		return nil, false, errors.New("oidc: token verification failed")
	}

	matchedAudiences := matchingAudiences(idToken.Audience, a.audiences)
	if len(matchedAudiences) == 0 {
		return nil, false, fmt.Errorf("oidc: token audience %v matches none of the accepted audiences", idToken.Audience)
	}

	now := a.now()
	if idToken.Expiry.IsZero() {
		return nil, false, errors.New("oidc: token has no expiry")
	}
	if now.After(idToken.Expiry.Add(a.clockSkew)) {
		return nil, false, errors.New("oidc: token is expired")
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, false, fmt.Errorf("oidc: cannot parse token claims: %w", err)
	}
	if rawNbf, present := claims["nbf"]; present {
		nbf, ok := numericDate(rawNbf)
		if !ok {
			return nil, false, errors.New("oidc: token has a malformed nbf claim")
		}
		if now.Add(a.clockSkew).Before(nbf) {
			return nil, false, errors.New("oidc: token is not valid yet")
		}
	}

	username, _ := claims[a.usernameClaim].(string)
	if username == "" {
		return nil, false, fmt.Errorf("oidc: token has no usable %q claim", a.usernameClaim)
	}
	if a.requireEmailVerified {
		if verified, _ := claims["email_verified"].(bool); !verified {
			return nil, false, errors.New("oidc: token's email_verified claim is not true")
		}
	}
	if a.usernamePrefix != "" {
		username = a.usernamePrefix + username
	} else if strings.HasPrefix(username, systemPrefix) {
		return nil, false, fmt.Errorf("oidc: %q claim value %q collides with a Kubernetes built-in identity; configure usernamePrefix", a.usernameClaim, username)
	}

	groups, err := stringsClaim(claims[a.groupsClaim])
	if err != nil {
		return nil, false, fmt.Errorf("oidc: malformed %q claim: %w", a.groupsClaim, err)
	}
	if a.groupsPrefix != "" {
		for i, group := range groups {
			groups[i] = a.groupsPrefix + group
		}
	} else {
		for _, group := range groups {
			if strings.HasPrefix(group, systemPrefix) {
				return nil, false, fmt.Errorf("oidc: %q claim value %q collides with a Kubernetes built-in identity; configure groupsPrefix", a.groupsClaim, group)
			}
		}
	}

	return &authenticator.Response{
		Audiences: authenticator.Audiences(matchedAudiences),
		User:      &user.DefaultInfo{Name: username, UID: idToken.Subject, Groups: groups},
	}, true, nil
}

// matchingAudiences returns the accepted audiences present in the token's
// audience list.
func matchingAudiences(tokenAudiences, accepted []string) []string {
	var matched []string
	for _, tokenAudience := range tokenAudiences {
		for _, acceptedAudience := range accepted {
			if tokenAudience == acceptedAudience {
				matched = append(matched, acceptedAudience)
			}
		}
	}
	return matched
}

// numericDate converts a JSON NumericDate claim value into a time. Claims
// are decoded into map[string]any by encoding/json, which always produces
// float64 for a JSON number, so that's the only shape handled here.
func numericDate(value any) (time.Time, bool) {
	seconds, ok := value.(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(seconds), 0), true
}

// stringsClaim normalizes a claim that may be absent, a single string, or a
// list of strings. Any other shape is an error so that a malformed groups
// claim is rejected rather than silently ignored.
func stringsClaim(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("list contains a non-string element %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected claim type %T", value)
	}
}
