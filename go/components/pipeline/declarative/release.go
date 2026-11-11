package declarative

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Release is one interpreter release file. The apiserver pins a pipeline to
// one release and validates it against that release's task-type schemas.
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

// Field is a leaf when Type is set, or a group when Fields is set.
type Field struct {
	Type     string
	Required bool
	Fields   Schema
}

var fieldTypes = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "list": true, "dict": true}

// UnmarshalJSON treats an object with a "type" key as a leaf and any other
// object as a group of nested fields. A leaf with extra keys keeps them in
// Fields, so checkSchema can report the mix with the field path.
func (f *Field) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["type"]; ok {
		var leaf struct {
			Type     string `json:"type"`
			Required bool   `json:"required"`
		}
		if err := json.Unmarshal(b, &leaf); err != nil {
			return err
		}
		f.Type, f.Required = leaf.Type, leaf.Required
		delete(raw, "type")
		delete(raw, "required")
		if len(raw) == 0 {
			return nil
		}
	}
	f.Fields = Schema{}
	for k, v := range raw {
		child := &Field{}
		if err := json.Unmarshal(v, child); err != nil {
			return err
		}
		f.Fields[k] = child
	}
	return nil
}

// LoadReleases reads every *.yaml and *.yml file in dir. It fails on a file
// name that does not equal the version it declares, and on a version that
// more than one file declares.
func LoadReleases(dir string) (map[string]*Release, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]*Release{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ext)
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		r, err := ParseRelease(stem, data)
		if err != nil {
			return nil, err
		}
		if r.Version != stem {
			return nil, fmt.Errorf("release file %q declares version %q; the file name must equal the version", e.Name(), r.Version)
		}
		if _, dup := out[r.Version]; dup {
			return nil, fmt.Errorf("release %q is declared by more than one file", r.Version)
		}
		out[r.Version] = r
	}
	return out, nil
}

// ParseRelease decodes one release file. name is used in error text when the
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
	case !strings.HasPrefix(r.Digest, "sha256:"):
		return nil, fmt.Errorf("release %q: digest must start with \"sha256:\"", name)
	case len(r.TaskTypes) == 0:
		return nil, fmt.Errorf("release %q: task_types is empty", name)
	}
	typeNames := make([]string, 0, len(r.TaskTypes))
	for tn := range r.TaskTypes {
		typeNames = append(typeNames, tn)
	}
	sort.Strings(typeNames)
	for _, tn := range typeNames {
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
