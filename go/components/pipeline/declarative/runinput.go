package declarative

import (
	"fmt"
	"sort"

	"github.com/gogo/protobuf/types"
)

// LastScheduleTimestampEnv is the only run input environ key that a
// declarative run accepts. The cron trigger sets it. Other keys can change
// the validated job_specs or the task image.
const LastScheduleTimestampEnv = "LAST_SCHEDULE_TIMESTAMP"

// CheckRunInput returns an *Error when the run input of a declarative run sets
// a key that the interpreter does not apply, so that the run does not silently
// ignore what its creator asked for.
func CheckRunInput(input *types.Struct) error {
	fields := input.GetFields()
	for _, key := range []string{"task_configs", "workflow_config"} {
		if _, ok := fields[key]; ok {
			return &Error{Msg: fmt.Sprintf("run input %s is not supported for declarative pipelines", key)}
		}
	}
	for _, key := range []string{"args", "kw_args"} {
		if !isEmpty(fields[key]) {
			return &Error{Msg: fmt.Sprintf("run input %s must be empty for declarative pipelines", key)}
		}
	}
	if !isEmpty(fields["parameter_map"]) {
		return &Error{Msg: "parameter overrides are not supported yet"}
	}
	environ := fields["environ"]
	if isEmpty(environ) {
		return nil
	}
	env := environ.GetStructValue()
	if env == nil {
		return &Error{Msg: "run input environ must be a struct"}
	}
	keys := make([]string, 0, len(env.Fields))
	for k := range env.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k != LastScheduleTimestampEnv {
			return &Error{Msg: fmt.Sprintf("run input environ key %q is not supported for declarative pipelines; only %s is allowed", k, LastScheduleTimestampEnv)}
		}
	}
	if _, ok := env.Fields[LastScheduleTimestampEnv].GetKind().(*types.Value_StringValue); !ok {
		return &Error{Msg: fmt.Sprintf("run input environ %s must be a string", LastScheduleTimestampEnv)}
	}
	return nil
}

// isEmpty reports whether v is absent, null, an empty list, or an empty struct.
func isEmpty(v *types.Value) bool {
	switch k := v.GetKind().(type) {
	case nil, *types.Value_NullValue:
		return true
	case *types.Value_ListValue:
		return len(k.ListValue.GetValues()) == 0
	case *types.Value_StructValue:
		return len(k.StructValue.GetFields()) == 0
	}
	return false
}
