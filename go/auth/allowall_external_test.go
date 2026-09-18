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

package auth_test

// End-to-end allow-all regression through a real generated handler: with
// the permissive default bundle and no bearer token on the context, every
// enforced RPC (writes and, since the Get/List fix, reads) passes the
// authentication and authorization checks and reaches the API handler.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/michelangelo-ai/michelangelo/go/auth"
	"github.com/michelangelo-ai/michelangelo/go/logging"
)

type recordingHandler struct {
	calls []string
}

func (r *recordingHandler) Create(_ context.Context, _ client.Object, _ *metav1.CreateOptions) error {
	r.calls = append(r.calls, "Create")
	return nil
}

func (r *recordingHandler) Get(_ context.Context, _, _ string, _ *metav1.GetOptions, _ client.Object) error {
	r.calls = append(r.calls, "Get")
	return nil
}

func (r *recordingHandler) Update(_ context.Context, _ client.Object, _ *metav1.UpdateOptions) error {
	r.calls = append(r.calls, "Update")
	return nil
}

func (r *recordingHandler) UpdateStatus(_ context.Context, _ client.Object, _ *metav1.UpdateOptions) error {
	r.calls = append(r.calls, "UpdateStatus")
	return nil
}

func (r *recordingHandler) Delete(_ context.Context, _ client.Object, _ *metav1.DeleteOptions) error {
	r.calls = append(r.calls, "Delete")
	return nil
}

func (r *recordingHandler) List(_ context.Context, _ string, _ *metav1.ListOptions, _ *apipb.ListOptionsExt, _ client.ObjectList) error {
	r.calls = append(r.calls, "List")
	return nil
}

func (r *recordingHandler) DeleteCollection(_ context.Context, _ client.Object, _ string, _ *metav1.DeleteOptions, _ *metav1.ListOptions) error {
	r.calls = append(r.calls, "DeleteCollection")
	return nil
}

func TestAllowAllBundleThroughGeneratedHandler(t *testing.T) {
	recording := &recordingHandler{}
	bundle := auth.NewAllowAllBundle()
	handler := v2pb.NewClusterServiceHandler(v2pb.FxClusterServiceHandlerParams{
		Handler:       recording,
		MetricsScope:  tally.NoopScope,
		Authenticator: bundle.Authenticator,
		Authorizer:    bundle.Authorizer,
		AuditLog:      &logging.DummyAuditLog{},
	})

	ctx := context.Background() // deliberately no inbound call, no bearer token
	cluster := &v2pb.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "team-a"},
		Spec: v2pb.ClusterSpec{
			Cluster: &v2pb.ClusterSpec_Kubernetes{Kubernetes: &v2pb.KubernetesSpec{}},
		},
	}

	_, err := handler.CreateCluster(ctx, &v2pb.CreateClusterRequest{Cluster: cluster})
	require.NoError(t, err)
	_, err = handler.GetCluster(ctx, &v2pb.GetClusterRequest{Name: "c1", Namespace: "team-a"})
	require.NoError(t, err)
	_, err = handler.ListCluster(ctx, &v2pb.ListClusterRequest{})
	require.NoError(t, err)

	assert.Equal(t, []string{"Create", "Get", "List"}, recording.calls)
}
