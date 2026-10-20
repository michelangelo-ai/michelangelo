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

package runnertoken

import (
	"context"
	"fmt"
)

// TokenSource adapts a Minter into go/auth's TokenSource (Token(ctx)
// (string, error)), for use with auth.NewBearerOutboundMiddleware -- the
// same outbound-middleware slot cmd/apiserver wires its inbound
// counterpart through (see go/auth).
type TokenSource struct {
	minter *Minter
}

// NewTokenSource returns a TokenSource backed by the given Minter.
func NewTokenSource(minter *Minter) TokenSource {
	return TokenSource{minter: minter}
}

// Token mints (or returns the cached) runner token for the project
// namespace set on ctx via WithNamespace. A call with no namespace on the
// context fails fast with a clear error instead of going out
// unauthenticated.
func (t TokenSource) Token(ctx context.Context) (string, error) {
	namespace, ok := NamespaceFromContext(ctx)
	if !ok || namespace == "" {
		return "", fmt.Errorf("runnertoken: outbound call has no project namespace on the context")
	}
	return t.minter.Token(ctx, namespace)
}
