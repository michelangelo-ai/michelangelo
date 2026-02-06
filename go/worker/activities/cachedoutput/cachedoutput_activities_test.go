package cachedoutput

import (
	"mock/github.com/michelangelo-ai/michelangelo/proto-go/api/v2/v2mock"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/test/types"
	"github.com/golang/mock/gomock"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
	act              *activities
	server           *httptest.Server
	t                *testing.T
	activitySuite    types.StarTestActivitySuite
	mockCachedOutput *v2mock.MockCachedOutputServiceYARPCClient
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
	r.mockCachedOutput = v2mock.NewMockCachedOutputServiceYARPCClient(ctrl)
	r.act = &activities{
		cachedOutput: r.mockCachedOutput,
	}
	r.activitySuite.RegisterActivity(r.act)
}
func (r *Suite) TearDownSuite() {}

func (r *Suite) BeforeTest(_, _ string) {}

func (r *Suite) Test_Get_Success() {
	request := &v2pb.GetCachedOutputRequest{
		Name:       "test",
		Namespace:  "default",
		GetOptions: nil,
	}
	r.mockCachedOutput.EXPECT().GetCachedOutput(gomock.Any(), request).Return(&v2pb.GetCachedOutputResponse{
		CachedOutput: &v2pb.CachedOutput{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
		},
	}, nil)
	val, err := r.activitySuite.ExecuteActivity(Activities.GetCachedOutput, *request)
	r.Require().NoError(err)
	r.Require().True(val.HasValue())

	var res v2pb.GetCachedOutputResponse
	r.Require().NoError(val.Get(&res))
	r.Require().Equal("test", res.GetCachedOutput().Name)
	r.Require().Equal("default", res.GetCachedOutput().Namespace)
}
