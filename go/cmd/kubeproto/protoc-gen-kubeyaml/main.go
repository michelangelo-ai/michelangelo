package main

import (
	"fmt"
	"io"
	"os"

	"github.com/michelangelo-ai/michelangelo/go/kubeproto/yaml"
	"google.golang.org/protobuf/proto"
)

// A protoc plugin that generates CRD yaml files from protobuf definitions.
//
// This tool is for testing and debugging purposes only. The same yaml schemas are embedded in the go code generated
// by protoc-gen-kubeproto (go_kubeproto compiler). The CRD schemas are registered / updated automatically by go code
// in Michelangelo API server. Therefore, Michelangleo users never need to manually generate and apply CRD yaml files.
func main() {
	reqData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read protoc request from stdin: %v\n", err)
		os.Exit(1)
	}
	resp := yaml.GenerateYaml(reqData)
	out, err := proto.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal response: %v\n", err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
}
