// Package auth provides the apiserver's authentication and authorization
// extension points.
//
// There are two layers, and they run in this order for every API call:
//
//  1. Transport authentication (authn.go). NewInboundMiddleware is a YARPC
//     inbound middleware that requires an "authorization: Bearer <token>"
//     header and verifies it with a deployment-specific Authenticator (e.g.
//     SPIFFE or OIDC). It optionally restricts callers to an allowlist of
//     service names, then stores the verified Principal in the request
//     context (see PrincipalFromContext). It is installed only when an
//     Authenticator is provided, so by default no token is required. For
//     outbound calls, NewBearerOutboundMiddleware attaches a token from a
//     TokenSource.
//
//  2. Handler authorization (auth.go). The Auth interface is called by the
//     generated API handlers (*.pb.kubeyarpc.go): UserAuthenticated checks the
//     caller is authenticated and UserAuthorized checks the caller may perform
//     an Action on a resource kind in a project. DummyAuth allows everything
//     and is the default wired by DummyAuthModule.
//
// The layers are independent. An Auth implementation can read the Principal
// set by the middleware to make its decisions, but DummyAuth ignores it.
package auth
