package pboptions_test

import (
	"fmt"
	"strings"
	"testing"

	testpb "github.com/michelangelo-ai/michelangelo/proto-go/test/kubeproto"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/pboptions"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/compiler/protogen"
	golangproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestOptions(t *testing.T) {
	data := testpb.GetProtocReqData()

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
		if strings.HasSuffix(f.Proto.GetName(), "/testobject.proto") {
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

	messages := make(map[string]*protogen.Message)
	for _, m := range testFile.Messages {
		messages[string(m.Desc.Name())] = m
	}
	options, _ = pboptions.ReadOptions(extTypes, messages["TestMsg3"].Desc.Options().(*descriptorpb.MessageOptions))
	assert.True(t, options.Bool("has_resource"))
	options, _ = pboptions.ReadOptions(extTypes, messages["TestMsg2"].Desc.Options().(*descriptorpb.MessageOptions))
	assert.False(t, options.Bool("has_resource"))
	options, _ = pboptions.ReadOptions(extTypes, messages["TestObjectList"].Desc.Options().(*descriptorpb.MessageOptions))
	assert.False(t, options.Bool("has_resource"))
	assert.True(t, options.Bool("has_resource_list"))
	options, _ = pboptions.ReadOptions(extTypes, messages["TestObject"].Desc.Options().(*descriptorpb.MessageOptions))
	assert.True(t, options.Bool("has_resource"))
	for i, f := range messages["TestObjectSpec"].Fields {
		fmt.Println(i, f.Desc.Name())
	}
	fmt.Println()
	options, _ = pboptions.ReadOptions(extTypes, getFieldByName(messages["TestObjectSpec"], "description").Desc.Options().(*descriptorpb.FieldOptions))
	assert.Equal(t, int64(2), options.Int64("len(validation)"))
	assert.True(t, options.Bool("has_validation[0]"))
	assert.Equal(t, "\\S+( \\S+)*", options.String("validation[0].pattern"))
	assert.True(t, options.Bool("has_validation[1]"))
	assert.Equal(t, "1", options.String("validation[1].min_length"))
	assert.Equal(t, "255", options.String("validation[1].max_length"))

	assert.True(t, groupInfoFile != nil)
	options, _ = pboptions.ReadOptions(extTypes, groupInfoFile.Proto.Options)
	assert.Equal(t, "michelangelo.api", options.String("group_info.name"))
	assert.Equal(t, "v2", options.String("group_info.version"))
}

func getFieldByName(m *protogen.Message, name string) *protogen.Field {
	for _, f := range m.Fields {
		if string(f.Desc.Name()) == name {
			return f
		}
	}
	return nil
}
