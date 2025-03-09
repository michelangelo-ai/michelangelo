package request

import (
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"io"
	"net/http"
)

type Response struct {
	Response *http.Response
}

var _ starlark.HasAttrs = (*Response)(nil)

func (r *Response) String() string                        { return "response" }
func (r *Response) Type() string                          { return "response" }
func (r *Response) Freeze()                               {}
func (r *Response) Truth() starlark.Bool                  { return true }
func (r *Response) Hash() (uint32, error)                 { return 0, fmt.Errorf("not hash-able") }
func (r *Response) Attr(n string) (starlark.Value, error) { return star.Attr(r, n, resB, resP) }
func (r *Response) AttrNames() []string                   { return star.AttrNames(resB, resP) }

var resP = map[string]star.PropertyFactory{
	"status_code": _statusCode,
}

var resB = map[string]*starlark.Builtin{
	"json": starlark.NewBuiltin("json", _json),
}

func _json(_ *starlark.Thread, fn *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	r := fn.Receiver().(*Response)
	body, err := io.ReadAll(r.Response.Body)
	if err != nil {
		return nil, err
	}
	var res starlark.Value
	err = star.Decode(body, &res)
	return res, err
}

func _statusCode(r starlark.Value) (starlark.Value, error) {
	code := r.(*Response).Response.StatusCode
	return starlark.MakeInt(code), nil
}
