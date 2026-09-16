// Package depsimage resolves ServingSpec.python_dependencies to a shared,
// content-addressed serving image instead of installing packages into a
// running pod.
//
// Two InferenceServers -- from any project, anywhere in the repo -- that
// declare the same dependency list resolve to the same image tag. Nothing
// here is keyed by project name or reads from a project directory.
package depsimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// imageRepository is the GHCR repository path (owner/name) that
	// build-inferenceserver-deps-image.yaml pushes to.
	imageRepository = "michelangelo-ai/inferenceserver-deps"

	// githubOwnerRepo is the GitHub repository (owner/name) whose
	// build-inferenceserver-deps-image.yaml workflow gets dispatched.
	githubOwnerRepo = "michelangelo-ai/michelangelo"

	// workflowFile is the workflow_dispatch target.
	workflowFile = "build-inferenceserver-deps-image.yaml"

	// githubTokenEnvVar names the environment variable the controller reads a
	// workflow_dispatch-capable GitHub token from. Must be provisioned as a
	// Kubernetes Secret and mounted into the controller pod -- see the PR
	// description for why this can't reuse a workflow's ambient GITHUB_TOKEN.
	githubTokenEnvVar = "INFERENCESERVER_DEPS_IMAGE_GITHUB_TOKEN"

	// dispatchDebounce is how long TriggerBuild suppresses a repeat dispatch
	// for the same tag, so a busy reconcile loop (default requeue: 1 minute)
	// doesn't spam workflow_dispatch calls while a build is already running.
	dispatchDebounce = 5 * time.Minute
)

// registryBaseURL and githubAPIBaseURL are overridden in tests to point at a
// local httptest.Server instead of the real GHCR/GitHub API.
var (
	registryBaseURL  = "https://ghcr.io"
	githubAPIBaseURL = "https://api.github.com"
)

// Image returns the fully-qualified image reference for a dependency set,
// e.g. "ghcr.io/michelangelo-ai/inferenceserver-deps:<tag>".
func Image(deps []string) string {
	return fmt.Sprintf("ghcr.io/%s:%s", imageRepository, Tag(deps))
}

// Tag canonicalizes deps (trimmed, deduplicated, sorted) and returns a short
// content hash suitable as an image tag. Order and duplicate entries in the
// input don't affect the result.
func Tag(deps []string) string {
	seen := make(map[string]struct{}, len(deps))
	var canonical []string
	for _, d := range deps {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		canonical = append(canonical, d)
	}
	sort.Strings(canonical)

	sum := sha256.Sum256([]byte(strings.Join(canonical, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

// Exists reports whether the dependency image for tag has already been
// built and pushed to the registry.
func Exists(ctx context.Context, httpClient *http.Client, tag string) (bool, error) {
	token, err := anonymousPullToken(ctx, httpClient)
	if err != nil {
		return false, fmt.Errorf("failed to get registry pull token: %w", err)
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registryBaseURL, imageRepository, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to build manifest HEAD request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check manifest for %s: %w", tag, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status %d checking manifest for %s", resp.StatusCode, tag)
	}
}

// anonymousPullToken gets a short-lived, unauthenticated GHCR pull token.
// Works for public packages; a private inferenceserver-deps package would
// need the controller's own registry credential here instead.
func anonymousPullToken(ctx context.Context, httpClient *http.Client) (string, error) {
	url := fmt.Sprintf("%s/token?service=ghcr.io&scope=repository:%s:pull", registryBaseURL, imageRepository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d fetching pull token: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode pull token response: %w", err)
	}
	return parsed.Token, nil
}

var (
	dispatchedMu sync.Mutex
	dispatchedAt = map[string]time.Time{}
)

// TriggerBuild kicks off a workflow_dispatch of build-inferenceserver-deps-image.yaml
// to build and push the image for tag. Debounced per-tag so repeated calls
// while a build is in flight don't dispatch duplicate runs.
func TriggerBuild(ctx context.Context, httpClient *http.Client, deps []string, tag string) error {
	dispatchedMu.Lock()
	if last, ok := dispatchedAt[tag]; ok && time.Since(last) < dispatchDebounce {
		dispatchedMu.Unlock()
		return nil
	}
	dispatchedAt[tag] = time.Now()
	dispatchedMu.Unlock()

	token := os.Getenv(githubTokenEnvVar)
	if token == "" {
		return fmt.Errorf("%s is not set; a GitHub token with actions:write on %s must be mounted into the controller", githubTokenEnvVar, githubOwnerRepo)
	}

	body, err := json.Marshal(map[string]any{
		"ref": "main",
		"inputs": map[string]string{
			"dependencies": strings.Join(deps, "\n"),
			"tag":          tag,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal workflow_dispatch payload: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/dispatches", githubAPIBaseURL, githubOwnerRepo, workflowFile)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to build workflow_dispatch request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch build for %s: %w", tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d dispatching build for %s: %s", resp.StatusCode, tag, string(respBody))
	}
	return nil
}
