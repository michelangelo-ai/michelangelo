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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/yarpcerrors"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

type recordingOutbound struct {
	transport.UnaryOutbound
	got *transport.Request
}

func (r *recordingOutbound) Call(_ context.Context, request *transport.Request) (*transport.Response, error) {
	r.got = request
	return &transport.Response{}, nil
}

func TestMiddlewareAttachesBearerToken(t *testing.T) {
	client, _ := newTokenRequestClient(time.Hour)
	outboundMiddleware := NewOutboundMiddleware(NewMinter(client, Config{}))
	outbound := &recordingOutbound{}

	original := &transport.Request{Procedure: "RayJobService::CreateRayJob"}
	_, err := outboundMiddleware.Call(WithNamespace(context.Background(), "team-a"), original, outbound)
	require.NoError(t, err)

	header, ok := outbound.got.Headers.Get("authorization")
	require.True(t, ok)
	assert.Equal(t, "Bearer token-1", header)

	// The original request is left untouched.
	_, ok = original.Headers.Get("authorization")
	assert.False(t, ok)
}

func TestMiddlewareFailsFastWithoutNamespace(t *testing.T) {
	client, minted := newTokenRequestClient(time.Hour)
	outboundMiddleware := NewOutboundMiddleware(NewMinter(client, Config{}))
	outbound := &recordingOutbound{}

	_, err := outboundMiddleware.Call(context.Background(), &transport.Request{Procedure: "RayJobService::CreateRayJob"}, outbound)
	assert.Equal(t, yarpcerrors.CodeInvalidArgument, yarpcerrors.FromError(err).Code())
	assert.ErrorContains(t, err, "has no project namespace")
	assert.Nil(t, outbound.got)
	assert.Empty(t, *minted)
}

func TestMiddlewarePropagatesMintErrors(t *testing.T) {
	client, _ := newTokenRequestClient(time.Hour)
	client.PrependReactor("create", "serviceaccounts", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	outboundMiddleware := NewOutboundMiddleware(NewMinter(client, Config{}))
	outbound := &recordingOutbound{}

	_, err := outboundMiddleware.Call(WithNamespace(context.Background(), "team-a"), &transport.Request{}, outbound)
	assert.ErrorContains(t, err, "cannot mint")
	assert.Nil(t, outbound.got)
}
