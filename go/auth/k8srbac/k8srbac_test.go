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

package k8srbac

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var alice = &user.DefaultInfo{Name: "alice@example.com", Groups: []string{"dev"}}

func attributes(verb, resource, namespace string) authorizer.Attributes {
	return authorizer.AttributesRecord{
		User:            alice,
		Verb:            verb,
		APIGroup:        "michelangelo.api",
		Resource:        resource,
		Namespace:       namespace,
		ResourceRequest: true,
	}
}

// newTestAuthorizer wires a SAR authorizer to a local API server stub whose
// SubjectAccessReview endpoint responds with the given allowed decision,
// capturing every review it receives. The delegating authorizer talks
// through a real REST client, so the fake clientset cannot back it.
func newTestAuthorizer(t *testing.T, allowCacheTTL, denyCacheTTL time.Duration, allowed bool) (authorizer.Authorizer, *[]*authorizationv1.SubjectAccessReview) {
	t.Helper()
	reviews := &[]*authorizationv1.SubjectAccessReview{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		review := &authorizationv1.SubjectAccessReview{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(review))
		*reviews = append(*reviews, review)

		review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SubjectAccessReview"}
		review.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: allowed, Reason: "decided by test"}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(review))
	}))
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	sarAuthorizer, err := NewSARAuthorizer(client.AuthorizationV1(), allowCacheTTL, denyCacheTTL)
	require.NoError(t, err)
	return sarAuthorizer, reviews
}

func TestSARAuthorizerAllows(t *testing.T) {
	sarAuthorizer, reviews := newTestAuthorizer(t, 0, 0, true)

	decision, _, err := sarAuthorizer.Authorize(context.Background(), attributes("create", "models", "team-a"))
	require.NoError(t, err)
	assert.Equal(t, authorizer.DecisionAllow, decision)

	// The SubjectAccessReview must carry the attributes and the caller's
	// identity verbatim.
	require.Len(t, *reviews, 1)
	resourceAttributes := (*reviews)[0].Spec.ResourceAttributes
	require.NotNil(t, resourceAttributes)
	assert.Equal(t, "create", resourceAttributes.Verb)
	assert.Equal(t, "michelangelo.api", resourceAttributes.Group)
	assert.Equal(t, "models", resourceAttributes.Resource)
	assert.Equal(t, "team-a", resourceAttributes.Namespace)
	assert.Equal(t, "alice@example.com", (*reviews)[0].Spec.User)
	assert.Equal(t, []string{"dev"}, (*reviews)[0].Spec.Groups)
}

func TestSARAuthorizerDoesNotAllowOnRefusal(t *testing.T) {
	sarAuthorizer, _ := newTestAuthorizer(t, 0, 0, false)

	decision, reason, err := sarAuthorizer.Authorize(context.Background(), attributes("delete", "projects", "team-a"))
	require.NoError(t, err)
	assert.NotEqual(t, authorizer.DecisionAllow, decision)
	assert.Equal(t, "decided by test", reason)
}

func TestAllowDecisionIsCached(t *testing.T) {
	sarAuthorizer, reviews := newTestAuthorizer(t, time.Hour, time.Hour, true)

	for i := 0; i < 3; i++ {
		decision, _, err := sarAuthorizer.Authorize(context.Background(), attributes("get", "models", "team-a"))
		require.NoError(t, err)
		assert.Equal(t, authorizer.DecisionAllow, decision)
	}
	assert.Len(t, *reviews, 1)

	// A different attribute tuple is a different cache key.
	_, _, err := sarAuthorizer.Authorize(context.Background(), attributes("get", "models", "team-b"))
	require.NoError(t, err)
	assert.Len(t, *reviews, 2)
}

func TestZeroTTLDisablesCaching(t *testing.T) {
	sarAuthorizer, reviews := newTestAuthorizer(t, 0, 0, true)

	for i := 0; i < 2; i++ {
		_, _, err := sarAuthorizer.Authorize(context.Background(), attributes("list", "models", "team-a"))
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}
	assert.Len(t, *reviews, 2)
}

func TestDenyCacheIsIndependentOfAllowCache(t *testing.T) {
	sarAuthorizer, reviews := newTestAuthorizer(t, time.Hour, 0, false)

	for i := 0; i < 2; i++ {
		decision, _, err := sarAuthorizer.Authorize(context.Background(), attributes("update", "models", "team-a"))
		require.NoError(t, err)
		assert.NotEqual(t, authorizer.DecisionAllow, decision)
		time.Sleep(5 * time.Millisecond)
	}
	assert.Len(t, *reviews, 2)
}
