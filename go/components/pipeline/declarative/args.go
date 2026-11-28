package declarative

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// WorkflowArgs returns the interpreter inputs for w: [spec, params].
//
// spec is w in the JSON form of the proto, with proto field names, enum
// names, and every unset field present. params maps each parameter name to
// its default value. A parameter with no default returns an *Error, because
// a run cannot supply a value yet.
func WorkflowArgs(w *v2pb.DeclarativeWorkflow) ([]any, error) {
	spec, err := toJSON(w)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(w.Parameters))
	for name := range w.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	params := map[string]any{}
	for _, name := range names {
		def := w.Parameters[name].GetDefault()
		if def == nil {
			return nil, &Error{Param: name, Msg: "has no value; set a default"}
		}
		if params[name], err = toJSON(def); err != nil {
			return nil, err
		}
	}
	return []any{spec, params}, nil
}

func toJSON(m gogoproto.Message) (any, error) {
	s, err := (&jsonpb.Marshaler{OrigName: true, EmitDefaults: true}).MarshalToString(m)
	if err != nil {
		return nil, fmt.Errorf("marshal %T: %w", m, err)
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("unmarshal %T: %w", m, err)
	}
	return v, nil
}
