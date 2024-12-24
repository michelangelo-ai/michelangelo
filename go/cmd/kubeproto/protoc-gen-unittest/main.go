package main

import (
	b64 "encoding/base64"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"

	"google.golang.org/protobuf/compiler/protogen"
	golangproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

var fileTemplate = template.Must(template.New("file").Parse(`
package {{.GoPackageName}}
`))

// record protoc request in a go file for unit test
func main() {
	reqData, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	var req pluginpb.CodeGeneratorRequest
	golangproto.Unmarshal(reqData, &req)
	util.ReplaceImportPath(&req)

	gen, err := protogen.Options{}.New(&req)
	if err != nil {
		panic(err)
	}

	recorded := false
	for _, f := range gen.Files {
		if !f.Generate {
			continue
		}

		filename := f.GeneratedFilenamePrefix + ".pb.record.go"
		g := gen.NewGeneratedFile(filename, f.GoImportPath)

		pkg := "package " + f.GoPackageName
		g.P(pkg)

		if recorded {
			continue
		}

		g.P()
		g.P("import b64 \"encoding/base64\"")
		data := b64.StdEncoding.EncodeToString(reqData)
		g.P("var data = `", data, "`")
		g.P("func GetProtocReqData() []byte {")
		g.P("  d, _ := b64.StdEncoding.DecodeString(data)")
		g.P("  return d")
		g.P("}")

		for _, msg := range f.Messages {
			var output string
			output += msg.GoIdent.GoName

			for _, comment := range msg.Comments.LeadingDetached {
				output += strings.TrimSpace(string(comment))
				output += " "
			}

			output += strings.TrimSpace(string(msg.Comments.Leading))
			strings.ReplaceAll(output, "`", "~")
			g.P("var _ = `")
			g.P(output)
			g.P("`")
		}

		recorded = true
	}

	resp := gen.Response()
	out, err := golangproto.Marshal(resp)
	if err != nil {
		panic(err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		panic(err)
	}
}
