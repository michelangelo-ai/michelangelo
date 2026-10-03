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
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/yarpcerrors"
)

// stubAuthenticator returns a fixed principal/error and records the token.
type stubAuthenticator struct {
	principal *Principal
	err       error
	gotToken  string
}

func (a *stubAuthenticator) Authenticate(_ context.Context, token string) (*Principal, error) {
	a.gotToken = token
	return a.principal, a.err
}

// stubHandler records whether it was invoked and with what context.
type stubHandler struct {
	called bool
	gotCtx context.Context
}

func (h *stubHandler) Handle(ctx context.Context, _ *transport.Request, _ transport.ResponseWriter) error {
	h.called = true
	h.gotCtx = ctx
	return nil
}

func newRequest(headers map[string]string) *transport.Request {
	h := transport.NewHeaders()
	for k, v := range headers {
		h = h.With(k, v)
	}
	return &transport.Request{Procedure: "Test::Call", Headers: h}
}

func service(name string) *Principal {
	return &Principal{Type: PrincipalTypeService, Name: name}
}

func TestInboundMiddleware_ValidToken_StoresPrincipal(t *testing.T) {
	t.Parallel()

	a := &stubAuthenticator{principal: service("my-service")}
	mw := NewInboundMiddleware(a, InboundConfig{AllowedServices: []string{"my-service"}}, nil)

	h := &stubHandler{}
	err := mw.Handle(context.Background(), newRequest(map[string]string{"authorization": "bearer tok"}), nil, h)

	require.NoError(t, err)
	assert.Equal(t, "tok", a.gotToken)
	require.True(t, h.called)
	p, ok := PrincipalFromContext(h.gotCtx)
	require.True(t, ok)
	assert.Equal(t, "my-service", p.Name)
}

func TestInboundMiddleware_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		header   map[string]string
		authn    *stubAuthenticator
		wantCode yarpcerrors.Code
	}{
		{
			name:     "missing header",
			header:   nil,
			authn:    &stubAuthenticator{principal: service("my-service")},
			wantCode: yarpcerrors.CodeUnauthenticated,
		},
		{
			name:     "empty bearer",
			header:   map[string]string{"authorization": "Bearer "},
			authn:    &stubAuthenticator{principal: service("my-service")},
			wantCode: yarpcerrors.CodeUnauthenticated,
		},
		{
			name:     "bare token without scheme",
			header:   map[string]string{"authorization": "some-bare-token"},
			authn:    &stubAuthenticator{principal: service("my-service")},
			wantCode: yarpcerrors.CodeUnauthenticated,
		},
		{
			name:     "verification fails",
			header:   map[string]string{"authorization": "Bearer tok"},
			authn:    &stubAuthenticator{err: errors.New("bad signature")},
			wantCode: yarpcerrors.CodeUnauthenticated,
		},
		{
			name:     "service not in allowlist",
			header:   map[string]string{"authorization": "Bearer tok"},
			authn:    &stubAuthenticator{principal: service("malicious")},
			wantCode: yarpcerrors.CodePermissionDenied,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mw := NewInboundMiddleware(tt.authn, InboundConfig{AllowedServices: []string{"my-service"}}, nil)
			h := &stubHandler{}
			err := mw.Handle(context.Background(), newRequest(tt.header), nil, h)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, yarpcerrors.FromError(err).Code())
			assert.False(t, h.called, "handler must not run when authentication fails")
		})
	}
}

func TestServiceAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		allowed   []string
		principal *Principal
		wantAllow bool
	}{
		{"empty allowlist accepts any service", nil, service("anything"), true},
		{"empty allowlist accepts user", nil, &Principal{Type: PrincipalTypeUser, Name: "u"}, true},
		{"allowed service accepted", []string{"my-service"}, service("my-service"), true},
		{"other service rejected", []string{"my-service"}, service("malicious"), false},
		{"user rejected when allowlist set", []string{"my-service"}, &Principal{Type: PrincipalTypeUser, Name: "my-service"}, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mw := NewInboundMiddleware(nil, InboundConfig{AllowedServices: tt.allowed}, nil).(*inboundMiddleware)
			assert.Equal(t, tt.wantAllow, mw.serviceAllowed(tt.principal))
		})
	}
}

// stubTokenSource is a TokenSource test double.
type stubTokenSource struct {
	token string
	err   error
}

func (s stubTokenSource) Token(context.Context) (string, error) { return s.token, s.err }

// recordingOutbound captures the request it receives.
type recordingOutbound struct {
	got *transport.Request
}

func (o *recordingOutbound) Call(_ context.Context, req *transport.Request) (*transport.Response, error) {
	o.got = req
	return &transport.Response{}, nil
}

func (o *recordingOutbound) Start() error                      { return nil }
func (o *recordingOutbound) Stop() error                       { return nil }
func (o *recordingOutbound) IsRunning() bool                   { return true }
func (o *recordingOutbound) Transports() []transport.Transport { return nil }

func TestBearerOutboundMiddleware_AttachesToken(t *testing.T) {
	t.Parallel()

	out := &recordingOutbound{}
	mw := NewBearerOutboundMiddleware(stubTokenSource{token: "tok"})

	resp, err := mw.Call(context.Background(), &transport.Request{Headers: transport.NewHeaders()}, out)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.NotNil(t, out.got)
	got, ok := out.got.Headers.Get(AuthorizationHeader)
	assert.True(t, ok)
	assert.Equal(t, "Bearer tok", got)
}

func TestBearerOutboundMiddleware_TokenErrorNotForwarded(t *testing.T) {
	t.Parallel()

	out := &recordingOutbound{}
	mw := NewBearerOutboundMiddleware(stubTokenSource{err: errors.New("no agent")})

	_, err := mw.Call(context.Background(), &transport.Request{Headers: transport.NewHeaders()}, out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get auth token")
	assert.Nil(t, out.got, "outbound must not be called when fetching the token fails")
}
