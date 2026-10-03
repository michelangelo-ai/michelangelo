// Copyright (c) 2023 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
)

// AuthorizationHeader is the YARPC request header carrying the bearer token.
// It round-trips as "authorization" through YARPC's logical header layer
// regardless of the transport's wire prefix.
const AuthorizationHeader = "authorization"

// bearerScheme is the Authorization header scheme, matched case-insensitively.
const bearerScheme = "bearer"

// Principal types.
const (
	// PrincipalTypeService is a workload (service-to-service) caller.
	PrincipalTypeService = "service"
	// PrincipalTypeUser is a human user caller.
	PrincipalTypeUser = "user"
)

// Principal is the authenticated identity of a caller.
type Principal struct {
	// Type is the kind of caller (PrincipalTypeService or PrincipalTypeUser).
	Type string
	// Name identifies the caller, e.g. the service name "my-service".
	Name string
	// Attrs holds implementation-specific attributes (e.g. env, issuer).
	Attrs map[string]string
}

// Authenticator verifies a bearer token and returns the caller's identity.
// Implementations are deployment-specific (e.g. SPIFFE/OIDC).
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*Principal, error)
}

// TokenSource produces a bearer token for an outbound call.
// Implementations are deployment-specific.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type principalContextKey struct{}

// WithPrincipal returns a copy of ctx carrying the authenticated principal.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFromContext returns the principal stashed by the inbound
// authentication middleware, if any.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(*Principal)
	return p, ok
}

// InboundConfig configures the inbound authentication middleware.
type InboundConfig struct {
	// AllowedServices restricts which service principals (Principal.Name) may
	// call the API. Empty means any successfully authenticated caller is
	// accepted (authenticate-only).
	AllowedServices []string
}

// NewInboundMiddleware returns a YARPC unary inbound middleware that requires
// an "authorization: Bearer <token>" header, verifies it with the given
// Authenticator, enforces the service allowlist, and stores the Principal in
// the request context (see PrincipalFromContext).
func NewInboundMiddleware(a Authenticator, cfg InboundConfig, logger *zap.Logger) middleware.UnaryInbound {
	if logger == nil {
		logger = zap.NewNop()
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedServices))
	for _, s := range cfg.AllowedServices {
		allowed[s] = struct{}{}
	}
	return &inboundMiddleware{authenticator: a, allowedServices: allowed, logger: logger}
}

type inboundMiddleware struct {
	authenticator   Authenticator
	allowedServices map[string]struct{}
	logger          *zap.Logger
}

// Handle implements middleware.UnaryInbound.
func (m *inboundMiddleware) Handle(ctx context.Context, req *transport.Request, resw transport.ResponseWriter, h transport.UnaryHandler) error {
	principal, err := m.authenticate(ctx, req)
	if err != nil {
		return err
	}
	return h.Handle(WithPrincipal(ctx, principal), req, resw)
}

// authenticate verifies the bearer token and enforces the service allowlist.
func (m *inboundMiddleware) authenticate(ctx context.Context, req *transport.Request) (*Principal, error) {
	token, err := bearerToken(req.Headers)
	if err != nil {
		return nil, err
	}

	principal, err := m.authenticator.Authenticate(ctx, token)
	if err != nil {
		m.logger.Warn("token verification failed", zap.Error(err), zap.String("procedure", req.Procedure))
		return nil, yarpcerrors.UnauthenticatedErrorf("invalid token: %v", err)
	}

	if !m.serviceAllowed(principal) {
		m.logger.Warn("service not allowed",
			zap.String("type", principal.Type),
			zap.String("name", principal.Name),
			zap.Any("attrs", principal.Attrs),
			zap.String("procedure", req.Procedure))
		return nil, yarpcerrors.PermissionDeniedErrorf("%s %q is not authorized", principal.Type, principal.Name)
	}

	return principal, nil
}

// serviceAllowed reports whether the principal is a permitted caller. An empty
// allowlist accepts any authenticated principal.
func (m *inboundMiddleware) serviceAllowed(p *Principal) bool {
	if len(m.allowedServices) == 0 {
		return true
	}
	if p.Type != PrincipalTypeService {
		return false
	}
	_, ok := m.allowedServices[p.Name]
	return ok
}

// bearerToken extracts the token from an "authorization: Bearer <token>"
// header. The scheme is required (matched case-insensitively), so a bare token
// is rejected rather than fed to the Authenticator verbatim.
func bearerToken(headers transport.Headers) (string, error) {
	raw, ok := headers.Get(AuthorizationHeader)
	if !ok || raw == "" {
		return "", yarpcerrors.UnauthenticatedErrorf("missing authorization header")
	}
	scheme, token, found := strings.Cut(raw, " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) {
		return "", yarpcerrors.UnauthenticatedErrorf("authorization header must use the Bearer scheme")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", yarpcerrors.UnauthenticatedErrorf("empty bearer token")
	}
	return token, nil
}

// NewBearerOutboundMiddleware returns a YARPC unary outbound middleware that
// fetches a token from ts on every call and attaches it as
// "authorization: Bearer <token>". If fetching fails the call is not sent.
func NewBearerOutboundMiddleware(ts TokenSource) middleware.UnaryOutbound {
	return bearerOutboundMiddleware{tokenSource: ts}
}

type bearerOutboundMiddleware struct {
	tokenSource TokenSource
}

// Call implements middleware.UnaryOutbound.
func (m bearerOutboundMiddleware) Call(ctx context.Context, req *transport.Request, out transport.UnaryOutbound) (*transport.Response, error) {
	token, err := m.tokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}
	req.Headers = req.Headers.With(AuthorizationHeader, "Bearer "+token)
	return out.Call(ctx, req)
}
