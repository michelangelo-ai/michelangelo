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

// Package k8srbac authorizes requests through Kubernetes RBAC by
// delegating each decision to the SubjectAccessReview API, with the
// upstream allow/deny decision cache bounding the round-trip cost.
package k8srbac

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	authorizationclient "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// NewSARAuthorizer builds a SubjectAccessReview-delegating authorizer with
// the upstream decision cache. A zero TTL disables caching for that
// decision kind. The retry backoff matches kube-apiserver's default for
// delegated authorization webhooks.
func NewSARAuthorizer(client authorizationclient.AuthorizationV1Interface, allowCacheTTL, denyCacheTTL time.Duration) (authorizer.Authorizer, error) {
	return authorizerfactory.DelegatingAuthorizerConfig{
		SubjectAccessReviewClient: client,
		AllowCacheTTL:             allowCacheTTL,
		DenyCacheTTL:              denyCacheTTL,
		WebhookRetryBackoff: &wait.Backoff{
			Duration: 500 * time.Millisecond,
			Factor:   1.5,
			Jitter:   0.2,
			Steps:    5,
		},
	}.New()
}
