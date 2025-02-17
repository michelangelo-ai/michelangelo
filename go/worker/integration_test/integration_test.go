package integration_test

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	"go.starlark.net/starlark"
	"go.uber.org/cadence/worker"
	"log"
	"net/http/httptest"
	"testing"

	"github.com/cadence-workflow/starlark-worker/cadstar"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/plugin"
)

type Suite struct {
	suite.Suite
	cadstar.StarTestSuite
	httpHandler ext.HTTPTestHandler
	server      *httptest.Server
	env         *cadstar.StarTestEnvironment
}

// MockRayPlugin is a mock implementation of the Ray plugin.
type MockRayPlugin struct{}

// ID returns the plugin ID.
func (m *MockRayPlugin) ID() string {
	return "ray"
}

// Create returns a mock task submission function.
func (m *MockRayPlugin) Create(info cadstar.RunInfo) starlark.Value {
	return starlark.NewBuiltin("submit_task", func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("mock_task_id"), nil
	})
}

// Register adds mock functions to the registry.
func (m *MockRayPlugin) Register(_ worker.Registry) {}

func TestIT(t *testing.T) { suite.Run(t, new(Suite)) }

func (r *Suite) SetupSuite() {
	r.httpHandler = ext.NewHTTPTestHandler(r.T())
	r.server = httptest.NewServer(r.httpHandler)
}

func (r *Suite) SetupTest() {
	mockPlugin := &MockRayPlugin{}
	plugin.Registry[mockPlugin.ID()] = mockPlugin

	r.env = r.NewEnvironment(r.T(), &cadstar.StarTestEnvironmentParams{
		RootDirectory: "/Users/weric/works/uber/michelangelo_ai/michelangelo/python/michelangelo/uniflow",
		Plugins:       plugin.Registry,
	})
}

func (r *Suite) TearDownTest() {
	r.env.AssertExpectations(r.T())
}

func (r *Suite) TearDownSuite() {
	r.server.Close()
}

func (r *Suite) TestRayTask() {

	// clean up test server resources if any
	resources := r.httpHandler.GetResources()
	for k := range resources {
		delete(resources, k)
	}

	// run the test
	r.runTestFunction("plugins/ray/task.star", "task", func() {
		err := r.env.GetResult(nil)
		require := r.Require()
		require.NoError(err)

		//var cadenceErr *cadence.CustomError
		//require.True(errors.As(err, &cadenceErr))
		//
		//var details map[string]any
		//require.NoError(cadenceErr.Details(&details))
		//require.NotNil(details["error"])
		//require.IsType("", details["error"])
	})

	// make sure the test run did not leak any resources on the test server
	r.Require().Equal(0, len(resources), "Test server contains leaked resources:\n%v", resources)
}

func (r *Suite) runTestFunction(filePath string, fn string, assert func()) {

	r.Run(fmt.Sprintf("%s//%s", filePath, fn), func() {

		r.SetupTest()
		defer r.TearDownTest()

		environ := starlark.NewDict(1)
		r.Require().NoError(environ.SetKey(starlark.String("TEST_SERVER_URL"), starlark.String(r.server.URL)))
		log.Printf("[t] environ: %s", environ.String())

		args := []starlark.Value{starlark.String("/some/default/path")}
		r.env.ExecuteFunction(filePath, fn, args, nil, environ)
		assert()
	})
}
