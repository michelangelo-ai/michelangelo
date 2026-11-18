package declarative

import (
	"fmt"
	"sort"

	gogoproto "github.com/gogo/protobuf/proto"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TypeURL is the Any type URL of a declarative manifest body.
const TypeURL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"

// SchemaVersion is the only accepted DeclarativeWorkflow.schema_version.
const SchemaVersion = "v1alpha1"

// Decode unpacks manifest.content. It rejects a wrong type URL, an unknown
// field at any nesting level, and an unknown schema_version.
func Decode(m *v2.PipelineManifest) (*v2.DeclarativeWorkflow, error) {
	if m == nil || m.Content == nil {
		return nil, &Error{Msg: "content is not set"}
	}
	if m.Content.TypeUrl != TypeURL {
		return nil, &Error{Msg: fmt.Sprintf("content type URL %q is not %q", m.Content.TypeUrl, TypeURL)}
	}
	if err := rejectUnknownFields(m.Content.Value); err != nil {
		return nil, err
	}
	w := &v2.DeclarativeWorkflow{}
	if err := gogoproto.Unmarshal(m.Content.Value, w); err != nil {
		return nil, &Error{Msg: "content does not decode: " + err.Error()}
	}
	if w.SchemaVersion != SchemaVersion {
		return nil, &Error{Msg: fmt.Sprintf("schema_version %q is not supported; use %q", w.SchemaVersion, SchemaVersion)}
	}
	return w, nil
}

// rejectUnknownFields parses raw with dynamicpb, which keeps the unknown
// fields that the gogo stubs drop, and returns an error for the first one.
func rejectUnknownFields(raw []byte) error {
	md, err := workflowDescriptor()
	if err != nil {
		return err
	}
	dm := dynamicpb.NewMessage(md)
	if err := (proto.UnmarshalOptions{}).Unmarshal(raw, dm); err != nil {
		return &Error{Msg: "content does not decode: " + err.Error()}
	}
	return walkUnknown("DeclarativeWorkflow", dm)
}

// walkUnknown returns an error for the first unknown field in m or in any
// message nested under it. path names m in the error. Map keys are visited in
// sorted order so the reported field is stable.
func walkUnknown(path string, m protoreflect.Message) error {
	if u := m.GetUnknown(); len(u) > 0 {
		num, _, _ := protowire.ConsumeTag(u)
		return &Error{Msg: fmt.Sprintf("%s has unknown field number %d", path, num)}
	}
	fds := m.Descriptor().Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !m.Has(fd) {
			continue
		}
		child := path + "." + string(fd.Name())
		switch {
		case fd.IsMap() && fd.MapValue().Kind() == protoreflect.MessageKind:
			mp := m.Get(fd).Map()
			keys := make([]string, 0, mp.Len())
			mp.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
				keys = append(keys, k.String())
				return true
			})
			sort.Strings(keys)
			for _, k := range keys {
				v := mp.Get(protoreflect.ValueOfString(k).MapKey())
				if err := walkUnknown(fmt.Sprintf("%s[%s]", child, k), v.Message()); err != nil {
					return err
				}
			}
		case fd.IsList() && fd.Kind() == protoreflect.MessageKind:
			l := m.Get(fd).List()
			for j := 0; j < l.Len(); j++ {
				if err := walkUnknown(fmt.Sprintf("%s[%d]", child, j), l.Get(j).Message()); err != nil {
					return err
				}
			}
		case !fd.IsMap() && !fd.IsList() && fd.Kind() == protoreflect.MessageKind:
			if err := walkUnknown(child, m.Get(fd).Message()); err != nil {
				return err
			}
		}
	}
	return nil
}
