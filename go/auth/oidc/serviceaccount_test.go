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

package oidc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newTokenReviewClient returns a fake clientset whose TokenReview endpoint
// serves the given status, capturing every review it receives.
func newTokenReviewClient(status authenticationv1.TokenReviewStatus, reviewErr error) (*fake.Clientset, *[]*authenticationv1.TokenReview) {
	client := fake.NewSimpleClientset()
	reviews := &[]*authenticationv1.TokenReview{}
	client.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if reviewErr != nil {
			return true, nil, reviewErr
		}
		review := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenReview)
		*reviews = append(*reviews, review)
		return true, &authenticationv1.TokenReview{Status: status}, nil
	})
	return client, reviews
}

func TestServiceAccountAuthenticateToken(t *testing.T) {
	client, reviews := newTokenReviewClient(authenticationv1.TokenReviewStatus{
		Authenticated: true,
		User: authenticationv1.UserInfo{
			Username: "system:serviceaccount:team-a:michelangelo-runner",
			UID:      "sa-uid-1",
			Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:team-a"},
			Extra:    map[string]authenticationv1.ExtraValue{"authentication.kubernetes.io/pod-name": {"worker-0"}},
		},
	}, nil)
	saAuthenticator := NewServiceAccountAuthenticator(client, []string{"ma-api"})

	response, ok, err := saAuthenticator.AuthenticateToken(context.Background(), "sa-token")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "system:serviceaccount:team-a:michelangelo-runner", response.User.GetName())
	assert.Equal(t, "sa-uid-1", response.User.GetUID())
	assert.Equal(t, []string{"system:serviceaccounts", "system:serviceaccounts:team-a"}, response.User.GetGroups())
	assert.Equal(t, map[string][]string{"authentication.kubernetes.io/pod-name": {"worker-0"}}, response.User.GetExtra())

	// The token and the configured audiences must reach the TokenReview.
	require.Len(t, *reviews, 1)
	assert.Equal(t, "sa-token", (*reviews)[0].Spec.Token)
	assert.Equal(t, []string{"ma-api"}, (*reviews)[0].Spec.Audiences)
}

func TestServiceAccountRejectsUnauthenticatedToken(t *testing.T) {
	client, _ := newTokenReviewClient(authenticationv1.TokenReviewStatus{
		Authenticated: false,
		Error:         "token has expired",
	}, nil)
	saAuthenticator := NewServiceAccountAuthenticator(client, nil)

	_, ok, err := saAuthenticator.AuthenticateToken(context.Background(), "stale-token")
	assert.False(t, ok)
	assert.ErrorContains(t, err, "token has expired")
}

func TestServiceAccountFailsClosedOnAPIError(t *testing.T) {
	client, _ := newTokenReviewClient(authenticationv1.TokenReviewStatus{}, errors.New("apiserver unreachable"))
	saAuthenticator := NewServiceAccountAuthenticator(client, nil)

	_, ok, err := saAuthenticator.AuthenticateToken(context.Background(), "any-token")
	assert.False(t, ok)
	assert.ErrorContains(t, err, "token review failed")
}
