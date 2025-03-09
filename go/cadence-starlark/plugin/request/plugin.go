package request

import (
	"net/http"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/worker"
)

var Plugin = &plugin{}

type plugin struct{}

var _ cadstar.IPlugin = (*plugin)(nil)

func (r *plugin) Create(_ cadstar.RunInfo) starlark.StringDict {
	return starlark.StringDict{"request": &Module{}}
}

func (r *plugin) Register(w worker.Registry) {
	w.RegisterActivity(&activities{
		client: http.DefaultClient,
	})
}
