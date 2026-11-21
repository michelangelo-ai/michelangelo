package buildkite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	starcadence "github.com/cadence-workflow/starlark-worker/cadence"
	startemporal "github.com/cadence-workflow/starlark-worker/temporal"
	starworker "github.com/cadence-workflow/starlark-worker/worker"
	starworkflow "github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	temporaltestsuite "go.temporal.io/sdk/testsuite"
	temporalworker "go.temporal.io/sdk/worker"
	"go.uber.org/cadence"
	"go.uber.org/cadence/testsuite"
	"go.uber.org/cadence/worker"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/yarpc/yarpcerrors"
)

func activityEnvironment(t *testing.T, handler http.HandlerFunc) (*testsuite.TestActivityEnvironment, *client) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("BUILDKITE_ORGANIZATION", "example")
	t.Setenv("BUILDKITE_ALLOWED_PIPELINES", "smoke-test")
	t.Setenv("BUILDKITE_API_URL", server.URL+"/v2")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	client := newClient()
	env := newActivityEnvironment()
	env.RegisterActivity(&activities{client: client})
	return env, client
}

func newActivityEnvironment() *testsuite.TestActivityEnvironment {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestActivityEnvironment()
	ctx := context.WithValue(context.Background(), starworkflow.BackendContextKey, starcadence.NewWorkflow())
	env.SetWorkerOptions(worker.Options{BackgroundActivityContext: ctx})
	return env
}

func rawBuild(state string) string {
	return `{"id":"build-uuid","number":17,"state":"` + state + `",` +
		`"web_url":"https://buildkite.com/example/smoke-test/builds/17",` +
		`"branch":"feature","commit":"sha","env":{"PRIVATE_VALUE":"do-not-return"},"jobs":[{}]}`
}

func expectedBuild(state string) BuildResponse {
	return BuildResponse{
		ID: "build-uuid", Number: 17, Pipeline: "smoke-test", State: state,
		WebURL: "https://buildkite.com/example/smoke-test/builds/17", Branch: "feature", Commit: "sha",
	}
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	var customError *cadence.CustomError
	require.ErrorAs(t, err, &customError)
	assert.Equal(t, reason, customError.Reason())
}

func TestTriggerBuildMapping(t *testing.T) {
	payloads := make(chan map[string]any, 1)
	env, _ := activityEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v2/organizations/example/pipelines/smoke-test/builds", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		payloads <- payload
		_, _ = io.WriteString(w, rawBuild("scheduled"))
	})
	req := TriggerBuildRequest{
		Pipeline: "smoke-test", Branch: "feature", Commit: "sha", Message: "smoke",
		Env: map[string]string{"MODE": "smoke"}, Metadata: map[string]string{"release": "bundle"},
	}
	value, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild, req)
	require.NoError(t, err)
	var got BuildResponse
	require.NoError(t, value.Get(&got))
	assert.Equal(t, expectedBuild("scheduled"), got)
	assert.Equal(t, map[string]any{
		"branch": "feature", "commit": "sha", "message": "smoke",
		"env": map[string]any{"MODE": "smoke"}, "meta_data": map[string]any{"release": "bundle"},
	}, <-payloads)
	assert.Equal(t, map[string]string{"MODE": "smoke"}, req.Env)
	assert.Equal(t, map[string]string{"release": "bundle"}, req.Metadata)
}

func TestGetAndCancelSnapshots(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name, method, suffix, state := "get", http.MethodGet, "/17", "failed"
		var function any = Activities.GetBuildkiteBuild
		var req any = GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
		if cancel {
			name, method, suffix, state = "cancel", http.MethodPut, "/17/cancel", "canceling"
			function, req = Activities.CancelBuildkiteBuild, CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}
		}
		t.Run(name, func(t *testing.T) {
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, method, r.Method)
				assert.Equal(t, "/v2/organizations/example/pipelines/smoke-test/builds"+suffix, r.URL.Path)
				if method == http.MethodGet {
					assert.Equal(t, "true", r.URL.Query().Get("exclude_jobs"))
				} else {
					assert.Empty(t, r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, rawBuild(state))
			})
			value, err := env.ExecuteActivity(function, req)
			require.NoError(t, err)
			var got BuildResponse
			require.NoError(t, value.Get(&got))
			assert.Equal(t, expectedBuild(state), got)
		})
	}
}

