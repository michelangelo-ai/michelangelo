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

	"go.uber.org/yarpc/api/middleware"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/yarpcerrors"
)

// OutboundMiddleware attaches a per-project runner bearer token to every
// outbound RPC, keyed by the namespace set via WithNamespace.
type OutboundMiddleware struct {
	minter *Minter
}

var _ middleware.UnaryOutbound = OutboundMiddleware{}

// NewOutboundMiddleware returns the middleware backed by the given Minter.
func NewOutboundMiddleware(minter *Minter) OutboundMiddleware {
	return OutboundMiddleware{minter: minter}
}

// Call mints the runner token for the call's project namespace and sends
// it as the authorization header. A call with no namespace on the context
// fails fast with a clear error instead of arriving unauthenticated.
func (m OutboundMiddleware) Call(ctx context.Context, request *transport.Request, next transport.UnaryOutbound) (*transport.Response, error) {
	namespace, ok := NamespaceFromContext(ctx)
	if !ok || namespace == "" {
		return nil, yarpcerrors.InvalidArgumentErrorf("runnertoken: outbound call %q has no project namespace on the context", request.Procedure)
	}
	token, err := m.minter.Token(ctx, namespace)
	if err != nil {
		return nil, err
	}

	withToken := *request
	withToken.Headers = request.Headers.With("authorization", "Bearer "+token)
	return next.Call(ctx, &withToken)
}
