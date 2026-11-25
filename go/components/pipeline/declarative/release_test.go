package declarative

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
		{"testdata/releases-dup", `release "core-v1" is declared by both "core-v1.yaml" and "core-v1.yml"; keep one`},
		{"testdata/releases-bad-type", `release file "core-v2.yaml": release "core-v2": task type "core/python" config field "x" has unknown type "float"; use string, integer, number, boolean, list, or dict`},
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
		{"empty type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: {type: \"\"}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "x": type must be a non-empty string`},
		{"null type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: {type: null}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "x": type must be a non-empty string`},
		{"nested null field", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      res: {cpu: null}\n", `release "r": task type "t" config field "res.cpu" is empty`},
		{"nested bad type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      res: {cpu: {type: float}}\n", `release "r": task type "t" config field "res.cpu" has unknown type "float"; use string, integer, number, boolean, list, or dict`},
		{"nested empty type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      res: {cpu: {type: \"\"}}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "res.cpu": type must be a non-empty string`},
		{"required not bool", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: {type: string, required: \"yes\"}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "x": required must be a boolean`},
		{"child named type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      res: {type: {type: string}}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "res": "type" is reserved for the field type; a group cannot have a child named "type"`},
		{"required on group", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      res: {required: true, cpu: {type: integer}}\n", `release "r": error unmarshaling JSON: while decoding JSON: field "res": "required" is reserved; set it next to "type" on a leaf field`},
		{"field not object", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: string\n", `release "r": error unmarshaling JSON: while decoding JSON: field "x": must be an object, not a string`},
		{"nil task type", "version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n", `release "r": task type "t" is empty`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseRelease("r", []byte(c.yaml))
			assert.EqualError(t, err, c.want)
		})
	}
}

func TestParseReleaseRejectsDeepSchema(t *testing.T) {
	var b strings.Builder
	b.WriteString("version: r\nartifact: a\ndigest: sha256:ab\ntask_types:\n  t:\n    config:\n      x: ")
	for i := 0; i < maxSchemaDepth+1; i++ {
		b.WriteString("{g: ")
	}
	b.WriteString("{type: string}")
	for i := 0; i < maxSchemaDepth+1; i++ {
		b.WriteString("}")
	}
	_, err := ParseRelease("r", []byte(b.String()+"\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema nests more than 32 levels")
}

func TestReleaseJSONRoundTrip(t *testing.T) {
	rs, err := LoadReleases("testdata/releases")
	require.NoError(t, err)
	rel := rs["core-v1.4.2"]
	b, err := json.Marshal(rel)
	require.NoError(t, err)
	got := &Release{}
	require.NoError(t, json.Unmarshal(b, got))
	assert.Equal(t, rel, got)
}

func TestLoadReleasesMissingDir(t *testing.T) {
	_, err := LoadReleases(filepath.Join(t.TempDir(), "missing"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func copyRelease(t *testing.T, dir string) {
	t.Helper()
	src, err := os.ReadFile("testdata/releases/core-v1.4.2.yaml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core-v1.4.2.yaml"), src, 0o600))
}

func TestLoadReleasesSkipsNonYAMLEntries(t *testing.T) {
	dir := t.TempDir()
	copyRelease(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "old.yaml"), 0o700))
	rs, err := LoadReleases(dir)
	require.NoError(t, err)
	assert.Len(t, rs, 1)
}

func TestLoadReleasesFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "..data")
	require.NoError(t, os.Mkdir(data, 0o700))
	copyRelease(t, data)
	require.NoError(t, os.Symlink(filepath.Join("..data", "core-v1.4.2.yaml"), filepath.Join(dir, "core-v1.4.2.yaml")))
	rs, err := LoadReleases(dir)
	require.NoError(t, err)
	assert.Contains(t, rs, "core-v1.4.2")
}

// Kubelet mounts a ConfigMap as a timestamped directory, a "..data" symlink
// to it, and one symlink per key into "..data". A change to Lstat would
// break every deployment, so this test pins the layout.
func TestLoadReleasesReadsConfigMapVolumeLayout(t *testing.T) {
	dir := t.TempDir()
	ts := filepath.Join(dir, "..2026_11_24_10_00_00.000000001")
	require.NoError(t, os.Mkdir(ts, 0o700))
	copyRelease(t, ts)
	require.NoError(t, os.Symlink(filepath.Base(ts), filepath.Join(dir, "..data")))
	require.NoError(t, os.Symlink(filepath.Join("..data", "core-v1.4.2.yaml"), filepath.Join(dir, "core-v1.4.2.yaml")))
	rs, err := LoadReleases(dir)
	require.NoError(t, err)
	assert.Len(t, rs, 1)
	assert.Contains(t, rs, "core-v1.4.2")
}

func TestLoadReleasesRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "v1.yaml"), 0o600))
	_, err := LoadReleases(dir)
	assert.EqualError(t, err, `release file "v1.yaml": is not a regular file`)
}

func TestLoadReleasesRejectsLargeFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "v1.yaml"), make([]byte, maxReleaseFileBytes+1), 0o600))
	_, err := LoadReleases(dir)
	assert.EqualError(t, err, `release file "v1.yaml": is larger than 1048576 bytes`)
}
