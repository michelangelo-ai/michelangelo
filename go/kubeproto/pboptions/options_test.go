package pboptions_test

import (
	"strings"
	"testing"

	protocreq "github.com/michelangelo-ai/michelangelo/proto/test/kubeproto/protocreq"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/compiler/protogen"
	golangproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestOptions(t *testing.T) {
	data := protocreq.GetData()

	var req pluginpb.CodeGeneratorRequest
	golangproto.Unmarshal(data, &req)
	util.ReplaceImportPath(&req)

	// initialize golang proto generator
	gen, err := protogen.Options{}.New(&req)
	if err != nil {
		panic(err)
	}

	extTypes := pboptions.LoadPBExtensions(gen.Files)

	var testFile *protogen.File
	var groupInfoFile *protogen.File
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}
		if strings.HasSuffix(f.Proto.GetName(), "/options_ut.proto") {
			testFile = f
		}
		if strings.HasSuffix(f.Proto.GetName(), "/groupversion_info_ut.proto") {
			groupInfoFile = f
		}
	}
	assert.True(t, testFile != nil)

	options, _ := pboptions.ReadOptions(extTypes, testFile.Proto.Options)
	assert.Equal(t, "", options.String("group_info.name"))
	assert.Equal(t, "", options.String("gropu_info.version"))
	assert.Equal(t, 6, len(testFile.Messages))
	options, _ = pboptions.ReadOptions(extTypes, testFile.Messages[0].Desc.Options().(*descriptorpb.MessageOptions))
	assert.True(t, options.Bool("has_resource"))
	options, _ = pboptions.ReadOptions(extTypes, testFile.Messages[1].Desc.Options().(*descriptorpb.MessageOptions))
	assert.False(t, options.Bool("has_resource"))
	options, _ = pboptions.ReadOptions(extTypes, testFile.Messages[2].Desc.Options().(*descriptorpb.MessageOptions))
	assert.False(t, options.Bool("has_resource"))
	assert.True(t, options.Bool("has_resource_list"))
	options, _ = pboptions.ReadOptions(extTypes, testFile.Messages[3].Desc.Options().(*descriptorpb.MessageOptions))
	assert.True(t, options.Bool("has_resource"))
	options, _ = pboptions.ReadOptions(extTypes, testFile.Messages[4].Fields[0].Desc.Options().(*descriptorpb.FieldOptions))
	assert.True(t, options.Bool("has_validation[0]"))
	assert.Equal(t, int64(2), options.Int64("len(validation)"))
	assert.Equal(t, "ml-code-[a-z0-9-]+", options.String("validation[0].pattern"))
	assert.Equal(t, "must start with 'ml-code-' and only contains lower case alphanumeric or dash", options.String("validation[0].msg"))
	assert.True(t, options.Bool("has_validation[1]"))
	assert.Equal(t, "32", options.String("validation[1].max_length"))

	assert.True(t, groupInfoFile != nil)
	options, _ = pboptions.ReadOptions(extTypes, groupInfoFile.Proto.Options)
	assert.Equal(t, "michelangelo.api", options.String("group_info.name"))
	assert.Equal(t, "v2", options.String("group_info.version"))
}
