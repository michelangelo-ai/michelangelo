package spark

import (
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Suite struct {
	suite.Suite
	act           *activities
	server        *httptest.Server
	t             *testing.T
	activitySuite types.StarTestActivitySuite
	mockSparkJob  *v2mock.MockSparkJobServiceYARPCClient
}

func TestITCadence(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewCadTestActivitySuite(),
		t:             t,
	})
}

func TestITTemporal(t *testing.T) {
	suite.Run(t, &Suite{
		activitySuite: service.NewTempTestActivitySuite(),
		t:             t,
	})
}

func (r *Suite) SetupSuite() {
	ctrl := gomock.NewController(r.t)
	r.mockSparkJob = v2mock.NewMockSparkJobServiceYARPCClient(ctrl)
	r.act = &activities{
		sparkJobService: r.mockSparkJob,
	}
	r.activitySuite.RegisterActivity(r.act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {}

func (r *Suite) Test_CreateSparkJob_Success() {
	jobName := "job_name"
	request := &v2pb.CreateSparkJobRequest{
		SparkJob: &v2pb.SparkJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: "default",
			},
			Spec: v2pb.SparkJobSpec{
				MainClass: "test",
			},
		},
	}
	r.mockSparkJob.EXPECT().CreateSparkJob(gomock.Any(), request).Return(&v2pb.CreateSparkJobResponse{
		SparkJob: request.SparkJob,
	}, nil)
	val, err := r.activitySuite.ExecuteActivity(Activities.CreateSparkJob, *request)
	r.Require().NoError(err)
	r.Require().True(val.HasValue())

	var res v2pb.CreateSparkJobResponse
	r.Require().NoError(val.Get(&res))
	r.Require().Equal(jobName, res.SparkJob.Name)
	r.Require().Equal("default", res.SparkJob.Namespace)
}

func (r *Suite) Test_GetSparkJob_Success() {
	jobName := "job_name"
	sparkJob := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "default",
		},
		Spec: v2pb.SparkJobSpec{
			MainClass: "test",
		},
	}
	request := &v2pb.GetSparkJobRequest{
		Name:      jobName,
		Namespace: "default",
	}
	r.mockSparkJob.EXPECT().GetSparkJob(gomock.Any(), request).Return(&v2pb.GetSparkJobResponse{
		SparkJob: sparkJob,
	}, nil)
	val, err := r.activitySuite.ExecuteActivity(Activities.GetSparkJob, *request)
	r.Require().NoError(err)
	r.Require().True(val.HasValue())

	var res v2pb.GetSparkJobResponse
	r.Require().NoError(val.Get(&res))
	r.Require().Equal(jobName, res.SparkJob.Name)
	r.Require().Equal("default", res.SparkJob.Namespace)
}
