package spark

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"mock/github.com/michelangelo-ai/michelangelo/proto/api/v2/v2mock"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
)

func Test_CreateSparkJob(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockSparkJob := v2mock.NewMockSparkJobServiceYARPCClient(ctrl)
	act := activities{
		sparkJobService: mockSparkJob,
	}
	sparkJob := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: v2pb.SparkJobSpec{},
	}
	request := &v2pb.CreateSparkJobRequest{
		SparkJob:      sparkJob,
		CreateOptions: &metav1.CreateOptions{},
	}
	mockSparkJob.EXPECT().CreateSparkJob(ctx, request).Return(&v2pb.CreateSparkJobResponse{
		SparkJob: sparkJob,
	}, nil)
	resp, err := act.CreateSparkJob(ctx, *request)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, sparkJob, resp.SparkJob)
}
