package depsimage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTag_OrderAndDuplicatesDontAffectResult(t *testing.T) {
	a := Tag([]string{"torch==2.4.1", "transformers==4.44.2"})
	b := Tag([]string{"transformers==4.44.2", "torch==2.4.1"})
	c := Tag([]string{"torch==2.4.1", "torch==2.4.1", "transformers==4.44.2", " "})

	assert.Equal(t, a, b, "order shouldn't affect the tag")
	assert.Equal(t, a, c, "duplicates/blank entries shouldn't affect the tag")
}

func TestTag_DifferentDepsProduceDifferentTags(t *testing.T) {
	a := Tag([]string{"torch==2.4.1"})
	b := Tag([]string{"torch==2.5.0"})
	assert.NotEqual(t, a, b)
}

func TestImage_UsesGHCRNamespaceAndTag(t *testing.T) {
	deps := []string{"torch==2.4.1"}
	assert.Equal(t, "ghcr.io/michelangelo-ai/inferenceserver-deps:"+Tag(deps), Image(deps))
}

func TestExists(t *testing.T) {
	for _, tc := range []struct {
		name       string
		manifest   int
		wantExists bool
		wantErr    bool
	}{
		{name: "found", manifest: http.StatusOK, wantExists: true},
		{name: "not found", manifest: http.StatusNotFound, wantExists: false},
		{name: "registry error", manifest: http.StatusInternalServerError, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/token":
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"token": "fake-token"})
				case r.Method == http.MethodHead:
					w.WriteHeader(tc.manifest)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			origRegistry := registryBaseURL
			registryBaseURL = server.URL
			defer func() { registryBaseURL = origRegistry }()

			exists, err := Exists(context.Background(), server.Client(), "abc123")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantExists, exists)
		})
	}
}

func TestTriggerBuild_RequiresToken(t *testing.T) {
	t.Setenv(githubTokenEnvVar, "")
	err := TriggerBuild(context.Background(), http.DefaultClient, []string{"torch==2.4.1"}, "sometag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), githubTokenEnvVar)
}

func TestTriggerBuild_SendsExpectedRequestAndDebounces(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var payload struct {
			Ref    string            `json:"ref"`
			Inputs map[string]string `json:"inputs"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "main", payload.Ref)
		assert.Equal(t, "uniquetag123", payload.Inputs["tag"])
		assert.Equal(t, "torch==2.4.1", payload.Inputs["dependencies"])

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	origAPI := githubAPIBaseURL
	githubAPIBaseURL = server.URL
	defer func() { githubAPIBaseURL = origAPI }()
	t.Setenv(githubTokenEnvVar, "test-token")

	require.NoError(t, TriggerBuild(context.Background(), server.Client(), []string{"torch==2.4.1"}, "uniquetag123"))
	require.NoError(t, TriggerBuild(context.Background(), server.Client(), []string{"torch==2.4.1"}, "uniquetag123"))
	assert.Equal(t, 1, calls, "second call within the debounce window should be suppressed")
}
