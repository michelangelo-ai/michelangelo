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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/yarpc/yarpctest"
)

func contextWithAuthorization(value string) context.Context {
	return yarpctest.ContextWithCall(context.Background(), &yarpctest.Call{
		Headers: map[string]string{"authorization": value},
	})
}

func TestBearerTokenFromContext(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		wantToken string
		wantOK    bool
	}{
		{name: "no inbound call", ctx: context.Background()},
		{name: "bearer token", ctx: contextWithAuthorization("Bearer abc.def.ghi"), wantToken: "abc.def.ghi", wantOK: true},
		{name: "lowercase scheme", ctx: contextWithAuthorization("bearer abc"), wantToken: "abc", wantOK: true},
		{name: "surrounding whitespace", ctx: contextWithAuthorization("  Bearer abc  "), wantToken: "abc", wantOK: true},
		{name: "missing header", ctx: yarpctest.ContextWithCall(context.Background(), &yarpctest.Call{})},
		{name: "not a bearer credential", ctx: contextWithAuthorization("Basic dXNlcjpwYXNz")},
		{name: "empty token", ctx: contextWithAuthorization("Bearer ")},
		{name: "scheme only", ctx: contextWithAuthorization("Bearer")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := BearerTokenFromContext(tt.ctx)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}
