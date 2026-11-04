package pipeline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// TestCommitMessageMaxLength checks that CommitInfo.message is bounded on write. The API
// handler runs api.Validate on every create and update, so an over-long message is rejected
// before it is copied into the Pipeline, Revision and PipelineRun objects.
func TestCommitMessageMaxLength(t *testing.T) {
	const maxMessageBytes = 8192

	pipelineWith := func(message string) *v2pb.Pipeline {
		return &v2pb.Pipeline{
			Spec: v2pb.PipelineSpec{
				Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main", Message: message},
			},
		}
	}

	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "empty message is allowed", message: ""},
		{name: "message at the limit is allowed", message: strings.Repeat("a", maxMessageBytes)},
		{name: "message over the limit is rejected", message: strings.Repeat("a", maxMessageBytes+1), wantErr: true},
		{
			// The limit counts bytes, not characters: 4096 two-byte runes is exactly 8192 bytes.
			name:    "multi-byte message is measured in bytes",
			message: strings.Repeat("é", maxMessageBytes/2+1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.Validate(pipelineWith(tt.message))
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Contains(t, err.Error(), "message")
		})
	}
}

// TestCommitURLValidation checks that CommitInfo.url is empty or an http(s) URL, so a stored
// javascript: or data: URL cannot reach a consumer that renders it as a link.
func TestCommitURLValidation(t *testing.T) {
	pipelineWith := func(url string) *v2pb.Pipeline {
		return &v2pb.Pipeline{
			Spec: v2pb.PipelineSpec{
				Commit: &v2pb.CommitInfo{GitRef: "abc123", Branch: "main", Url: url},
			},
		}
	}

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "empty url is allowed", url: ""},
		{name: "https url is allowed", url: "https://github.com/org/repo/commit/abc123"},
		{name: "http url is allowed", url: "http://git.example.com/commit/abc123"},
		{name: "scheme is case-insensitive", url: "HTTPS://github.com/org/repo/commit/abc123"},
		{name: "javascript scheme is rejected", url: "javascript:alert(1)", wantErr: true},
		{name: "data scheme is rejected", url: "data:text/html,<script>alert(1)</script>", wantErr: true},
		{name: "ftp scheme is rejected", url: "ftp://example.com/commit/abc123", wantErr: true},
		{name: "relative url is rejected", url: "/org/repo/commit/abc123", wantErr: true},
		{name: "scheme only is rejected", url: "https://", wantErr: true},
		{name: "whitespace is rejected", url: "https://example.com/a b", wantErr: true},
		{name: "url over 2048 bytes is rejected", url: "https://example.com/" + strings.Repeat("a", 2048), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := api.Validate(pipelineWith(tt.url))
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Contains(t, err.Error(), "url")
		})
	}
}
