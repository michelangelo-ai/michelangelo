package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	temporalactivity "go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.uber.org/cadence"
	"go.uber.org/yarpc/yarpcerrors"
)

const (
	defaultAPIURL    = "https://api.buildkite.com/v2"
	requestTimeout   = 30 * time.Second
	maxResponseBytes = 8 << 20
	maxErrorBytes    = 4 << 10
	submissionHint   = "; submission outcome may be unknown: check the pipeline's recent builds before retrying"
)

type client struct {
	organization     string
	apiURL           string
	token            string
	allowedPipelines string
	http             *http.Client
}

// buildSnapshot keeps completion metadata inside the activity boundary.
type buildSnapshot struct {
	build   *BuildResponse
	blocked bool
}

func newClient() *client {
	apiURL := os.Getenv("BUILDKITE_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return &client{
		organization:     os.Getenv("BUILDKITE_ORGANIZATION"),
		apiURL:           strings.TrimRight(apiURL, "/"),
		token:            os.Getenv("BUILDKITE_API_TOKEN"),
		allowedPipelines: os.Getenv("BUILDKITE_ALLOWED_PIPELINES"),
		http: &http.Client{
			Timeout: requestTimeout,
			// Do not forward credentials or replay mutations through redirects.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *client) validate() error {
	if c.organization == "" {
		return fmt.Errorf("BUILDKITE_ORGANIZATION is required; set it on the local process or Go worker")
	}
	if err := validateSlug(c.organization); err != nil {
		return fmt.Errorf("BUILDKITE_ORGANIZATION %w", err)
	}
	endpoint, err := url.Parse(c.apiURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil ||
		strings.ContainsAny(c.apiURL, "?#") ||
		strings.ContainsFunc(c.apiURL, func(character rune) bool { return character <= 32 }) {
		return fmt.Errorf("BUILDKITE_API_URL must be an HTTP(S) base URL without credentials, query, or fragment")
	}
	if endpoint.Scheme != "https" {
		host := endpoint.Hostname()
		ip := net.ParseIP(host)
		if endpoint.Scheme != "http" || host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("BUILDKITE_API_URL requires HTTPS except for a loopback proxy")
		}
	}
	if c.apiURL == defaultAPIURL && c.token == "" {
		return fmt.Errorf("BUILDKITE_API_TOKEN is required for the Buildkite API; set it on the local process or Go worker")
	}
	if strings.ContainsFunc(c.token, func(character rune) bool { return character < 32 || character == 127 }) {
		return fmt.Errorf("BUILDKITE_API_TOKEN must not contain control characters")
	}
	return nil
}

func (c *client) authorize(ctx context.Context, pipeline string) error {
	if strings.TrimSpace(c.allowedPipelines) == "" {
		return activityError(ctx, yarpcerrors.CodePermissionDenied.String(),
			"BUILDKITE_ALLOWED_PIPELINES is required; set comma-separated pipeline slugs on the local process or Go worker")
	}
	allowed := false
	for _, entry := range strings.Split(c.allowedPipelines, ",") {
		slug := strings.TrimSpace(entry)
		if err := validateSlug(slug); err != nil {
			return activityError(ctx, yarpcerrors.CodeInvalidArgument.String(),
				"BUILDKITE_ALLOWED_PIPELINES must contain only comma-separated pipeline slugs; wildcards are not supported")
		}
		allowed = allowed || pipeline == slug
	}
	if !allowed {
		return activityError(ctx, yarpcerrors.CodePermissionDenied.String(),
			fmt.Sprintf("pipeline %s is not in BUILDKITE_ALLOWED_PIPELINES", pipeline))
	}
	return nil
}

func (c *client) request(ctx context.Context, method, pipeline, suffix string, payload any) (*buildSnapshot, error) {
	// Configuration is validated on use so installing this optional plugin does
	// not prevent an otherwise unconfigured worker from starting.
	if err := c.validate(); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
	}
	if err := c.authorize(ctx, pipeline); err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), "invalid Buildkite submission")
		}
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	endpoint := c.apiURL + "/organizations/" + c.organization + "/pipelines/" + pipeline + "/builds" + suffix
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), "invalid Buildkite endpoint")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, transportError(ctx, method, pipeline, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Drain only a bounded error body, without logging or returning it.
		// Small error responses can then reuse their keep-alive connection.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBytes))
		return nil, activityError(ctx, httpErrorReason(response.StatusCode),
			httpErrorMessage(method, pipeline, response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, transportError(ctx, method, pipeline, err)
	}
	if len(data) > maxResponseBytes {
		return nil, responseError(ctx, method, "Buildkite response exceeds 8 MiB")
	}
	var build struct {
		ID      string  `json:"id"`
		Number  int64   `json:"number"`
		State   string  `json:"state"`
		WebURL  *string `json:"web_url"`
		Branch  *string `json:"branch"`
		Commit  *string `json:"commit"`
		Blocked bool    `json:"blocked"`
	}
	if err := json.Unmarshal(data, &build); err != nil {
		return nil, responseError(ctx, method, "Buildkite returned invalid build JSON")
	}
	if build.ID == "" || build.Number <= 0 || build.State == "" {
		return nil, responseError(ctx, method, "Buildkite returned an incomplete build")
	}
	return &buildSnapshot{
		build: &BuildResponse{
			ID: build.ID, Number: build.Number, Pipeline: pipeline, State: build.State,
			WebURL: stringValue(build.WebURL), Branch: stringValue(build.Branch), Commit: stringValue(build.Commit),
		},
		blocked: build.Blocked,
	}, nil
}

func transportError(ctx context.Context, method, pipeline string, err error) error {
	if errors.Is(err, context.Canceled) {
		if temporalactivity.IsActivity(ctx) {
			return temporal.NewCanceledError()
		}
		return cadence.NewCanceledError()
	}
	reason := yarpcerrors.CodeUnavailable.String()
	if errors.Is(err, context.DeadlineExceeded) {
		reason = yarpcerrors.CodeDeadlineExceeded.String()
	}
	// Never return the raw transport error, URL, headers, or upstream body.
	message := fmt.Sprintf("Buildkite %s for %s could not complete", method, pipeline)
	if method == http.MethodPost {
		message += submissionHint
	}
	return activityError(ctx, reason, message)
}

func responseError(ctx context.Context, method, message string) error {
	if method == http.MethodPost {
		message += submissionHint
	}
	return activityError(ctx, yarpcerrors.CodeInternal.String(), message)
}

func httpErrorMessage(method, pipeline string, status int) string {
	message := fmt.Sprintf("Buildkite %s for %s failed (HTTP %d)", method, pipeline, status)
	switch status {
	case http.StatusUnauthorized:
		message += ": check that BUILDKITE_API_TOKEN is valid on the local process or Go worker"
	case http.StatusForbidden:
		message += ": check the token's read_builds/write_builds scopes and pipeline access"
	case http.StatusNotFound:
		message += ": check BUILDKITE_ORGANIZATION, the pipeline slug, the build number, and token visibility"
	case http.StatusUnprocessableEntity:
		message += ": Buildkite rejected the request; check the branch, commit, and build parameters"
	}
	if method == http.MethodPost &&
		(status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500) {
		message += submissionHint
	}
	return message
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func httpErrorReason(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return yarpcerrors.CodeUnauthenticated.String()
	case status == http.StatusForbidden:
		return yarpcerrors.CodePermissionDenied.String()
	case status == http.StatusNotFound:
		return yarpcerrors.CodeNotFound.String()
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500:
		return yarpcerrors.CodeUnavailable.String()
	case status >= 400:
		return yarpcerrors.CodeInvalidArgument.String()
	default:
		return yarpcerrors.CodeInternal.String()
	}
}
