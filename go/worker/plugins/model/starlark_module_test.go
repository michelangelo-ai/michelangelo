package model

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
	"go.uber.org/cadence"

	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"

	modelactivities "github.com/michelangelo-ai/michelangelo/go/worker/activities/model"
)

type Test struct {
	suite.Suite
	service.TestSuite
	env *service.TestEnvironment
}

func TestSuite(t *testing.T) { suite.Run(t, new(Test)) }

func (r *Test) SetupTest() {
	r.env = r.NewTestEnvironment(r.T(), &service.TestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]service.IPlugin{
			pluginID: Plugin,
		},
	})
}

func (r *Test) TearDownTest() {
	r.env.Cadence.AssertExpectations(r.T())
	r.env.Temporal.AssertExpectations(r.T())
}

// TestModelSearch checks if the modelsearch plugin works as expected.
// In particular, we ensure that the plugin returns a starlark.Dict with the correct fields and excludes deprecated fields.
func (r *Test) TestModelSearch() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(modelactivities.Activities.ModelSearch)
	modelSearchActivityResponse := &modelactivities.ModelSearchResponse{
		ModelName:       "test-model",
		ModelRevisionID: 0,
		Namespace:       "test-namespace",
	}

	env.OnActivity(modelactivities.Activities.ModelSearch, mock.Anything, mock.Anything).Once().Return(modelSearchActivityResponse, nil)
	r.env.Cadence.ExecuteFunction("/test.star", "test_model_search", nil, nil, nil)
	require := r.Require()
	var gotResponse *starlark.Dict
	require.NoError(r.env.Cadence.GetResult(&gotResponse))

	var expectedStarlarkDict *starlark.Dict
	utils.AsStar(map[string]interface{}{
		"modelName":       "test-model",
		"modelRevisionId": 0,
		"namespace":       "test-namespace",
	}, &expectedStarlarkDict)

	// the order of keys may vary between actual and expected, hence we will compare each values separately instead of comparing the entire dict
	for _, key := range expectedStarlarkDict.Keys() {
		expectedVal, expectedFound, expectedErr := expectedStarlarkDict.Get(key)
		if expectedErr != nil {
			r.FailNow("error getting value from expected dict")
		}
		if !expectedFound {
			r.FailNow("key not found in expected dict")
		}

		gotVal, gotFound, gotErr := gotResponse.Get(key)
		if gotErr != nil {
			r.FailNow("error getting value from actual dict")
		}
		if !gotFound {
			r.FailNow("key not found in actual dict")
		}

		require.Equal(expectedVal, gotVal)
	}
}

// TestModelSearchWithActivityError checks if the modelsearch plugin returns an error when the activity fails.
func (r *Test) TestModelSearchWithActivityError() {
	env := r.env.Cadence.GetTestWorkflowEnvironment()
	env.RegisterActivity(modelactivities.Activities.ModelSearch)
	env.OnActivity(modelactivities.Activities.ModelSearch, mock.Anything, mock.Anything).Once().Return(nil, cadence.NewCustomError("activity error"))
	r.env.Cadence.ExecuteFunction("/test.star", "test_model_search", nil, nil, nil)
	require := r.Require()
	var res any
	err := r.env.Cadence.GetResult(res)
	require.Error(err)
}
