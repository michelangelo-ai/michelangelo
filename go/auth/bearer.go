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
	"strings"

	"go.uber.org/yarpc"
)

const authorizationHeader = "authorization"

// BearerTokenFromContext extracts the bearer token from the inbound YARPC
// call's authorization header. It returns false when there is no inbound
// call on the context, the header is absent, or the header does not carry a
// bearer credential.
func BearerTokenFromContext(ctx context.Context) (string, bool) {
	call := yarpc.CallFromContext(ctx)
	raw := strings.TrimSpace(call.Header(authorizationHeader))
	const prefix = "bearer "
	if len(raw) <= len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(raw[len(prefix):])
	return token, token != ""
}
