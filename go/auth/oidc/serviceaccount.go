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
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
)

// ServiceAccountAuthenticator verifies Kubernetes ServiceAccount bearer
// tokens through the TokenReview API. It covers in-cluster callers (such as
// the worker) that authenticate with a projected ServiceAccount token
// instead of an OIDC ID token.
type ServiceAccountAuthenticator struct {
	client    kubernetes.Interface
	audiences []string
}

var _ authenticator.Token = (*ServiceAccountAuthenticator)(nil)

// NewServiceAccountAuthenticator returns a TokenReview-backed authenticator.
// audiences, when non-empty, is passed to the TokenReview so the API server
// only accepts tokens bound to one of them.
func NewServiceAccountAuthenticator(client kubernetes.Interface, audiences []string) *ServiceAccountAuthenticator {
	return &ServiceAccountAuthenticator{client: client, audiences: audiences}
}

// AuthenticateToken submits the token for review and fails closed: an
// unreachable TokenReview API denies the request rather than allowing it.
func (s *ServiceAccountAuthenticator) AuthenticateToken(ctx context.Context, rawToken string) (*authenticator.Response, bool, error) {
	review := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token:     rawToken,
			Audiences: s.audiences,
		},
	}
	result, err := s.client.AuthenticationV1().TokenReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("oidc: token review failed: %w", err)
	}
	if !result.Status.Authenticated {
		if result.Status.Error != "" {
			return nil, false, fmt.Errorf("oidc: token not authenticated: %s", result.Status.Error)
		}
		return nil, false, errors.New("oidc: token not authenticated")
	}

	info := &user.DefaultInfo{
		Name:   result.Status.User.Username,
		UID:    result.Status.User.UID,
		Groups: result.Status.User.Groups,
	}
	if len(result.Status.User.Extra) > 0 {
		info.Extra = make(map[string][]string, len(result.Status.User.Extra))
		for key, values := range result.Status.User.Extra {
			info.Extra[key] = values
		}
	}
	return &authenticator.Response{User: info}, true, nil
}
