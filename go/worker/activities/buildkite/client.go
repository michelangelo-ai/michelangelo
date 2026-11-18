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
)

type client struct {
	organization string
	apiURL       string
	token        string
	http         *http.Client
}

func newClient() *client {
	apiURL := os.Getenv("BUILDKITE_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return &client{
		organization: os.Getenv("BUILDKITE_ORGANIZATION"),
		apiURL:       strings.TrimRight(apiURL, "/"),
		token:        os.Getenv("BUILDKITE_API_TOKEN"),
		http: &http.Client{
			Timeout: requestTimeout,
			// Do not forward credentials or replay mutations through redirects.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *client) validate() error {
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
		return fmt.Errorf("BUILDKITE_API_TOKEN is required for the Buildkite API")
	}
	if strings.ContainsFunc(c.token, func(character rune) bool { return character < 32 || character == 127 }) {
		return fmt.Errorf("BUILDKITE_API_TOKEN must not contain control characters")
	}
	return nil
}

func (c *client) request(ctx context.Context, method, pipeline, suffix string, payload any) (*BuildResponse, error) {
	// Configuration is validated on use so installing this optional plugin does
	// not prevent an otherwise unconfigured worker from starting.
	if err := c.validate(); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInvalidArgument.String(), err.Error())
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
		return nil, transportError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, activityError(ctx, httpErrorReason(response.StatusCode),
			fmt.Sprintf("Buildkite request failed (HTTP %d)", response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, transportError(ctx, err)
	}
	if len(data) > maxResponseBytes {
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(), "Buildkite response exceeds 8 MiB")
	}
	var build struct {
		ID     string  `json:"id"`
		Number int64   `json:"number"`
		State  string  `json:"state"`
		WebURL *string `json:"web_url"`
		Branch *string `json:"branch"`
		Commit *string `json:"commit"`
	}
	if err := json.Unmarshal(data, &build); err != nil {
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(), "Buildkite returned invalid build JSON")
	}
	if build.ID == "" || build.Number <= 0 || build.State == "" {
		return nil, activityError(ctx, yarpcerrors.CodeInternal.String(), "Buildkite returned an incomplete build")
	}
	return &BuildResponse{
		ID: build.ID, Number: build.Number, Pipeline: pipeline, State: build.State,
		WebURL: stringValue(build.WebURL), Branch: stringValue(build.Branch), Commit: stringValue(build.Commit),
	}, nil
}

func transportError(ctx context.Context, err error) error {
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
	return activityError(ctx, reason, "Buildkite request could not complete")
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
