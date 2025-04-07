package main

import (
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/yaml"
)

// A protoc plugin that generates CRD yaml files from protobuf definitions.
//
// This tool is for testing and debugging purposes only. The same yaml schemas are embedded in the go code generated
// by protoc-gen-kubeproto (go_kubeproto compiler). The CRD schemas are registered / updated automatically by go code
// in Michelangelo API server. Therefore, Michelangleo users never need to manually generate and apply CRD yaml files.
func main() {
	reqData := util.ReadRequest()
	resp := yaml.GenerateYaml(reqData)
	util.WriteResponse(resp)
}
