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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

// serviceAccountNamePrefix identifies a Kubernetes ServiceAccount subject
// (e.g. from TokenReview), as opposed to a human user.
const serviceAccountNamePrefix = "system:serviceaccount:"

// groupsAttr and uidAttr are the Principal.Attrs keys FromTokenAuthenticator
// and K8sRBACAuth use to round-trip a k8s.io/apiserver user.Info's groups
// and UID through a Principal, since Principal has no dedicated fields for
// them. groupsAttr is a JSON-encoded []string, not a delimited string, so a
// group name containing a comma (e.g. an LDAP DN) round-trips intact.
const (
	groupsAttr = "groups"
	uidAttr    = "uid"
)

// FromTokenAuthenticator adapts a k8s.io/apiserver authenticator.Token --
// for example a composed OIDC and/or ServiceAccount TokenReview
// authenticator, see go/auth/oidc -- into this package's Authenticator, for
// use with NewInboundMiddleware. The verified identity's username becomes
// the Principal's Name; its UID and groups (if any) are carried in Attrs so
// K8sRBACAuth can reconstruct a user.Info for the authorization decision.
// Extra and Audiences on the underlying authenticator.Response, if any, are
// not carried over -- Principal has no fields for them today.
func FromTokenAuthenticator(inner authenticator.Token) Authenticator {
	if inner == nil {
		panic("auth: FromTokenAuthenticator requires a non-nil authenticator.Token")
	}
	return tokenAuthenticatorAdapter{inner: inner}
}

type tokenAuthenticatorAdapter struct {
	inner authenticator.Token
}

// Authenticate implements Authenticator.
func (a tokenAuthenticatorAdapter) Authenticate(ctx context.Context, token string) (*Principal, error) {
	response, ok, err := a.inner.AuthenticateToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !ok || response == nil || response.User == nil || response.User.GetName() == "" {
		return nil, errors.New("auth: token did not authenticate")
	}
	return principalFromUserInfo(response.User), nil
}

// principalFromUserInfo maps a verified user.Info onto a Principal. A name
// matching serviceAccountNamePrefix (e.g. from ServiceAccount TokenReview)
// is labelled PrincipalTypeService rather than PrincipalTypeUser, so
// InboundConfig.AllowedServices can match it.
func principalFromUserInfo(u user.Info) *Principal {
	principalType := PrincipalTypeUser
	if strings.HasPrefix(u.GetName(), serviceAccountNamePrefix) {
		principalType = PrincipalTypeService
	}
	attrs := map[string]string{}
	if groups := u.GetGroups(); len(groups) > 0 {
		if encoded, err := json.Marshal(groups); err == nil {
			attrs[groupsAttr] = string(encoded)
		}
	}
	if uid := u.GetUID(); uid != "" {
		attrs[uidAttr] = uid
	}
	return &Principal{Type: principalType, Name: u.GetName(), Attrs: attrs}
}

func userInfoFromPrincipal(p *Principal) user.Info {
	var groups []string
	if raw := p.Attrs[groupsAttr]; raw != "" {
		// A decode failure leaves groups nil (fails closed to fewer
		// grants, not more) rather than surfacing a parse error here --
		// this string is never anything but our own encoding's output.
		_ = json.Unmarshal([]byte(raw), &groups)
	}
	return &user.DefaultInfo{Name: p.Name, UID: p.Attrs[uidAttr], Groups: groups}
}

// K8sRBACAuth adapts a k8s.io/apiserver authorizer.Authorizer -- for
// example one built by k8srbac.NewSARAuthorizer, delegating to Kubernetes
// RBAC via SubjectAccessReview -- into this package's Auth, for use in
// place of DummyAuth. It reads the Principal set in ctx by the inbound
// authentication middleware (see NewInboundMiddleware and
// PrincipalFromContext); both UserAuthenticated and UserAuthorized
// therefore report failure when that middleware has not already verified
// the caller, rather than performing their own token verification.
type K8sRBACAuth struct {
	authorizer authorizer.Authorizer
	logger     *zap.Logger
}

var _ Auth = K8sRBACAuth{}

// NewK8sRBACAuth returns an Auth implementation backed by authorizer. A nil
// logger is treated as a no-op logger.
func NewK8sRBACAuth(authorizer authorizer.Authorizer, logger *zap.Logger) K8sRBACAuth {
	if authorizer == nil {
		panic("auth: NewK8sRBACAuth requires a non-nil authorizer.Authorizer")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return K8sRBACAuth{authorizer: authorizer, logger: logger}
}

// UserAuthenticated implements Auth. It does not re-verify any token --
// that already happened in the inbound middleware -- it only reports
// whether a verified Principal is present on ctx.
func (a K8sRBACAuth) UserAuthenticated(ctx context.Context) (bool, error) {
	if principal, ok := PrincipalFromContext(ctx); !ok || principal == nil {
		return false, errors.New("auth: no authenticated principal on context")
	}
	return true, nil
}

// UserAuthorized implements Auth. It asks the wrapped authorizer whether
// the Principal on ctx may perform action on kind within namespace,
// denying unless the decision is DecisionAllow. An empty namespace (a
// cluster-wide request) requires a cluster-scoped RBAC grant. Unknown
// actions and kinds, and a missing Principal, fail closed.
func (a K8sRBACAuth) UserAuthorized(ctx context.Context, namespace string, action Action, kind string) (bool, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal == nil {
		return false, errors.New("auth: no authenticated principal on context")
	}
	attributes, err := Attributes(userInfoFromPrincipal(principal), namespace, action, kind)
	if err != nil {
		return false, err
	}
	decision, reason, err := a.authorizer.Authorize(ctx, attributes)
	if err != nil {
		return false, fmt.Errorf("auth: authorization check failed: %w", err)
	}
	if decision != authorizer.DecisionAllow {
		// The authorizer's reason can echo back RBAC policy detail (role
		// and binding names, etc.); log it server-side but keep the
		// caller-visible error generic.
		a.logger.Info("authorization denied",
			zap.String("principal", principal.Name),
			zap.String("verb", string(action)),
			zap.String("kind", kind),
			zap.String("namespace", namespace),
			zap.String("reason", reason))
		return false, fmt.Errorf("%s cannot %s %s in namespace %q", principal.Name, string(action), kind, namespace)
	}
	return true, nil
}
