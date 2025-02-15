package ray

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"mock/github.com/michelangelo-ai/michelangelo/proto/api/v2/v2mock"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
)

func Test_CreateRayJob(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockRayJob := v2mock.NewMockRayJobServiceYARPCClient(ctrl)
	mockRayCluster := v2mock.NewMockRayClusterServiceYARPCClient(ctrl)
	act := activities{
		rayJobService:     mockRayJob,
		rayClusterService: mockRayCluster,
	}
	rayJob := &v2pb.RayJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayJobSpec{},
	}
	request := &v2pb.CreateRayJobRequest{
		RayJob:        rayJob,
		CreateOptions: &metav1.CreateOptions{},
	}
	mockRayJob.EXPECT().CreateRayJob(ctx, request).Return(&v2pb.CreateRayJobResponse{
		RayJob: rayJob,
	}, nil)
	resp, err := act.CreateRayJob(ctx, request)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, rayJob, resp.RayJob)
}

func Test_CreateRayCluster(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockRayJob := v2mock.NewMockRayJobServiceYARPCClient(ctrl)
	mockRayCluster := v2mock.NewMockRayClusterServiceYARPCClient(ctrl)
	act := activities{
		rayJobService:     mockRayJob,
		rayClusterService: mockRayCluster,
	}
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayClusterSpec{},
	}
	request := &v2pb.CreateRayClusterRequest{
		RayCluster:    rayCluster,
		CreateOptions: &metav1.CreateOptions{},
	}
	mockRayCluster.EXPECT().CreateRayCluster(ctx, request).Return(&v2pb.CreateRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	resp, err := act.CreateRayCluster(ctx, request)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, rayCluster, resp.RayCluster)
}

func Test_TerminateCluster(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockRayJob := v2mock.NewMockRayJobServiceYARPCClient(ctrl)
	mockRayCluster := v2mock.NewMockRayClusterServiceYARPCClient(ctrl)
	act := activities{
		rayJobService:     mockRayJob,
		rayClusterService: mockRayCluster,
	}
	reason := "job failed"
	rayCluster := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.RayClusterSpec{},
	}
	request := &v2pb.UpdateRayClusterRequest{
		RayCluster:    rayCluster,
		UpdateOptions: &metav1.UpdateOptions{},
	}
	mockRayCluster.EXPECT().GetRayCluster(ctx, gomock.Any()).Return(&v2pb.GetRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	mockRayCluster.EXPECT().UpdateRayCluster(ctx, request).Return(&v2pb.UpdateRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	resp, err := act.TerminateCluster(ctx, &TerminateClusterRequest{
		Name:      rayCluster.Name,
		Namespace: rayCluster.Namespace,
		Type:      v2pb.TERMINATION_TYPE_FAILED.String(),
		Reason:    reason,
	})
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, v2pb.TERMINATION_TYPE_FAILED, resp.RayCluster.Spec.Termination.Type)
	assert.Equal(t, reason, resp.RayCluster.Spec.Termination.Reason)

	mockRayCluster.EXPECT().GetRayCluster(ctx, gomock.Any()).Return(&v2pb.GetRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)
	mockRayCluster.EXPECT().UpdateRayCluster(ctx, request).Return(&v2pb.UpdateRayClusterResponse{
		RayCluster: rayCluster,
	}, nil)

	resp, err = act.TerminateCluster(ctx, &TerminateClusterRequest{
		Name:      rayCluster.Name,
		Namespace: rayCluster.Namespace,
		Type:      v2pb.TERMINATION_TYPE_SUCCEEDED.String(),
		Reason:    reason,
	})
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, v2pb.TERMINATION_TYPE_SUCCEEDED, resp.RayCluster.Spec.Termination.Type)
	assert.Equal(t, reason, resp.RayCluster.Spec.Termination.Reason)
}