func TestSensorStates(t *testing.T) {
	for _, state := range []string{
		"passed", "creating", "scheduled", "waiting", "running", "failing", "blocked", "canceling",
		"failed", "canceled", "skipped", "not_run", "unknown",
	} {
		t.Run(state, func(t *testing.T) {
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, rawBuild(state))
			})
			value, err := env.ExecuteActivity(Activities.SensorBuildkiteBuild,
				GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			if state == "passed" {
				require.NoError(t, err)
				var got BuildResponse
				require.NoError(t, value.Get(&got))
				assert.Equal(t, expectedBuild(state), got)
			} else if state == "failed" || state == "canceled" || state == "skipped" || state == "not_run" || state == "unknown" {
				requireReason(t, err, yarpcerrors.CodeInternal.String())
			} else {
				requireReason(t, err, NotReadyReason)
			}
		})
	}
}

func TestSensorChecksBlockedBeforeReportedState(t *testing.T) {
	for _, state := range []string{"passed", "failed"} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/blocked=%t", state, blocked), func(t *testing.T) {
				env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
					body := strings.TrimSuffix(rawBuild(state), "}")
					_, _ = fmt.Fprintf(w, `%s,"blocked":%t}`, body, blocked)
				})
				value, err := env.ExecuteActivity(Activities.SensorBuildkiteBuild,
					GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
				switch {
				case blocked:
					requireReason(t, err, NotReadyReason)
				case state == "failed":
					requireReason(t, err, yarpcerrors.CodeInternal.String())
				default:
					require.NoError(t, err)
					var got BuildResponse
					require.NoError(t, value.Get(&got))
					assert.Equal(t, expectedBuild("passed"), got)
				}
			})
		}
	}
}

func TestNumberedOperationsRejectAnotherBuild(t *testing.T) {
	for _, operation := range []string{"get", "sensor", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, rawBuild("passed"))
			})
			var method any = Activities.GetBuildkiteBuild
			var request any = GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 18}
			switch operation {
			case "sensor":
				method = Activities.SensorBuildkiteBuild
			case "cancel":
				method = Activities.CancelBuildkiteBuild
				request = CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 18}
			}
			_, err := env.ExecuteActivity(method, request)
			requireReason(t, err, yarpcerrors.CodeInternal.String())
		})
	}
}

func TestInvalidInputsDoNotReachBuildkite(t *testing.T) {
	for _, test := range []struct {
		name     string
		function any
		request  any
	}{
		{"trigger pipeline", Activities.TriggerBuildkiteBuild, TriggerBuildRequest{Pipeline: "../other"}},
		{"trigger branch", Activities.TriggerBuildkiteBuild, TriggerBuildRequest{Pipeline: "smoke-test", Commit: "HEAD"}},
		{"trigger commit", Activities.TriggerBuildkiteBuild, TriggerBuildRequest{Pipeline: "smoke-test", Branch: "main"}},
		{"get pipeline", Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "UPPERCASE", BuildNumber: 17}},
		{"get number", Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test"}},
		{"cancel number", Activities.CancelBuildkiteBuild, CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: -1}},
		{"cancel pipeline", Activities.CancelBuildkiteBuild, CancelBuildRequest{Pipeline: "", BuildNumber: 17}},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, _ := activityEnvironment(t, func(http.ResponseWriter, *http.Request) {
				t.Error("invalid input must not reach Buildkite")
			})
			_, err := env.ExecuteActivity(test.function, test.request)
			requireReason(t, err, yarpcerrors.CodeInvalidArgument.String())
		})
	}
}

