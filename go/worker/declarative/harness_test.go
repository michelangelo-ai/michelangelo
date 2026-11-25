package declarative

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cadence-workflow/starlark-worker/cadence"
	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/plugin"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/worker"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"go.uber.org/cadence/encoded"
	"go.uber.org/cadence/testsuite"
	cadworker "go.uber.org/cadence/worker"
	cadworkflow "go.uber.org/cadence/workflow"

	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/cachedoutput"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/ray"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/spark"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/storage"
)

func workerPlugins() map[string]service.IPlugin {
	r := map[string]service.IPlugin{}
	for id, p := range plugin.Registry {
		r[id] = p
	}
	for _, p := range []service.IPlugin{storage.Plugin, cachedoutput.Plugin, ray.Plugin, spark.Plugin} {
		r[p.ID()] = p
	}
	return r
}

func cadenceWorkerOptions() cadworker.Options {
	ctx := context.WithValue(context.Background(), workflow.BackendContextKey, cadence.NewWorkflow())
	return cadworker.Options{DataConverter: &cadence.DataConverter{}, BackgroundActivityContext: ctx}
}

type reg struct {
	env *testsuite.TestWorkflowEnvironment
}

func (r reg) RegisterActivity(a interface{}) { r.env.RegisterActivity(a) }
func (r reg) RegisterActivityWithOptions(a interface{}, _ worker.RegisterActivityOptions) {
	r.env.RegisterActivity(a)
}
func (r reg) RegisterWorkflow(w interface{}, _ string) {
	r.env.RegisterWorkflow(cadence.UpdateWorkflowFunctionContextArgument(w))
}
func (r reg) RegisterWorkflowWithOptions(w interface{}, o worker.RegisterWorkflowOptions) {
	r.env.RegisterWorkflowWithOptions(cadence.UpdateWorkflowFunctionContextArgument(w), cadworkflow.RegisterOptions{Name: o.Name})
}

// runStar starts the worker's Starlark workflow the way the run controller
// does. The controller's Cadence client sets no DataConverter, so its inputs
// are encoded by the Cadence default JSON converter, while the worker decodes
// them with the starlark-worker converter. Encoding here with the default
// converter and passing the bytes through unchanged keeps that asymmetry:
// the test env's own converter would encode Go maps with jsoniter, which
// does not sort map keys.
func runStar(t *testing.T, tar []byte, path, fn string, args ...any) (*starlark.Dict, error) {
	t.Helper()
	svc, err := service.NewServiceBuilder(service.CadenceBackend).SetPlugins(workerPlugins()).SetClientTaskList("test").Build()
	require.NoError(t, err)
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()
	env.SetWorkerOptions(cadenceWorkerOptions())
	svc.RegisterWithOptions(reg{env: env}, worker.RegisterWorkflowOptions{Name: service.ServiceWorkflowFunc})

	input, err := encoded.GetDefaultDataConverter().ToData(tar, path, fn, args, []any{}, map[string]any{})
	require.NoError(t, err)
	env.ExecuteWorkflow(service.ServiceWorkflowFunc, input)
	require.True(t, env.IsWorkflowCompleted())
	if err := env.GetWorkflowError(); err != nil {
		return nil, err
	}
	var res *starlark.Dict
	require.NoError(t, env.GetWorkflowResult(&res))
	return res, nil
}

// dirTar tars dir with every relative load made absolute. The worker anchors
// every load at the tar root, so a file-relative load only resolves after
// this rewrite, which build.py also applies when it packages a workflow.
func dirTar(t *testing.T, dir string) []byte {
	t.Helper()
	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if strings.HasSuffix(name, ".star") {
			if src, err = rewriteLoads("/"+name, src); err != nil {
				return err
			}
		}
		files[name] = src
		return nil
	}))
	var buf bytes.Buffer
	require.NoError(t, ext.WriteTar(files, &buf))
	return buf.Bytes()
}

