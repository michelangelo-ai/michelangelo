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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestNamespaceContextRoundTrip(t *testing.T) {
	_, ok := NamespaceFromContext(context.Background())
	assert.False(t, ok)

	namespace, ok := NamespaceFromContext(WithNamespace(context.Background(), "team-a"))
	assert.True(t, ok)
	assert.Equal(t, "team-a", namespace)
}

type mintedRequest struct {
	namespace string
	name      string
	spec      authenticationv1.TokenRequestSpec
}

// newTokenRequestClient returns a fake clientset whose serviceaccounts/token
// subresource serves sequentially numbered tokens with the given lifetime,
// capturing every mint it performs.
func newTokenRequestClient(lifetime time.Duration) (*fake.Clientset, *[]mintedRequest) {
	client := fake.NewSimpleClientset()
	minted := &[]mintedRequest{}
	client.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateActionImpl)
		if createAction.GetSubresource() != "token" {
			return false, nil, nil
		}
		request := createAction.GetObject().(*authenticationv1.TokenRequest)
		*minted = append(*minted, mintedRequest{
			namespace: createAction.GetNamespace(),
			name:      createAction.Name,
			spec:      request.Spec,
		})
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{
				Token:               fmt.Sprintf("token-%d", len(*minted)),
				ExpirationTimestamp: metav1.NewTime(time.Now().Add(lifetime)),
			},
		}, nil
	})
	return client, minted
}

func TestMinterMintsThroughTokenRequest(t *testing.T) {
	client, minted := newTokenRequestClient(time.Hour)
	minter := NewMinter(client, Config{Audiences: []string{"ma-api"}, TTL: 10 * time.Minute})

	token, err := minter.Token(context.Background(), "team-a")
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)

	require.Len(t, *minted, 1)
	assert.Equal(t, "team-a", (*minted)[0].namespace)
	assert.Equal(t, ServiceAccountName, (*minted)[0].name)
	assert.Equal(t, []string{"ma-api"}, (*minted)[0].spec.Audiences)
	require.NotNil(t, (*minted)[0].spec.ExpirationSeconds)
	assert.Equal(t, int64(600), *(*minted)[0].spec.ExpirationSeconds)
}

func TestMinterZeroTTLLeavesClusterDefault(t *testing.T) {
	client, minted := newTokenRequestClient(time.Hour)
	minter := NewMinter(client, Config{})

	_, err := minter.Token(context.Background(), "team-a")
	require.NoError(t, err)
	require.Len(t, *minted, 1)
	assert.Nil(t, (*minted)[0].spec.ExpirationSeconds)
}

func TestMinterCachesPerNamespace(t *testing.T) {
	client, minted := newTokenRequestClient(time.Hour)
	minter := NewMinter(client, Config{})

	for i := 0; i < 3; i++ {
		token, err := minter.Token(context.Background(), "team-a")
		require.NoError(t, err)
		assert.Equal(t, "token-1", token)
	}
	token, err := minter.Token(context.Background(), "team-b")
	require.NoError(t, err)
	assert.Equal(t, "token-2", token)
	assert.Len(t, *minted, 2)
}

func TestMinterRefreshesNearExpiry(t *testing.T) {
	client, minted := newTokenRequestClient(10 * time.Minute)
	minter := NewMinter(client, Config{})

	_, err := minter.Token(context.Background(), "team-a")
	require.NoError(t, err)

	// Just past the refresh margin, the cached token is re-minted.
	minter.now = func() time.Time { return time.Now().Add(10*time.Minute - refreshMargin + time.Second) }
	token, err := minter.Token(context.Background(), "team-a")
	require.NoError(t, err)
	assert.Equal(t, "token-2", token)
	assert.Len(t, *minted, 2)
}

func TestMinterFailsClosedOnAPIError(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "serviceaccounts", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("no permission to mint")
	})
	minter := NewMinter(client, Config{})

	_, err := minter.Token(context.Background(), "team-a")
	assert.ErrorContains(t, err, `cannot mint a michelangelo-runner token in namespace "team-a"`)
}
