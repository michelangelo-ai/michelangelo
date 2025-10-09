package groupinfo

import (
	"testing"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	testpb "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/pluginpb"
)

func readInput(t *testing.T) (*protogen.Plugin, *protoregistry.Types) {
	reqData := testpb.GetProtocReqData()

	req := &pluginpb.CodeGeneratorRequest{}
	err := proto.Unmarshal(reqData, req)
	assert.True(t, err == nil)
	util.ReplaceImportPath(req)

	gen, err := protogen.Options{}.New(req)
	assert.True(t, err == nil)

	extTypes := pboptions.LoadPBExtensions(gen.Files)

	return gen, extTypes
}

func TestGroupVersionInfo(t *testing.T) {
	gen, extTypes := readInput(t)
	gInfoMap := Load(gen, extTypes)
	assert.Equal(t, 1, len(gInfoMap))
	gInfo, ok := gInfoMap["michelangelo.test.kubeproto"]
	assert.True(t, ok)
	assert.Equal(t, "v2", gInfo.Version)
	assert.Equal(t, "michelangelo.api", gInfo.Name)
}
