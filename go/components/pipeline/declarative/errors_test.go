package declarative

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorString(t *testing.T) {
	cases := []struct {
		name string
		err  Error
		want string
	}{
		{"task field release", Error{Task: "train", Field: "job_specs.ray", Msg: `field "regions" is not declared by task type "core/ray"`, Release: "core-v1.4.2"},
			`task "train": job_specs.ray field "regions" is not declared by task type "core/ray" in core-v1.4.2`},
		{"parameter", Error{Param: "epochs", Msg: "default 1.5 is not an integer"},
			`parameter "epochs": default 1.5 is not an integer`},
		{"workflow only", Error{Msg: `schema_version "v2" is not supported; use "v1alpha1"`},
			`schema_version "v2" is not supported; use "v1alpha1"`},
		{"task only", Error{Task: "b", Msg: `depends_on names unknown task "zz"`},
			`task "b": depends_on names unknown task "zz"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.err.Error())
		})
	}
}

func TestDefaultLimits(t *testing.T) {
	assert.Equal(t, 500, DefaultLimits().MaxTasks)
	assert.Equal(t, 500, (Limits{}).withDefaults().MaxTasks)
	assert.Equal(t, 7, (Limits{MaxTasks: 7}).withDefaults().MaxTasks)
	assert.Equal(t, 5000, DefaultLimits().MaxDependencies)
	assert.Equal(t, 5000, (Limits{}).withDefaults().MaxDependencies)
	assert.Equal(t, 9, (Limits{MaxDependencies: 9}).withDefaults().MaxDependencies)
}
