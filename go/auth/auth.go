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

	"go.uber.org/fx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
)

// Action performed by user
type Action string

const (
	Create           Action = "Create"
	Get                     = "Get"
	Update                  = "Update"
	Delete                  = "Delete"
	DeleteCollection        = "DeleteCollection"
	List                    = "List"
)

// TokenAuthenticator and Authorizer alias the k8s.io/apiserver contracts
// this package exposes, so generated handlers only import this package.
type (
	TokenAuthenticator = authenticator.Token
	Authorizer         = authorizer.Authorizer
)

// AuthBundle carries the authentication and authorization implementations
// consumed by every generated service handler.
type AuthBundle struct {
	fx.Out

	Authenticator TokenAuthenticator
	Authorizer    Authorizer
}

// alwaysAuthenticated accepts every request, bearer token or not; paired
// with the always-allow authorizer it reproduces the permissive default.
type alwaysAuthenticated struct{}

func (alwaysAuthenticated) AuthenticateToken(context.Context, string) (*authenticator.Response, bool, error) {
	return &authenticator.Response{
		User: &user.DefaultInfo{Name: user.Anonymous, Groups: []string{user.AllUnauthenticated}},
	}, true, nil
}

// NewAllowAllBundle returns the permissive default: every caller is
// authenticated and every request is allowed.
func NewAllowAllBundle() AuthBundle {
	return AuthBundle{
		Authenticator: alwaysAuthenticated{},
		Authorizer:    authorizerfactory.NewAlwaysAllowAuthorizer(),
	}
}

// AuthModule provides the authenticator and authorizer used by the API
// server's generated handlers.
var AuthModule = fx.Options(
	fx.Provide(NewAllowAllBundle),
)

// Authenticate extracts the bearer token from the inbound YARPC call (an
// absent token is passed through as empty -- the authenticator decides) and
// resolves the caller's identity. Errors are gRPC status errors, returned
// to the client verbatim by the generated handlers.
func Authenticate(ctx context.Context, tokenAuthenticator TokenAuthenticator) (user.Info, error) {
	token, _ := BearerTokenFromContext(ctx)
	response, ok, err := tokenAuthenticator.AuthenticateToken(ctx, token)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "bearer token rejected: %v", err)
	}
	if !ok || response == nil || response.User == nil {
		return nil, status.Error(codes.Unauthenticated, "request is not authenticated")
	}
	return response.User, nil
}

// Authorize asks the authorizer whether the authenticated caller may
// perform the action on the resource kind in the namespace, denying unless
// the decision is DecisionAllow. Unknown actions and kinds fail closed, as
// does an errored authorization check, with distinct error codes so an
// explicit RBAC denial is distinguishable from an unavailable authorizer.
func Authorize(ctx context.Context, resourceAuthorizer Authorizer, userInfo user.Info, namespace string, action Action, kind string) error {
	attributes, err := Attributes(userInfo, namespace, action, kind)
	if err != nil {
		return status.Errorf(codes.PermissionDenied, "%v", err)
	}
	decision, reason, err := resourceAuthorizer.Authorize(ctx, attributes)
	if err != nil {
		return status.Errorf(codes.Internal, "authorization check failed: %v", err)
	}
	if decision != authorizer.DecisionAllow {
		if reason == "" {
			reason = "denied by Kubernetes RBAC"
		}
		username := ""
		if userInfo != nil {
			username = userInfo.GetName()
		}
		return status.Errorf(codes.PermissionDenied,
			"%s cannot %s %s in namespace %q: %s", username, string(action), kind, namespace, reason)
	}
	return nil
}
