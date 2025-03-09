package star

import (
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"path/filepath"
	"strings"
)

const PluginPrefix = "@"

var FileOptions = &syntax.FileOptions{}

type FS interface {
	Read(path string) ([]byte, error)
}

func ThreadLoad(
	fs FS,
	predeclared starlark.StringDict,
	modules map[string]starlark.StringDict,
) func(*starlark.Thread, string) (starlark.StringDict, error) {
	return func(t *starlark.Thread, path string) (starlark.StringDict, error) {
		if strings.HasPrefix(path, PluginPrefix) {
			if mod, found := modules[path[1:]]; !found {
				return nil, fmt.Errorf("module-not-found: %v", path)
			} else {
				return mod, nil
			}
		}
		// TODO: andrii: support relative path (relative to the thread's call stack)
		if src, err := fs.Read(path); err != nil {
			return nil, err
		} else if strings.HasSuffix(path, ".star") || strings.HasSuffix(path, ".py") {
			return starlark.ExecFileOptions(FileOptions, t, path, src, predeclared)
		} else {
			return loadDataFile(path, src)
		}
	}
}

func Call(
	t *starlark.Thread,
	path string,
	function string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	globals, err := t.Load(t, path)
	if err != nil {
		return nil, err
	}
	fn := globals[function]
	if fn == nil {
		err := &starlark.EvalError{
			Msg:       fmt.Sprintf("function not found: %s, available globals: %s", function, globals.Keys()),
			CallStack: t.CallStack(),
		}
		return nil, err
	}
	return starlark.Call(t, fn, args, kwargs)
}

func loadDataFile(path string, data []byte) (starlark.StringDict, error) {
	ext := filepath.Ext(path)
	switch ext {
	case ".txt":
		return starlark.StringDict{"txt": starlark.String(data)}, nil
	case ".json":
		var v starlark.Value
		if err := Decode(data, &v); err != nil {
			return nil, err
		}
		return starlark.StringDict{"json": v}, nil
	default:
		return nil, fmt.Errorf("unsupported-ext: %s (%s)", ext, path)
	}
}
