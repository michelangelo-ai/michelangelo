package main

import (
	"io"
	"os"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/yaml"
	"google.golang.org/protobuf/proto"
)

func main() {
	reqData, _ := io.ReadAll(os.Stdin)
	resp := yaml.GenerateYaml(reqData)
	out, _ := proto.Marshal(resp)
	os.Stdout.Write(out)
}
