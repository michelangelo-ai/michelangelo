package declarative_test

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func ExamplePlan() {
	rel, err := declarative.ParseRelease("core-v1", []byte(`version: core-v1
artifact: s3://bucket/core-v1.tar.gz
digest: sha256:00
task_types:
  core/python:
    config:
      entry_point: {type: string, required: true}
      input: {type: string}
`))
	if err != nil {
		panic(err)
	}
	str := func(s string) *types.Value { return &types.Value{Kind: &types.Value_StringValue{StringValue: s}} }
	w := &v2pb.DeclarativeWorkflow{
		SchemaVersion: declarative.SchemaVersion,
		Tasks: map[string]*v2pb.DeclarativeTask{
			"extract": {TaskFunction: "core/python", Config: &types.Struct{Fields: map[string]*types.Value{
				"entry_point": str("extract.py"),
			}}},
			"train": {TaskFunction: "core/python", Config: &types.Struct{Fields: map[string]*types.Value{
				"entry_point": str("train.py"),
				"input":       str("{{ tasks.extract.output }}"),
			}}, DependsOn: []*v2pb.TaskDependency{{Task: "extract"}}},
		},
	}
	p, err := declarative.Plan(w, declarative.Limits{}, rel, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("interpreter:", p.InterpreterVersion)
	for i, l := range p.Levels {
		fmt.Println("level", i, l.Tasks)
	}
	// Output:
	// interpreter: core-v1
	// level 0 [extract]
	// level 1 [train]
}