// rewriteLoads makes each load module of file, which is not "@"-prefixed or
// absolute, an absolute path resolved against the directory of file.
func rewriteLoads(file string, src []byte) ([]byte, error) {
	f, err := syntax.Parse(file, src, 0)
	if err != nil {
		return nil, err
	}
	var loads []*syntax.Literal
	for _, stmt := range f.Stmts {
		if l, ok := stmt.(*syntax.LoadStmt); ok {
			m := l.Module.Value.(string)
			if !strings.HasPrefix(m, "@") && !strings.HasPrefix(m, "/") {
				loads = append(loads, l.Module)
			}
		}
	}
	out := src
	// Splice back to front so that earlier offsets stay valid.
	for i := len(loads) - 1; i >= 0; i-- {
		lit := loads[i]
		module := lit.Value.(string)
		rel := path.Join(strings.TrimPrefix(path.Dir(file), "/"), module)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("%s: load %q leaves the tar root", file, module)
		}
		joined := "/" + rel
		off := byteOffset(src, lit.TokenPos)
		if string(src[off:off+len(lit.Raw)]) != lit.Raw {
			return nil, fmt.Errorf("%s:%d: load literal not found at its position", file, lit.TokenPos.Line)
		}
		out = append(append(append([]byte{}, out[:off]...), strconv.Quote(joined)...), out[off+len(lit.Raw):]...)
	}
	return out, nil
}

// byteOffset converts a Starlark position, whose column counts runes, to a
// byte offset in src.
func byteOffset(src []byte, pos syntax.Position) int {
	off := 0
	for line := int32(1); line < pos.Line; line++ {
		off += bytes.IndexByte(src[off:], '\n') + 1
	}
	for col := int32(1); col < pos.Col; col++ {
		_, size := utf8.DecodeRune(src[off:])
		off += size
	}
	return off
}

// toGo converts a Starlark result of strings, lists, and dicts to Go values
// so that a test can compare it with a literal.
func toGo(t *testing.T, v starlark.Value) any {
	t.Helper()
	switch v := v.(type) {
	case starlark.String:
		return string(v)
	case *starlark.List:
		out := []any{}
		for i := 0; i < v.Len(); i++ {
			out = append(out, toGo(t, v.Index(i)))
		}
		return out
	case *starlark.Dict:
		out := map[string]any{}
		for _, item := range v.Items() {
			k, ok := item[0].(starlark.String)
			require.True(t, ok, "dict key %s is not a string", item[0])
			out[string(k)] = toGo(t, item[1])
		}
		return out
	}
	t.Fatalf("unsupported Starlark value %s (%s)", v, v.Type())
	return nil
}

func TestProbeInputShapes(t *testing.T) {
	spec := map[string]any{"n": float64(3), "x": 1.5, "nil": nil, "m": map[string]any{"k": "v"}, "l": []any{}}
	res, err := runStar(t, dirTar(t, "testdata/root"), "/probe.star", "probe", spec, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"keys":   []any{"l", "m", "n", "nil", "x"},
		"spec":   map[string]any{"n": "int", "x": "float", "nil": "NoneType", "m": "dict", "l": "list"},
		"params": "dict",
	}, toGo(t, res))
}

func TestRewriteLoads(t *testing.T) {
	src := "load(\"@plugin\", \"ray\")\nload(\"../commons.star\", \"A\")\nE = \"é\"; load('../../x/y.star', \"B\")\nload(\"/abs.star\", \"C\")\n"
	got, err := rewriteLoads("/michelangelo/uniflow/declarative/engine.star", []byte(src))
	require.NoError(t, err)
	assert.Equal(t, "load(\"@plugin\", \"ray\")\nload(\"/michelangelo/uniflow/commons.star\", \"A\")\nE = \"é\"; load(\"/michelangelo/x/y.star\", \"B\")\nload(\"/abs.star\", \"C\")\n", string(got))

	_, err = rewriteLoads("/a.star", []byte(`load("../b.star", "B")`))
	assert.EqualError(t, err, `/a.star: load "../b.star" leaves the tar root`)
}

// The worker tars a test root with WalkDir, which does not descend into a
// directory symlink, so a linked directory must sit outside the tarred root.
func TestDirSymlinkResolves(t *testing.T) {
	entries, err := os.ReadDir("testdata/dirlink/uniflow")
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"commons.star", "declarative"}, names)

	src, err := os.ReadFile("testdata/dirlink/uniflow/declarative/engine.star")
	require.NoError(t, err)
	firstLine, _, _ := strings.Cut(string(src), "\n")
	assert.Equal(t, `load("../commons.star", "TASK_STATE_SUCCEEDED")`, firstLine)
}
