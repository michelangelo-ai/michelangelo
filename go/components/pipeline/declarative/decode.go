package declarative

import (
	"fmt"
	"sort"
	"strings"

	gogoproto "github.com/gogo/protobuf/proto"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TypeURL is the Any type URL of a declarative manifest body.
const TypeURL = "type.googleapis.com/michelangelo.api.v2.DeclarativeWorkflow"

// SchemaVersion is the only accepted DeclarativeWorkflow.schema_version.
const SchemaVersion = "v1alpha1"

// maxContentBytes bounds the size of manifest.content.
const maxContentBytes = 4 << 20

// recursionLimit bounds message nesting in manifest.content. Each level of a
// nested config object uses three message levels (Struct, map entry, and
// Value), so a config object can nest 65 levels.
const recursionLimit = 200

// Decode unpacks manifest.content. It rejects a wrong type URL, content
// larger than 4 MiB or nested too deeply, an unknown field at any nesting
// level, and an unknown schema_version.
func Decode(m *v2pb.PipelineManifest) (w *v2pb.DeclarativeWorkflow, err error) {
	if m == nil || m.Content == nil {
		return nil, &Error{Msg: "content is not set"}
	}
	if m.Content.TypeUrl != TypeURL {
		return nil, &Error{Msg: fmt.Sprintf("content type URL %q is not %q", clip(m.Content.TypeUrl), TypeURL)}
	}
	if n := len(m.Content.Value); n > maxContentBytes {
		return nil, &Error{Msg: fmt.Sprintf("content is %d bytes; the limit is %d", n, maxContentBytes)}
	}
	// protobuf-go v1.36 dynamicpb panics on some malformed map entries.
	defer func() {
		if r := recover(); r != nil {
			w, err = nil, &Error{Msg: fmt.Sprintf("content does not decode: %v", r)}
		}
	}()
	if err := rejectUnknownFields(m.Content.Value); err != nil {
		return nil, err
	}
	w = &v2pb.DeclarativeWorkflow{}
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
	if err := (proto.UnmarshalOptions{RecursionLimit: recursionLimit}).Unmarshal(raw, dm); err != nil {
		return &Error{Msg: "content does not decode: " + err.Error()}
	}
	return walkUnknown(&fieldPath{seg: "DeclarativeWorkflow"}, dm)
}

// fieldPath names a message by its chain of parents. The text is built only
// when an error is reported, so each nesting level costs O(1).
type fieldPath struct {
	parent *fieldPath
	seg    string
}

// String joins the segments from the root down.
func (p *fieldPath) String() string {
	var segs []string
	for ; p != nil; p = p.parent {
		segs = append(segs, p.seg)
	}
	var b strings.Builder
	for i := len(segs) - 1; i >= 0; i-- {
		b.WriteString(segs[i])
	}
	return b.String()
}

// clip shortens caller-supplied text for an error message.
func clip(s string) string {
	const max = 128
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// walkUnknown returns an error for the first unknown field in m or in any
// message nested under it. path names m in the error. Map keys are visited in
// sorted order so the reported field is stable, and quoted because they are
// caller-supplied.
func walkUnknown(path *fieldPath, m protoreflect.Message) error {
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
		child := &fieldPath{parent: path, seg: "." + string(fd.Name())}
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
				if err := walkUnknown(&fieldPath{parent: child, seg: fmt.Sprintf("[%q]", k)}, v.Message()); err != nil {
					return err
				}
			}
		case fd.IsList() && fd.Kind() == protoreflect.MessageKind:
			l := m.Get(fd).List()
			for j := 0; j < l.Len(); j++ {
				if err := walkUnknown(&fieldPath{parent: child, seg: fmt.Sprintf("[%d]", j)}, l.Get(j).Message()); err != nil {
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
