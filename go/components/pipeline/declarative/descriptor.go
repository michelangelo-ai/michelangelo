package declarative

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync"

	gogoproto "github.com/gogo/protobuf/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

const declarativeProtoFile = "michelangelo/api/v2/declarative.proto"

var (
	descOnce sync.Once
	descMsg  protoreflect.MessageDescriptor
	descErr  error
)

// workflowDescriptor builds the DeclarativeWorkflow descriptor from the bytes
// the gogo stubs register. The gogo stubs drop unknown fields at parse, so
// strict decoding needs a descriptor the google.golang.org runtime can use.
func workflowDescriptor() (protoreflect.MessageDescriptor, error) {
	descOnce.Do(func() {
		gz := gogoproto.FileDescriptor(declarativeProtoFile)
		if gz == nil {
			descErr = fmt.Errorf("gogo registry has no descriptor for %s", declarativeProtoFile)
			return
		}
		r, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			descErr = err
			return
		}
		raw, err := io.ReadAll(r)
		if err != nil {
			descErr = err
			return
		}
		fdp := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(raw, fdp); err != nil {
			descErr = err
			return
		}
		fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
		if err != nil {
			descErr = err
			return
		}
		descMsg = fd.Messages().ByName("DeclarativeWorkflow")
		if descMsg == nil {
			descErr = fmt.Errorf("%s has no DeclarativeWorkflow message", declarativeProtoFile)
		}
	})
	return descMsg, descErr
}
