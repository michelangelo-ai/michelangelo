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
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestTokenSourceReturnsMintedToken(t *testing.T) {
	client, _ := newTokenRequestClient(time.Hour)
	tokenSource := NewTokenSource(NewMinter(client, Config{}))

	token, err := tokenSource.Token(WithNamespace(context.Background(), "team-a"))
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)
}

func TestTokenSourceFailsFastWithoutNamespace(t *testing.T) {
	client, minted := newTokenRequestClient(time.Hour)
	tokenSource := NewTokenSource(NewMinter(client, Config{}))

	_, err := tokenSource.Token(context.Background())
	assert.ErrorContains(t, err, "has no project namespace")
	assert.Empty(t, *minted)
}

func TestTokenSourcePropagatesMintErrors(t *testing.T) {
	client, _ := newTokenRequestClient(time.Hour)
	client.PrependReactor("create", "serviceaccounts", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	tokenSource := NewTokenSource(NewMinter(client, Config{}))

	_, err := tokenSource.Token(WithNamespace(context.Background(), "team-a"))
	assert.ErrorContains(t, err, "cannot mint")
}
