package ext

import (
	"fmt"
	"strconv"
	"strings"
)

func JP[T any](obj any, path string) (T, error) {
	// [!] Limited json path support
	// TODO: [oss] full json path support

	var zeroT T

	if strings.Contains(path, "..") || strings.ContainsAny(path, "()<>=*?'\" ") {
		return zeroT, fmt.Errorf("jp: unsupported: advansed json path is not supported, path: %s", path)
	}
	if strings.HasPrefix(path, "$.") {
		path = path[1:]
	}
	var res = obj
	for _, f := range strings.Split(path, ".") {
		if f == "" {
			continue
		}
		r, ok := res.(map[string]any)
		if !ok {
			return zeroT, fmt.Errorf("jp: bad-type: expected: %T, got: %T, path: %s", map[string]any{}, res, path)
		}

		idx := -1
		if f[len(f)-1] == ']' {
			i := strings.Index(f, "[")
			if i == -1 {
				return zeroT, fmt.Errorf("jp: bad-path: bad square brackets, path: %s", path)
			}
			_idx := f[i+1 : len(f)-1]
			f = f[:i]
			var err error
			if idx, err = strconv.Atoi(_idx); err != nil {
				return zeroT, fmt.Errorf("jp: bad-path: can not parse array index: %s, path: %s", _idx, path)
			}
		}

		var found = false
		res, found = r[f]
		if !found {
			return zeroT, fmt.Errorf("jp: not-found: field: %s path: %s", f, path)
		}
		if idx > -1 {
			_res, ok := res.([]any)
			if !ok {
				return zeroT, fmt.Errorf("jp: bad-type: expected: []any, got: %T, path: %s", res, path)
			}
			if idx >= len(_res) {
				return zeroT, fmt.Errorf("jp: not-found: field: %s, index: %d path: %s", f, idx, path)
			}
			res = _res[idx]
		}
	}
	resT, ok := res.(T)
	if !ok {
		return zeroT, fmt.Errorf("jp: bad-type: expected: %T, got: %T, path: %s", zeroT, res, path)
	}
	return resT, nil
}
