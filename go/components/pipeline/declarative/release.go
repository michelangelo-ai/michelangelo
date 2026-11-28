package declarative

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Release is one interpreter release file.
type Release struct {
	Version   string               `json:"version"`
	Artifact  string               `json:"artifact"`
	Digest    string               `json:"digest"`
	TaskTypes map[string]*TaskType `json:"task_types"`
}

// TaskType is one entry of a release's task_types map.
type TaskType struct {
	Capabilities []string `json:"capabilities"`
	Config       Schema   `json:"config"`
	JobSpecs     Schema   `json:"job_specs"`
}

// HasCapability reports whether the type lists name under capabilities.
func (t *TaskType) HasCapability(name string) bool {
	for _, c := range t.Capabilities {
		if c == name {
			return true
		}
	}
	return false
}

// Schema maps a field name to its declaration.
type Schema map[string]*Field

// Field is a leaf when Type is set, or a group when Fields is set. In a
// release file, an object with a "type" key is a leaf, with an optional
// "required" boolean. Any other object is a group. Inside a group, the keys
// "type" and "required" are reserved, so a group cannot have a child with
// either name.
type Field struct {
	Type     string
	Required bool
	Fields   Schema
}

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var fieldTypes = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "list": true, "dict": true}

// maxSchemaDepth bounds how deeply a task type schema may nest groups.
const maxSchemaDepth = 32

// UnmarshalJSON decodes a schema and names the field in a decoding error. A
// null field stays nil, and checkSchema reports it as empty.
func (s *Schema) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("schema must be an object, not %s", jsonKind(v))
	}
	fs, err := schemaFromAny(obj, "", 1)
	if err != nil {
		return err
	}
	*s = fs
	return nil
}

// UnmarshalJSON decodes one field declaration.
func (f *Field) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	return f.fromAny(v, "", 1)
}

// MarshalJSON writes f in the shape that UnmarshalJSON reads.
func (f Field) MarshalJSON() ([]byte, error) {
	if f.Type == "" {
		return json.Marshal(f.Fields)
	}
	return json.Marshal(struct {
		Type     string `json:"type"`
		Required bool   `json:"required,omitempty"`
	}{f.Type, f.Required})
}

// schemaFromAny decodes each field of a group. prefix names the group in an
// error.
func schemaFromAny(obj map[string]any, prefix string, depth int) (Schema, error) {
	out := make(Schema, len(obj))
	for k, cv := range obj {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if cv == nil {
			out[k] = nil
			continue
		}
		f := &Field{}
		if err := f.fromAny(cv, path, depth); err != nil {
			return nil, err
		}
		out[k] = f
	}
	return out, nil
}

// fromAny decodes a field from a parsed JSON value. path names the field in
// an error.
func (f *Field) fromAny(v any, path string, depth int) error {
	fail := func(format string, args ...any) error {
		msg := fmt.Sprintf(format, args...)
		if path == "" {
			return errors.New(msg)
		}
		return fmt.Errorf("field %q: %s", path, msg)
	}
	if depth > maxSchemaDepth {
		return fail("schema nests more than %d levels", maxSchemaDepth)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return fail("must be an object, not %s", jsonKind(v))
	}
	if t, ok := obj["type"]; ok {
		if _, group := t.(map[string]any); group {
			return fail(`"type" is reserved for the field type; a group cannot have a child named "type"`)
		}
		ts, ok := t.(string)
		if !ok || ts == "" {
			return fail("type must be a non-empty string")
		}
		f.Type = ts
		if r, ok := obj["required"]; ok {
			rb, ok := r.(bool)
			if !ok {
				return fail("required must be a boolean")
			}
			f.Required = rb
		}
		rest := make(map[string]any, len(obj))
		for k, cv := range obj {
			if k != "type" && k != "required" {
				rest[k] = cv
			}
		}
		if len(rest) == 0 {
			return nil
		}
		obj = rest
	} else if _, ok := obj["required"]; ok {
		return fail(`"required" is reserved; set it next to "type" on a leaf field`)
	}
	fs, err := schemaFromAny(obj, path, depth+1)
	if err != nil {
		return err
	}
	f.Fields = fs
	return nil
}

