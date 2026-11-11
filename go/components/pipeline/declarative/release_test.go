package declarative

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReleases(t *testing.T) {
	rs, err := LoadReleases("testdata/releases")
	require.NoError(t, err)
	require.Contains(t, rs, "core-v1.4.2")
	r := rs["core-v1.4.2"]
	assert.Equal(t, "s3://ma-artifacts/interpreters/core-v1.4.2.tar.gz", r.Artifact)
	ray := r.TaskTypes["core/ray"]
	require.NotNil(t, ray)
	assert.True(t, ray.HasCapability("retry"))
	assert.False(t, ray.HasCapability("gpu"))
	assert.Equal(t, &Field{Type: "string", Required: true}, ray.Config["entry_point"])
	assert.Equal(t, &Field{Fields: Schema{"head_cpu": {Type: "integer"}, "image": {Type: "string"}}}, ray.JobSpecs["ray"])
	assert.False(t, r.TaskTypes["core/python"].HasCapability("retry"))
}

func TestLoadReleasesErrors(t *testing.T) {
	cases := []struct{ dir, want string }{
		{"testdata/releases-bad-name", `release file "core-v9.yaml" declares version "core-v1.4.2"; the file name must equal the version`},
		{"testdata/releases-dup", `release "core-v1" is declared by more than one file`},
		{"testdata/releases-bad-type", `release "core-v2": task type "core/python" config field "x" has unknown type "float"; use string, integer, number, boolean, list, or dict`},
		{"testdata/does-not-exist", "open testdata/does-not-exist: no such file or directory"},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			_, err := LoadReleases(c.dir)
			assert.EqualError(t, err, c.want)
		})
	}
}

func TestParseReleaseErrors(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"no version", "artifact: a\ndigest: sha256:ab\ntask_types: {}\n", `release "r": version is not set`},
		{"no artifact", "version: r\ndigest: sha256:ab\ntask_types: {}\n", `release "r": artifact is not set`},
		{"bad digest", "version: r\nartifact: a\ndigest: ab\ntask_types: {}\n", `release "r": digest must start with "sha256:"`},
		{"no task types", "version: r\nartifact: a\ndigest: sha256:ab\n", `release "r": task_types is empty`},
		{"unknown key", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types: {}\nextra: 1\n", `release "r": error unmarshaling JSON: while decoding JSON: json: unknown field "extra"`},
		{"leaf with children", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: {type: string, y: {type: string}}\n", `release "r": task type "t" config field "x" mixes type with nested fields`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseRelease("r", []byte(c.yaml))
			assert.EqualError(t, err, c.want)
		})
	}
}