func TestInvalidEnvironmentKeysNeverSubmit(t *testing.T) {
	for _, key := range []string{"", "1FIRST", "A=B", "A\nB", "naïve", "BUILDKITE_REPO"} {
		t.Run(key, func(t *testing.T) {
			env, _ := activityEnvironment(t, func(http.ResponseWriter, *http.Request) {
				t.Error("invalid environment must not reach Buildkite")
			})
			_, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild, TriggerBuildRequest{
				Pipeline: "smoke-test", Branch: "main", Commit: "HEAD",
				Env: map[string]string{key: "private-value"},
			})
			requireReason(t, err, yarpcerrors.CodeInvalidArgument.String())
		})
	}
}

func TestPipelineAllowlistGuardsEveryOperation(t *testing.T) {
	for _, test := range []struct {
		name, allowed, reason string
	}{
		{"unset", "", yarpcerrors.CodePermissionDenied.String()},
		{"blank", "  ", yarpcerrors.CodePermissionDenied.String()},
		{"denied", "other-pipeline", yarpcerrors.CodePermissionDenied.String()},
		{"wildcard", "*", yarpcerrors.CodeInvalidArgument.String()},
		{"malformed", "smoke-test,../other", yarpcerrors.CodeInvalidArgument.String()},
		{"empty entry", "smoke-test,", yarpcerrors.CodeInvalidArgument.String()},
	} {
		for _, operation := range []struct {
			name            string
			method, request any
		}{
			{"trigger", Activities.TriggerBuildkiteBuild, TriggerBuildRequest{Pipeline: "smoke-test", Branch: "main", Commit: "HEAD"}},
			{"get", Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}},
			{"sensor", Activities.SensorBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}},
			{"cancel", Activities.CancelBuildkiteBuild, CancelBuildRequest{Pipeline: "smoke-test", BuildNumber: 17}},
		} {
			t.Run(test.name+"/"+operation.name, func(t *testing.T) {
				env, client := activityEnvironment(t, func(http.ResponseWriter, *http.Request) {
					t.Error("a denied pipeline must not reach Buildkite")
				})
				t.Setenv("BUILDKITE_ALLOWED_PIPELINES", test.allowed)
				*client = *newClient()
				_, err := env.ExecuteActivity(operation.method, operation.request)
				requireReason(t, err, test.reason)
			})
		}
	}
}

func TestAllowlistWhitespaceDuplicatesAndValidEnvironment(t *testing.T) {
	env, client := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, rawBuild("scheduled"))
	})
	t.Setenv("BUILDKITE_ALLOWED_PIPELINES", "other-pipeline, smoke-test,smoke-test ")
	*client = *newClient()
	_, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild, TriggerBuildRequest{
		Pipeline: "smoke-test", Branch: "main", Commit: "HEAD",
		Env:      map[string]string{"_Release_2": "smoke"},
		Metadata: map[string]string{"BUILDKITE_NOTE": "metadata is not the job environment"},
	})
	require.NoError(t, err)
}

func TestHTTPFailuresAreSanitized(t *testing.T) {
	for _, test := range []struct {
		status int
		reason string
		hint   string
	}{
		{301, yarpcerrors.CodeInternal.String(), ""},
		{400, yarpcerrors.CodeInvalidArgument.String(), ""},
		{401, yarpcerrors.CodeUnauthenticated.String(), "BUILDKITE_API_TOKEN"},
		{403, yarpcerrors.CodePermissionDenied.String(), "read_builds/write_builds"},
		{404, yarpcerrors.CodeNotFound.String(), "pipeline slug"},
		{422, yarpcerrors.CodeInvalidArgument.String(), "branch, commit"},
		{408, yarpcerrors.CodeUnavailable.String(), ""},
		{429, yarpcerrors.CodeUnavailable.String(), ""},
		{503, yarpcerrors.CodeUnavailable.String(), ""},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, `private-upstream-body`)
			})
			_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			requireReason(t, err, test.reason)
			var customError *cadence.CustomError
			require.ErrorAs(t, err, &customError)
			var details string
			require.NoError(t, customError.Details(&details))
			assert.NotContains(t, details, "private")
			assert.NotContains(t, details, "test-token")
			assert.Contains(t, details, test.hint)
		})
	}
}

