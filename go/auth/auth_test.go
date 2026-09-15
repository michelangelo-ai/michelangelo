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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/config"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/rest"
)

// TestAuthModule pins the permissive default: with the auth key absent
// from the configuration and no bearer token on the context, every action
// on every kind is authenticated and allowed -- exactly the retired
// DummyAuth behavior.
func TestAuthModule(t *testing.T) {
	var executed = false
	app := fxtest.New(t,
		AuthModule,
		fx.Supply(&rest.Config{Host: "https://127.0.0.1:1"}, zap.NewNop()),
		fx.Provide(func() config.Provider { return newTestProvider(t, "apiserver: {}") }),
		fx.Invoke(func(tokenAuthenticator TokenAuthenticator, resourceAuthorizer Authorizer) {
			userInfo, err := Authenticate(context.Background(), tokenAuthenticator)
			require.NoError(t, err)
			require.NotNil(t, userInfo)
			for action := range verbForAction {
				for kind := range resourceForKind {
					assert.NoError(t, Authorize(context.Background(), resourceAuthorizer, userInfo, "namespace", action, kind))
				}
			}
			executed = true
		}))
	app.RequireStart()
	app.RequireStop()
	assert.True(t, executed)
}

type stubTokenAuthenticator struct {
	response *authenticator.Response
	ok       bool
	err      error
	gotToken string
}

func (s *stubTokenAuthenticator) AuthenticateToken(_ context.Context, token string) (*authenticator.Response, bool, error) {
	s.gotToken = token
	return s.response, s.ok, s.err
}

type stubAuthorizer struct {
	decision authorizer.Decision
	reason   string
	err      error
	got      authorizer.Attributes
}

func (s *stubAuthorizer) Authorize(_ context.Context, attributes authorizer.Attributes) (authorizer.Decision, string, error) {
	s.got = attributes
	return s.decision, s.reason, s.err
}

var alice = &user.DefaultInfo{Name: "alice@example.com", Groups: []string{"dev"}}

func TestAuthenticatePassesBearerTokenThrough(t *testing.T) {
	stub := &stubTokenAuthenticator{response: &authenticator.Response{User: alice}, ok: true}
	userInfo, err := Authenticate(contextWithAuthorization("Bearer tok-123"), stub)
	require.NoError(t, err)
	assert.Equal(t, "tok-123", stub.gotToken)
	assert.Equal(t, alice, userInfo)
}

func TestAuthenticateMapsRejectionsToUnauthenticated(t *testing.T) {
	stub := &stubTokenAuthenticator{err: errors.New("bad signature")}
	_, err := Authenticate(context.Background(), stub)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	stub = &stubTokenAuthenticator{ok: false}
	_, err = Authenticate(context.Background(), stub)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthorizeBuildsAttributesAndAllows(t *testing.T) {
	stub := &stubAuthorizer{decision: authorizer.DecisionAllow}
	require.NoError(t, Authorize(context.Background(), stub, alice, "team-a", Create, "Cluster"))
	assert.Equal(t, alice, stub.got.GetUser())
	assert.Equal(t, "create", stub.got.GetVerb())
	assert.Equal(t, APIGroup, stub.got.GetAPIGroup())
	assert.Equal(t, "clusters", stub.got.GetResource())
	assert.Equal(t, "team-a", stub.got.GetNamespace())
	assert.True(t, stub.got.IsResourceRequest())
}

func TestAuthorizeDeniesUnlessAllow(t *testing.T) {
	for _, decision := range []authorizer.Decision{authorizer.DecisionDeny, authorizer.DecisionNoOpinion} {
		stub := &stubAuthorizer{decision: decision, reason: "no RoleBinding"}
		err := Authorize(context.Background(), stub, alice, "team-a", Delete, "Model")
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
		assert.Contains(t, err.Error(), "no RoleBinding")
	}
}

func TestAuthorizeFailsClosed(t *testing.T) {
	stub := &stubAuthorizer{err: errors.New("apiserver unreachable")}
	err := Authorize(context.Background(), stub, alice, "team-a", Get, "Model")
	assert.Equal(t, codes.Internal, status.Code(err))

	err = Authorize(context.Background(), stub, alice, "team-a", Action("Patch"), "Model")
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	err = Authorize(context.Background(), stub, alice, "team-a", Get, "NotAKind")
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