// jsonKind names the JSON kind of a parsed value for an error message.
func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	case []any:
		return "a list"
	case nil:
		return "null"
	}
	return "an object"
}

// maxReleaseFileBytes bounds the size of one release file.
const maxReleaseFileBytes = 1 << 20

// readReleaseFile reads a regular file of at most maxReleaseFileBytes. It
// follows symlinks, which ConfigMap volume mounts use.
func readReleaseFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.Unwrap(err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Unwrap(err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReleaseFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseFileBytes {
		return nil, fmt.Errorf("is larger than %d bytes", maxReleaseFileBytes)
	}
	return data, nil
}

// LoadReleases reads every *.yaml and *.yml file in dir. It fails on a file
// that is not a regular file or is larger than 1 MiB, on a file name that
// does not equal the version it declares, and on a version that more than one
// file declares. Errors name the file, not its full path.
func LoadReleases(dir string) (map[string]*Release, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]*Release{}
	files := map[string]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ext)
		data, err := readReleaseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("release file %q: %w", e.Name(), err)
		}
		r, err := ParseRelease(stem, data)
		if err != nil {
			return nil, fmt.Errorf("release file %q: %w", e.Name(), err)
		}
		if r.Version != stem {
			return nil, fmt.Errorf("release file %q declares version %q; the file name must equal the version", e.Name(), r.Version)
		}
		if prev, dup := files[r.Version]; dup {
			return nil, fmt.Errorf("release %q is declared by both %q and %q; keep one", r.Version, prev, e.Name())
		}
		files[r.Version] = e.Name()
		out[r.Version] = r
	}
	return out, nil
}

// ParseRelease decodes one release file. It uses name in error text when the
// file has no version.
func ParseRelease(name string, data []byte) (*Release, error) {
	r := &Release{}
	if err := yaml.UnmarshalStrict(data, r); err != nil {
		return nil, fmt.Errorf("release %q: %w", name, err)
	}
	if r.Version != "" {
		name = r.Version
	}
	switch {
	case r.Version == "":
		return nil, fmt.Errorf("release %q: version is not set", name)
	case r.Artifact == "":
		return nil, fmt.Errorf("release %q: artifact is not set", name)
	case !digestPattern.MatchString(r.Digest):
		return nil, fmt.Errorf("release %q: digest must be \"sha256:\" and 64 lowercase hex characters", name)
	case len(r.TaskTypes) == 0:
		return nil, fmt.Errorf("release %q: task_types is empty", name)
	}
	ttNames := make([]string, 0, len(r.TaskTypes))
	for tn := range r.TaskTypes {
		ttNames = append(ttNames, tn)
	}
	sort.Strings(ttNames)
	for _, tn := range ttNames {
		tt := r.TaskTypes[tn]
		if tt == nil {
			return nil, fmt.Errorf("release %q: task type %q is empty", name, tn)
		}
		for _, section := range []struct {
			n string
			s Schema
		}{{"config", tt.Config}, {"job_specs", tt.JobSpecs}} {
			if err := checkSchema(section.s, ""); err != nil {
				return nil, fmt.Errorf("release %q: task type %q %s field %w", name, tn, section.n, err)
			}
		}
	}
	return r, nil
}

// checkSchema checks that each field in s is a leaf with a known type or a
// group of fields, never both. prefix names s in the error.
func checkSchema(s Schema, prefix string) error {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := s[k]
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if f == nil {
			return fmt.Errorf("%q is empty", path)
		}
		if f.Type != "" && f.Fields != nil {
			return fmt.Errorf("%q mixes type with nested fields", path)
		}
		if f.Type != "" {
			if !fieldTypes[f.Type] {
				return fmt.Errorf("%q has unknown type %q; use string, integer, number, boolean, list, or dict", path, f.Type)
			}
			continue
		}
		if err := checkSchema(f.Fields, path); err != nil {
			return err
		}
	}
	return nil
}