func TestAmbiguousSubmissionErrorsWarnBeforeRetrying(t *testing.T) {
	for _, status := range []int{408, 429, 502, 504, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private-malformed-acknowledgement")
			})
			_, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild,
				TriggerBuildRequest{Pipeline: "smoke-test", Branch: "main", Commit: "HEAD"})
			var customError *cadence.CustomError
			require.ErrorAs(t, err, &customError)
			var details string
			require.NoError(t, customError.Details(&details))
			assert.Contains(t, details, submissionHint)
			assert.NotContains(t, details, "private")
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

type trackedBody struct {
	io.Reader
	bytesRead int
	closed    bool
}

func (b *trackedBody) Read(data []byte) (int, error) {
	n, err := b.Reader.Read(data)
	b.bytesRead += n
	return n, err
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestHTTPErrorBodyDrainIsBoundedAndClosed(t *testing.T) {
	env, client := activityEnvironment(t, nil)
	body := &trackedBody{Reader: strings.NewReader(strings.Repeat("x", maxErrorBytes*2))}
	client.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: body, Header: make(http.Header)}, nil
	})
	_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild,
		GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	requireReason(t, err, yarpcerrors.CodeUnavailable.String())
	assert.Equal(t, maxErrorBytes, body.bytesRead)
	assert.True(t, body.closed)
}

func TestMalformedOrOversizedResponses(t *testing.T) {
	for _, body := range []string{
		"not-json", "null", "[]", "{}", `{"id":"uuid","number":0,"state":"passed"}`,
		`{"id":"uuid","number":true,"state":"passed"}`,
		`{"id":"","number":17,"state":"passed"}`, `{"id":"uuid","number":17,"state":""}`,
		`{"id":"uuid","number":17,"state":"passed","web_url":false}`,
		strings.Repeat("x", maxResponseBytes+1),
	} {
		t.Run(body[:min(len(body), 60)], func(t *testing.T) {
			env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			})
			_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			requireReason(t, err, yarpcerrors.CodeInternal.String())
		})
	}
}

