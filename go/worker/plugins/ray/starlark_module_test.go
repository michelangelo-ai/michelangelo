package ray

import (
	"testing"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/stretchr/testify/suite"
)

type Test struct {
	suite.Suite
	cadstar.StarTestSuite
	env *cadstar.StarTestEnvironment
}

func TestSuite(t *testing.T) { suite.Run(t, new(Test)) }

func (r *Test) SetupTest() {
	r.env = r.NewEnvironment(r.T(), &cadstar.StarTestEnvironmentParams{
		RootDirectory: "testdata",
		Plugins: map[string]cadstar.IPlugin{
			"ray": Plugin,
		},
	})
}

func (r *Test) TearDownTest() { r.env.AssertExpectations(r.T()) }

func (r *Test) TestRunJob() {
	r.env.ExecuteFunction("/test.star", "test_run_job", nil, nil, nil)
	require := r.Require()
	var res any
	require.NoError(r.env.GetResult(&res))
}
