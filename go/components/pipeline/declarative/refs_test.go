package declarative

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRefs(t *testing.T) {
	cases := []struct {
		in   string
		want []ref
	}{
		{"plain", nil},
		{"{{ params.epochs }}", []ref{{Kind: refParam, Name: "epochs", Whole: true}}},
		{"{{params.epochs}}", []ref{{Kind: refParam, Name: "epochs", Whole: true}}},
		{"{{ tasks.extract.output }}", []ref{{Kind: refTaskOutput, Name: "extract", Path: nil, Whole: true}}},
		{"{{ tasks.extract.output.path }}", []ref{{Kind: refTaskOutput, Name: "extract", Path: []string{"path"}, Whole: true}}},
		{"{{ tasks.extract.output.a.b }}", []ref{{Kind: refTaskOutput, Name: "extract", Path: []string{"a", "b"}, Whole: true}}},
		{"gs://{{ params.bucket }}/{{ tasks.extract.output.path }}", []ref{
			{Kind: refParam, Name: "bucket"},
			{Kind: refTaskOutput, Name: "extract", Path: []string{"path"}},
		}},
		{" {{ params.x }} ", []ref{{Kind: refParam, Name: "x"}}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseRefs(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestParseRefsErrors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"{{ params.x", `unterminated "{{" in "{{ params.x"`},
		{"a {{ b", `unterminated "{{" in "a {{ b"`},
		{"{{ env.HOME }}", `reference "{{ env.HOME }}" must start with "params." or "tasks."`},
		{"{{ tasks.extract }}", `reference "{{ tasks.extract }}" must name "tasks.extract.output"`},
		{"{{ tasks.extract.result }}", `reference "{{ tasks.extract.result }}" must name "tasks.extract.output"`},
		{"{{ params.Bad-Name }}", `reference "{{ params.Bad-Name }}" has a malformed name`},
		{"{{ params. }}", `reference "{{ params. }}" has a malformed name`},
		{"{{ }}", `reference "{{ }}" must start with "params." or "tasks."`},
		{"{{ tasks.Bad.output }}", `reference "{{ tasks.Bad.output }}" has a malformed name`},
		{"{{ tasks.a.output.1x }}", `reference "{{ tasks.a.output.1x }}" has a malformed name`},
		{"{{ tasks.a.output. }}", `reference "{{ tasks.a.output. }}" has a malformed name`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			_, err := parseRefs(c.in)
			assert.EqualError(t, err, c.want)
		})
	}
}
