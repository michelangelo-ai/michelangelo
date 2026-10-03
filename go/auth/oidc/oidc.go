// Package oidc authenticates bearer tokens presented to the Michelangelo
// API server as OIDC ID tokens verified against a configured issuer,
// implementing the k8s.io/apiserver authenticator.Token contract.
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Authenticator verifies OIDC ID tokens issued by a single configured
// issuer. Signature and issuer checks are delegated to the go-oidc verifier,
// which caches the issuer's JWKS and refreshes it when it sees an unknown
// key ID. Audience and time-based claims are validated here instead so that
// multiple accepted audiences (any-of) and clock-skew leeway are supported.
type Authenticator struct {
	verifier      *gooidc.IDTokenVerifier
	audiences     []string
	usernameClaim string
	groupsClaim   string
	clockSkew     time.Duration
	// now is the clock used for exp/nbf validation; overridable in tests.
	now func() time.Time
}

var _ authenticator.Token = (*Authenticator)(nil)

// New fetches the issuer's discovery document and returns an Authenticator
// for it. It fails fast when the configuration is incomplete or the issuer
// is unreachable, so a misconfigured deployment stops at startup instead of
// denying every request at runtime.
func New(ctx context.Context, config Config) (*Authenticator, error) {
	config = config.withDefaults()
	if config.IssuerURL == "" {
		return nil, errors.New("oidc: issuerUrl is required")
	}
	if len(config.Audiences) == 0 {
		return nil, errors.New("oidc: at least one audience is required")
	}
	provider, err := gooidc.NewProvider(ctx, config.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: issuer discovery failed for %q: %w", config.IssuerURL, err)
	}
	verifier := provider.Verifier(&gooidc.Config{
		// The audience is validated in AuthenticateToken to support any-of
		// matching across multiple accepted audiences.
		SkipClientIDCheck: true,
		// exp/nbf are validated in AuthenticateToken with clock-skew leeway.
		SkipExpiryCheck: true,
	})
	return &Authenticator{
		verifier:      verifier,
		audiences:     config.Audiences,
		usernameClaim: config.UsernameClaim,
		groupsClaim:   config.GroupsClaim,
		clockSkew:     config.ClockSkewLeeway,
		now:           time.Now,
	}, nil
}

// AuthenticateToken verifies the raw ID token's signature, issuer, audience,
// and time-based claims, then extracts the caller's identity from the
// configured username and groups claims. It fails closed: any verification
// problem returns a nil response and a non-nil error.
func (a *Authenticator) AuthenticateToken(ctx context.Context, rawToken string) (*authenticator.Response, bool, error) {
	idToken, err := a.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, false, fmt.Errorf("oidc: token verification failed: %w", err)
	}

	if !anyAudienceMatches(idToken.Audience, a.audiences) {
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
	if nbf, ok := numericDate(claims["nbf"]); ok && now.Add(a.clockSkew).Before(nbf) {
		return nil, false, errors.New("oidc: token is not valid yet")
	}

	username, _ := claims[a.usernameClaim].(string)
	if username == "" {
		return nil, false, fmt.Errorf("oidc: token has no usable %q claim", a.usernameClaim)
	}
	groups, err := stringsClaim(claims[a.groupsClaim])
	if err != nil {
		return nil, false, fmt.Errorf("oidc: malformed %q claim: %w", a.groupsClaim, err)
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{Name: username, Groups: groups},
	}, true, nil
}

// anyAudienceMatches reports whether the token's audience list contains at
// least one of the accepted audiences.
func anyAudienceMatches(tokenAudiences, accepted []string) bool {
	for _, tokenAudience := range tokenAudiences {
		for _, acceptedAudience := range accepted {
			if tokenAudience == acceptedAudience {
				return true
			}
		}
	}
	return false
}

// numericDate converts a JSON NumericDate claim value into a time.
func numericDate(value any) (time.Time, bool) {
	switch v := value.(type) {
	case float64:
		return time.Unix(int64(v), 0), true
	case json.Number:
		seconds, err := v.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.Unix(seconds, 0), true
	default:
		return time.Time{}, false
	}
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
