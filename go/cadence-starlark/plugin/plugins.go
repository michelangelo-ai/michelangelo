package plugin

import (
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/atexit"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/cad"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/concurrent"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/hashlib"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/json"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/os"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/progress"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/request"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/test"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/time"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/plugin/uuid"
)

var Registry = []cadstar.IPlugin{
	cad.Plugin,
	request.Plugin,
	time.Plugin,
	test.Plugin,
	os.Plugin,
	json.Plugin,
	uuid.Plugin,
	concurrent.Plugin,
	atexit.Plugin,
	progress.Plugin,
	hashlib.Plugin,
}
