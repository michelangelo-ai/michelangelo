package cachedoutput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/cachedoutput"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"go.starlark.net/starlark"
)

type CachedOutputModuleTestSuite struct {
	suite.Suite
	service.TestSuite
	env *service.TestEnvironment
}

func TestCachedOutputModuleSuite(t *testing.T) {
	suite.Run(t, new(CachedOutputModuleTestSuite))
}

func (s *CachedOutputModuleTestSuite) SetupTest() {
	s.env = s.NewTestEnvironment(s.T(), &service.TestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]service.IPlugin{
			"cachedoutput": Plugin,
		},
	})
}

func (s *CachedOutputModuleTestSuite) TearDownTest() {
	s.env.Cadence.AssertExpectations(s.T())
	s.env.Temporal.AssertExpectations(s.T())
}

func (s *CachedOutputModuleTestSuite) TestQueryCachedOutputsSuccessfully() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(cachedoutput.Activities.ListCachedOutput)

	// creat timestamp 2025-05-05 00:00:00
	timestamp := time.Date(2025, 5, 5, 0, 0, 0, 0, time.UTC)
	timestampOld := time.Date(2025, 5, 2, 0, 0, 0, 0, time.UTC)
	cachedOutputs := &v2pb.CachedOutputList{
		Items: []v2pb.CachedOutput{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cached-output-1",
					Namespace: "default",
					CreationTimestamp: metav1.Time{
						Time: timestamp,
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cached-output-2",
					Namespace: "default",
					CreationTimestamp: metav1.Time{
						Time: timestampOld,
					},
				},
			},
		},
	}

	var queryRequest v2pb.ListCachedOutputRequest
	env.OnActivity(cachedoutput.Activities.ListCachedOutput, mock.Anything, mock.Anything).Once().
		Run(func(args mock.Arguments) {
			queryRequest = args.Get(1).(v2pb.ListCachedOutputRequest)
		}).
		Return(func(ctx context.Context, req v2pb.ListCachedOutputRequest) (*v2pb.ListCachedOutputResponse, error) {
			return &v2pb.ListCachedOutputResponse{
				CachedOutputList: cachedOutputs,
			}, nil
		})

	s.env.Cadence.ExecuteFunction("/test.star", "test_query", nil, nil, nil)
	require := s.Require()
	var res any
	err := s.env.Cadence.GetResult(&res)
	require.NoError(err)
	require.Equal("default", queryRequest.Namespace)
	require.NotNil(res)
	mapRes := res.(map[string]interface{})
	require.Equal(1, len(mapRes))
	require.Equal("test-cached-output-1", mapRes["cachedOutputList"].(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["metadata"].(map[string]interface{})["name"])
}

func (s *CachedOutputModuleTestSuite) TestQueryCachedOutputsError() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(cachedoutput.Activities.ListCachedOutput)

	env.OnActivity(cachedoutput.Activities.ListCachedOutput, mock.Anything, mock.Anything).Once().
		Return(func(ctx context.Context, req v2pb.ListCachedOutputRequest) (*v2pb.ListCachedOutputResponse, error) {
			return nil, errors.New("some error")
		})

	s.env.Cadence.ExecuteFunction("/test.star", "test_query", nil, nil, nil)
	require := s.Require()
	var res any
	err := s.env.Cadence.GetResult(&res)
	require.Error(err)
}

func (s *CachedOutputModuleTestSuite) TestShouldOverrideCacheForRetry() {
	kw := func(k string, v starlark.Value) starlark.Tuple { return starlark.Tuple{starlark.String(k), v} }
	str := func(v string) starlark.Value { return starlark.String(v) }
	valid := []starlark.Tuple{kw("namespace", str("default")), kw("task_path", str("a.b.task")), kw("task_name", str("task"))}

	tests := []struct {
		name   string
		args   starlark.Tuple
		kwargs []starlark.Tuple
	}{
		{name: "missing required args", kwargs: valid[:2]},
		{name: "wrong type", kwargs: []starlark.Tuple{kw("namespace", str("default")), kw("task_path", starlark.MakeInt(1)), kw("task_name", str("task"))}},
		{name: "unexpected kwarg", kwargs: append([]starlark.Tuple{kw("bogus", str("x"))}, valid...)},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest() // fresh workflow env per case
			env := s.env.Cadence.GetTestWorkflowEnvironment()
			env.RegisterActivity(cachedoutput.Activities.ShouldOverrideCacheForRetry)
			// Arg-unpacking failures must surface before the activity is scheduled.
			env.OnActivity(cachedoutput.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).Maybe().
				Run(func(mock.Arguments) { s.Fail("activity must not run when args are invalid") }).
				Return(&cachedoutput.ShouldOverrideCacheForRetryResponse{}, nil)

			s.env.Cadence.ExecuteFunction("/test.star", "call_override", tt.args, tt.kwargs, nil)
			var res any
			s.Error(s.env.Cadence.GetResult(&res))
		})
	}

	s.Run("success", func() {
		s.SetupTest()
		env := s.env.Cadence.GetTestWorkflowEnvironment()
		env.RegisterActivity(cachedoutput.Activities.ShouldOverrideCacheForRetry)
		var got cachedoutput.ShouldOverrideCacheForRetryRequest
		env.OnActivity(cachedoutput.Activities.ShouldOverrideCacheForRetry, mock.Anything, mock.Anything).Once().
			Run(func(args mock.Arguments) {
				got = args.Get(1).(cachedoutput.ShouldOverrideCacheForRetryRequest)
			}).
			Return(&cachedoutput.ShouldOverrideCacheForRetryResponse{HasOverride: true, UseCache: true, ActivityID: "act-1"}, nil)

		s.env.Cadence.ExecuteFunction("/test.star", "call_override", nil, valid, nil)
		var res map[string]any
		s.Require().NoError(s.env.Cadence.GetResult(&res))
		s.Equal(cachedoutput.ShouldOverrideCacheForRetryRequest{Namespace: "default", TaskPath: "a.b.task", TaskName: "task"}, got)
		s.Equal(map[string]any{"has_override": true, "use_cache": true, "activity_id": "act-1"}, res)
	})
}
