package utils

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestUpdateStatusIfWithRetries(t *testing.T) {
	conflict := errors.New("rpc error: code = FailedPrecondition desc = the object has been modified")
	setState := func(state v2pb.RayClusterState) func(*v2pb.RayCluster) {
		return func(rc *v2pb.RayCluster) { rc.Status.State = state }
	}
	tests := []struct {
		name        string
		reads       []func(*v2pb.RayCluster) // the state each re-read returns
		updateErrs  []error                  // each UpdateStatus call's result
		wantWritten bool
		wantErr     error
	}{
		{
			name:        "guard holds: written",
			reads:       []func(*v2pb.RayCluster){setState(v2pb.RAY_CLUSTER_STATE_PROVISIONING)},
			updateErrs:  []error{nil},
			wantWritten: true,
		},
		{
			name:  "guard fails: nothing written",
			reads: []func(*v2pb.RayCluster){setState(v2pb.RAY_CLUSTER_STATE_READY)},
		},
		{
			name: "guard re-evaluated on the re-read after a conflict",
			reads: []func(*v2pb.RayCluster){
				setState(v2pb.RAY_CLUSTER_STATE_PROVISIONING),
				setState(v2pb.RAY_CLUSTER_STATE_READY),
			},
			updateErrs: []error{conflict},
		},
		{
			name:       "non-retriable write error",
			reads:      []func(*v2pb.RayCluster){setState(v2pb.RAY_CLUSTER_STATE_PROVISIONING)},
			updateErrs: []error{assert.AnError},
			wantErr:    assert.AnError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := apimocks.NewMockHandler(ctrl)
			reads := 0
			handler.EXPECT().Get(gomock.Any(), "ns", "rc", gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj client.Object) error {
					tt.reads[reads](obj.(*v2pb.RayCluster))
					reads++
					return nil
				}).Times(len(tt.reads))
			updates := 0
			handler.EXPECT().UpdateStatus(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, obj client.Object, _ *metav1.UpdateOptions) error {
					assert.Equal(t, "updated", obj.(*v2pb.RayCluster).Status.LogUrl)
					err := tt.updateErrs[updates]
					updates++
					return err
				}).Times(len(tt.updateErrs))

			job := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rc"}}
			written, err := UpdateStatusIfWithRetries(context.Background(), handler, job, func(obj client.Object) bool {
				rc := obj.(*v2pb.RayCluster)
				if rc.Status.State != v2pb.RAY_CLUSTER_STATE_PROVISIONING {
					return false
				}
				rc.Status.LogUrl = "updated"
				return true
			}, &metav1.UpdateOptions{})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantWritten, written)
		})
	}
}
