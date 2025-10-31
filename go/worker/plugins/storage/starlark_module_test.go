package storage

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"

	storageactivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/storage"
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
			pluginID: Plugin,
		},
	})
}

func (s *StoragePluginTestSuite) TearDownTest() {
	s.env.Cadence.AssertExpectations(s.T())
	s.env.Temporal.AssertExpectations(s.T())
}

// TestStorageReadWithoutProvider tests the storage.read function without storage provider
func (s *StoragePluginTestSuite) TestStorageReadWithoutProvider() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(storageactivities.Activities.Read)

	// Mock the storage activity response
	expectedData := map[string]interface{}{
		"key":   "value",
		"test":  "data",
	}

	env.OnActivity(storageactivities.Activities.Read, mock.Anything, "s3://bucket/file", "").Once().Return(expectedData, nil)

	s.env.Cadence.ExecuteFunction("/test_storage_read_without_provider.star", "test", nil, nil, nil)
	require := s.Require()
	var result *starlark.Dict
	err := s.env.Cadence.GetResult(&result)
	require.NoError(err)
	require.NotNil(result)

	// Verify the content
	keyVal, found, err := result.Get(starlark.String("key"))
	require.NoError(err)
	require.True(found)
	require.Equal("value", string(keyVal.(starlark.String)))

	testVal, found, err := result.Get(starlark.String("test"))
	require.NoError(err)
	require.True(found)
	require.Equal("data", string(testVal.(starlark.String)))
}

// TestStorageReadWithProvider tests the storage.read function with explicit storage provider
func (s *StoragePluginTestSuite) TestStorageReadWithProvider() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(storageactivities.Activities.Read)

	// Mock the storage activity response
	expectedData := map[string]interface{}{
		"key":      "value",
		"provider": "aws-prod",
	}

	env.OnActivity(storageactivities.Activities.Read, mock.Anything, "s3://bucket/file", "aws-prod").Once().Return(expectedData, nil)

	s.env.Cadence.ExecuteFunction("/test_storage_read_with_provider.star", "test", nil, nil, nil)
	require := s.Require()
	var result *starlark.Dict
	err := s.env.Cadence.GetResult(&result)
	require.NoError(err)
	require.NotNil(result)

	// Verify the content
	keyVal, found, err := result.Get(starlark.String("key"))
	require.NoError(err)
	require.True(found)
	require.Equal("value", string(keyVal.(starlark.String)))

	providerVal, found, err := result.Get(starlark.String("provider"))
	require.NoError(err)
	require.True(found)
	require.Equal("aws-prod", string(providerVal.(starlark.String)))
}

// TestStorageReadError tests error handling in storage.read function
func (s *StoragePluginTestSuite) TestStorageReadError() {
	env := s.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(storageactivities.Activities.Read)

	// Mock the storage activity to return an error
	env.OnActivity(storageactivities.Activities.Read, mock.Anything, "invalid://url", "").Once().Return(nil, mock.AnythingOfType("*cadence.CustomError"))

	s.env.Cadence.ExecuteFunction("/test_storage_read_error.star", "test", nil, nil, nil)
	require := s.Require()
	var result any
	err := s.env.Cadence.GetResult(&result)
	require.Error(err)
}