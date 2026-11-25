package declarative

import (
	gogoproto "github.com/gogo/protobuf/proto"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Encode returns one byte form for w, so a re-apply of an equal workflow
// stores equal bytes. Map entries are sorted by key at every level, which
// includes the Struct maps in config and job_specs. The form is stable only
// within one binary: a protobuf-go upgrade can change it.
//
// The gogo marshaler writes maps in random order and rejects
// SetDeterministic for this type. Encode therefore re-encodes through
// dynamicpb with the google.golang.org deterministic option.
func Encode(w *v2pb.DeclarativeWorkflow) ([]byte, error) {
	raw, err := gogoproto.Marshal(w)
	if err != nil {
		return nil, err
	}
	md, err := workflowDescriptor()
	if err != nil {
		return nil, err
	}
	dm := dynamicpb.NewMessage(md)
	if err := (proto.UnmarshalOptions{RecursionLimit: recursionLimit}).Unmarshal(raw, dm); err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(dm)
}
