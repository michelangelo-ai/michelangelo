package main

import (
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/conversion"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/util"
)

func main() {
	reqData := util.ReadRequest()
	resp := conversion.Generate(reqData)
	util.WriteResponse(resp)
}