func TestMissingOptionalFields(t *testing.T) {
	env, _ := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"uuid","number":17,"state":"passed","web_url":null}`)
	})
	value, err := env.ExecuteActivity(Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	require.NoError(t, err)
	var got BuildResponse
	require.NoError(t, value.Get(&got))
	assert.Equal(t, BuildResponse{ID: "uuid", Number: 17, Pipeline: "smoke-test", State: "passed"}, got)
}

func TestConfigurationErrorsAreSafeAndLazy(t *testing.T) {
	for _, test := range []struct{ variable, value string }{
		{"BUILDKITE_ORGANIZATION", ""},
		{"BUILDKITE_ORGANIZATION", "../other"},
		{"BUILDKITE_API_URL", defaultAPIURL},
		{"BUILDKITE_API_URL", "http://example.com/v2"},
		{"BUILDKITE_API_URL", "https://user:private-password@example.com/v2"},
		{"BUILDKITE_API_URL", "https://example.com/v2?private-query"},
		{"BUILDKITE_API_URL", "https://example.com/v2?"},
		{"BUILDKITE_API_URL", "https://example.com/v2#fragment"},
		{"BUILDKITE_API_URL", "file:///etc/passwd"},
		{"BUILDKITE_API_URL", "https://[bad"},
		{"BUILDKITE_API_TOKEN", "private-token\n"},
	} {
		t.Run(test.variable+"="+test.value, func(t *testing.T) {
			env, client := activityEnvironment(t, func(http.ResponseWriter, *http.Request) {
				t.Error("invalid configuration must not reach Buildkite")
			})
			t.Setenv("BUILDKITE_API_TOKEN", "")
			t.Setenv(test.variable, test.value)
			// Construction is intentionally successful even without credentials.
			*client = *newClient()
			_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			requireReason(t, err, yarpcerrors.CodeInvalidArgument.String())
			assert.NotContains(t, err.Error(), "private-")
		})
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return r(request)
}

func TestTransportDeadlineAndCancellation(t *testing.T) {
	for _, test := range []struct {
		err    error
		reason string
	}{
		{errors.New("private-transport-detail"), yarpcerrors.CodeUnavailable.String()},
		{context.DeadlineExceeded, yarpcerrors.CodeDeadlineExceeded.String()},
	} {
		t.Run(test.reason, func(t *testing.T) {
			env, client := activityEnvironment(t, nil)
			var calls int
			client.http.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				assert.True(t, ok)
				assert.LessOrEqual(t, time.Until(deadline), requestTimeout)
				return nil, test.err
			})
			_, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild,
				TriggerBuildRequest{Pipeline: "smoke-test", Branch: "main", Commit: "HEAD"})
			requireReason(t, err, test.reason)
			assert.Equal(t, 1, calls, "the client must not retry an ambiguous mutation")
			var customError *cadence.CustomError
			require.ErrorAs(t, err, &customError)
			var details string
			require.NoError(t, customError.Details(&details))
			assert.Contains(t, details, submissionHint)
			assert.NotContains(t, details, "private-transport-detail")
		})
	}
}

func TestTransportCancellation(t *testing.T) {
	env, client := activityEnvironment(t, nil)
	client.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})
	_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild,
		GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	assert.True(t, cadence.IsCanceledError(err))
}

type failingBody struct {
	err    error
	closed bool
}

func (b *failingBody) Read([]byte) (int, error) { return 0, b.err }
func (b *failingBody) Close() error {
	b.closed = true
	return nil
}

func TestReadFailuresCloseBodyAndPreserveCancellation(t *testing.T) {
	for _, readError := range []error{io.ErrUnexpectedEOF, context.Canceled, context.DeadlineExceeded} {
		t.Run(readError.Error(), func(t *testing.T) {
			env, client := activityEnvironment(t, nil)
			body := &failingBody{err: readError}
			client.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})
			_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild,
				GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			if errors.Is(readError, context.Canceled) {
				assert.True(t, cadence.IsCanceledError(err))
			} else {
				reason := yarpcerrors.CodeUnavailable.String()
				if errors.Is(readError, context.DeadlineExceeded) {
					reason = yarpcerrors.CodeDeadlineExceeded.String()
				}
				requireReason(t, err, reason)
			}
			assert.True(t, body.closed)
		})
	}
}

func TestExplicitProxyDoesNotInventCredentials(t *testing.T) {
	env, client := activityEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, rawBuild("passed"))
	})
	t.Setenv("BUILDKITE_API_TOKEN", "")
	*client = *newClient()
	_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild,
		GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	require.NoError(t, err)
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		forwarded.Add(1)
	}))
	defer target.Close()
	env, _ := activityEnvironment(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, err := env.ExecuteActivity(Activities.TriggerBuildkiteBuild,
		TriggerBuildRequest{Pipeline: "smoke-test", Branch: "main", Commit: "HEAD"})
	requireReason(t, err, yarpcerrors.CodeInternal.String())
	assert.Zero(t, forwarded.Load())
}

type activityTestWorker struct {
	starworker.Worker
	env         *testsuite.TestActivityEnvironment
	temporalEnv *temporaltestsuite.TestActivityEnvironment
}

func (w *activityTestWorker) RegisterActivity(value any) {
	if w.env != nil {
		w.env.RegisterActivity(value)
	} else {
		w.temporalEnv.RegisterActivity(value)
	}
}

type otherBuildActivities struct{}

func (*otherBuildActivities) GetBuild(context.Context, GetBuildRequest) (string, error) {
	return "other integration", nil
}

func (*otherBuildActivities) CancelBuild(context.Context, CancelBuildRequest) (string, error) {
	return "other integration", nil
}

func TestModuleRegistersExecutableActivitiesOnEveryWorker(t *testing.T) {
	t.Setenv("BUILDKITE_ORGANIZATION", "example")
	t.Setenv("BUILDKITE_ALLOWED_PIPELINES", "")
	t.Setenv("BUILDKITE_API_URL", "")
	t.Setenv("BUILDKITE_API_TOKEN", "")
	first, second := newActivityEnvironment(), newActivityEnvironment()
	var temporalSuite temporaltestsuite.WorkflowTestSuite
	temporalEnv := temporalSuite.NewTestActivityEnvironment()
	ctx := context.WithValue(context.Background(), starworkflow.BackendContextKey, startemporal.NewWorkflow())
	temporalEnv.SetWorkerOptions(temporalworker.Options{BackgroundActivityContext: ctx})
	// Temporal registers struct methods under short names. Another integration
	// must be able to use GetBuild/CancelBuild without a duplicate-name panic.
	temporalEnv.RegisterActivity(&otherBuildActivities{})
	app := fxtest.New(t, Module, fx.Supply([]starworker.Worker{
		&activityTestWorker{env: first}, &activityTestWorker{env: second}, &activityTestWorker{temporalEnv: temporalEnv},
	}))
	app.RequireStart()
	defer app.RequireStop()
	for _, env := range []*testsuite.TestActivityEnvironment{first, second} {
		_, err := env.ExecuteActivity(Activities.GetBuildkiteBuild, GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
		// An argument/configuration error proves that each method was registered
		// and executed, without requiring credentials just to start the module.
		requireReason(t, err, yarpcerrors.CodeInvalidArgument.String())
	}
	_, err := temporalEnv.ExecuteActivity("GetBuildkiteBuild", GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	var applicationError *temporal.ApplicationError
	require.ErrorAs(t, err, &applicationError)
	assert.Equal(t, yarpcerrors.CodeInvalidArgument.String(), applicationError.Type())
	_, err = temporalEnv.ExecuteActivity("GetBuild", GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
	require.NoError(t, err)
}

func TestTemporalActivitiesPreserveRetryCategories(t *testing.T) {
	for _, test := range []struct {
		state  string
		status int
		reason string
	}{
		{"passed", 200, ""},
		{"running", 200, NotReadyReason},
		{"failed", 200, yarpcerrors.CodeInternal.String()},
		{"", 403, yarpcerrors.CodePermissionDenied.String()},
		{"", 503, yarpcerrors.CodeUnavailable.String()},
	} {
		t.Run(test.state+http.StatusText(test.status), func(t *testing.T) {
			_, client := activityEnvironment(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, rawBuild(test.state))
			})
			var suite temporaltestsuite.WorkflowTestSuite
			env := suite.NewTestActivityEnvironment()
			ctx := context.WithValue(context.Background(), starworkflow.BackendContextKey, startemporal.NewWorkflow())
			env.SetWorkerOptions(temporalworker.Options{BackgroundActivityContext: ctx})
			env.RegisterActivity(&activities{client: client})
			_, err := env.ExecuteActivity(Activities.SensorBuildkiteBuild,
				GetBuildRequest{Pipeline: "smoke-test", BuildNumber: 17})
			if test.reason == "" {
				require.NoError(t, err)
			} else {
				var applicationError *temporal.ApplicationError
				require.ErrorAs(t, err, &applicationError)
				assert.Equal(t, test.reason, applicationError.Type())
			}
		})
	}
}
