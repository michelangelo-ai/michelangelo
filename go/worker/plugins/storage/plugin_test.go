package storage

import (
	"errors"
	"github.com/stretchr/testify/mock"
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
)

type StoragePluginTestSuite struct {
	suite.Suite
	service.TestSuite
	env *service.TestEnvironment
}

func TestStoragePluginSuite(t *testing.T) {
	suite.Run(t, new(StoragePluginTestSuite))
}

func (s *StoragePluginTestSuite) SetupTest() {
	s.env = s.NewTestEnvironment(s.T(), &service.TestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]service.IPlugin{
			"storage": Plugin,
		},
	})
}

func (s *StoragePluginTestSuite) TearDownTest() {
	s.env.Cadence.AssertExpectations(s.T())
	s.env.Temporal.AssertExpectations(s.T())
}

func (s *StoragePluginTestSuite) TestCadenceReadSuccessful() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(storage.Activities.Read)
	env.OnActivity(storage.Activities.Read, mock.Anything, mock.Anything, mock.Anything).Return(
		starlark.String("test"), nil)
	s.env.Cadence.ExecuteFunction("/test.star", "test_read", nil, nil, nil)
	var res any
	err := s.env.Cadence.GetResult(&res)
	s.Require().NoError(err)
	s.Require().EqualValues(res, "test")
}

func (s *StoragePluginTestSuite) TestCadenceReadFailed() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(storage.Activities.Read)
	env.OnActivity(storage.Activities.Read, mock.Anything, mock.Anything, mock.Anything).Return(
		nil, errors.New("not found"))
	s.env.Cadence.ExecuteFunction("/test.star", "test_read", nil, nil, nil)
	var res any
	err := s.env.Cadence.GetResult(&res)
	s.Require().NoError(err)
	s.Require().Nil(res)
}

func (s *StoragePluginTestSuite) TestTemporalReadSuccessful() {
	env := s.env.Temporal.GetTestWorkflowEnvironment()
	env.RegisterActivity(storage.Activities.Read)
	env.OnActivity(storage.Activities.Read, mock.Anything, mock.Anything, mock.Anything).Return(
		starlark.String("test"), nil)
	s.env.Temporal.ExecuteFunction("/test.star", "test_read", nil, nil, nil)
	var res any
	err := s.env.Temporal.GetResult(&res)
	s.Require().NoError(err)
	s.Require().EqualValues(res, "test")
}

func (s *StoragePluginTestSuite) TestTemporalReadFailed() {
	env := s.env.Temporal.GetTestWorkflowEnvironment()
	env.RegisterActivity(storage.Activities.Read)
	env.OnActivity(storage.Activities.Read, mock.Anything, mock.Anything, mock.Anything).Return(
		nil, errors.New("not found"))
	s.env.Temporal.ExecuteFunction("/test.star", "test_read", nil, nil, nil)
	var res any
	err := s.env.Temporal.GetResult(&res)
	s.Require().NoError(err)
	s.Require().Nil(res)
}
