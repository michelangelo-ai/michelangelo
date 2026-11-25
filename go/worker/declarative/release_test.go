package declarative

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/syntax"
	"sigs.k8s.io/yaml"

	pipelinedecl "github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
)

const (
	releaseTar    = "testdata/release/core-v0.1.0.tar.gz"
	releaseFile   = "testdata/release/core-v0.1.0.yaml"
	releaseSource = "testdata/release/core-v0.1.0.src.yaml"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func parseRelease(t *testing.T) *pipelinedecl.Release {
	t.Helper()
	r, err := pipelinedecl.ParseRelease("core-v0.1.0", readFile(t, releaseFile))
	require.NoError(t, err)
	return r
}

func TestReleaseDigest(t *testing.T) {
	sum := sha256.Sum256(readFile(t, releaseTar))
	assert.Equal(t, "sha256:"+hex.EncodeToString(sum[:]), parseRelease(t).Digest)
}

func TestReleaseParses(t *testing.T) {
	r := parseRelease(t)
	assert.Equal(t, "core-v0.1.0", r.Version)
	assert.Equal(t, "s3://default/interpreters/core-v0.1.0.tar.gz", r.Artifact)
	require.Contains(t, r.TaskTypes, "core/ray")
	assert.Len(t, r.TaskTypes, 1)
	assert.Equal(t, []string{"retry"}, r.TaskTypes["core/ray"].Capabilities)
}

// TestReleaseLoads runs the tar with no path and no function, so the worker
// takes both from /meta.json. Running main forces every load in the tar.
func TestReleaseLoads(t *testing.T) {
	empty := map[string]any{"schema_version": "v1alpha1", "workflow_config": nil, "parameters": map[string]any{}, "tasks": map[string]any{}}
	res, _, err := runStar(t, readFile(t, releaseTar), "", "", empty, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"states": map[string]any{}, "outputs": map[string]any{}}, toGo(t, res))
}

// sourceType is one task type of a release source file. The source holds the
// factory and job_specs_key, which the release file drops.
type sourceType struct {
	Factory      string                    `json:"factory"`
	JobSpecsKey  string                    `json:"job_specs_key"`
	Capabilities []string                  `json:"capabilities"`
	Config       map[string]any            `json:"config"`
	JobSpecs     map[string]map[string]any `json:"job_specs"`
}

// factoryGaps returns, per task type, the kwargs that the engine passes to
// the factory, and the ones that the factory does not declare as named
// parameters. A **kwargs parameter does not count: it would hide a typo.
func factoryGaps(t *testing.T, tar, source []byte) (passed, missing map[string][]string) {
	t.Helper()
	var src struct {
		TaskTypes map[string]sourceType `json:"task_types"`
	}
	require.NoError(t, yaml.Unmarshal(source, &src))
	members := map[string][]byte{}
	require.NoError(t, ext.ReadTar(bytes.NewReader(tar), func(name string, content []byte) error {
		members[name] = content
		return nil
	}))

	passed, missing = map[string][]string{}, map[string][]string{}
	for name, tt := range src.TaskTypes {
		file, fn, ok := strings.Cut(tt.Factory, ":")
		require.True(t, ok, tt.Factory)
		params := factoryParams(t, "/"+file, members["/"+file], fn)

		var want []string
		for f := range tt.Config {
			if f != "entry_point" && f != "args" && f != "kwargs" {
				want = append(want, f)
			}
		}
		for f := range tt.JobSpecs[tt.JobSpecsKey] {
			want = append(want, f)
		}
		want = append(want, "alias")
		for _, c := range tt.Capabilities {
			if c == "retry" {
				want = append(want, "retry_attempts")
			}
		}
		sort.Strings(want)
		passed[name] = want
		for _, w := range want {
			if !params[w] {
				missing[name] = append(missing[name], w)
			}
		}
	}
	return passed, missing
}

// factoryParams returns the named parameters of the def fn in file.
func factoryParams(t *testing.T, file string, src []byte, fn string) map[string]bool {
	t.Helper()
	require.NotNil(t, src, "%s is not in the tar", file)
	f, err := syntax.Parse(file, src, 0)
	require.NoError(t, err)
	for _, stmt := range f.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok || def.Name.Name != fn {
			continue
		}
		params := map[string]bool{}
		for _, p := range def.Params {
			switch p := p.(type) {
			case *syntax.Ident:
				params[p.Name] = true
			case *syntax.BinaryExpr:
				params[p.X.(*syntax.Ident).Name] = true
			}
		}
		return params
	}
	t.Fatalf("%s has no def %s", file, fn)
	return nil
}

func TestReleaseFactoryConformance(t *testing.T) {
	passed, missing := factoryGaps(t, readFile(t, releaseTar), readFile(t, releaseSource))
	assert.Equal(t, map[string][]string{"core/ray": {"alias", "head_cpu", "retry_attempts", "worker_instances"}}, passed)
	assert.Empty(t, missing, "kwargs that the factory does not declare")
}
