package star

import (
	jsoniter "github.com/json-iterator/go"
	"go.starlark.net/starlark"
	"sort"
)

type PropertyFactory = func(receiver starlark.Value) (starlark.Value, error)

func Attr(
	receiver starlark.Value,
	name string,
	builtins map[string]*starlark.Builtin,
	properties map[string]PropertyFactory,
) (starlark.Value, error) {
	b := builtins[name]
	if b != nil {
		return b.BindReceiver(receiver), nil
	}
	p := properties[name]
	if p != nil {
		return p(receiver)
	}
	return nil, nil
}

func AttrNames(
	builtins map[string]*starlark.Builtin,
	properties map[string]PropertyFactory,
) []string {
	res := make([]string, 0, len(builtins)+len(properties))
	for name := range builtins {
		res = append(res, name)
	}
	for name := range properties {
		res = append(res, name)
	}
	sort.Strings(res)
	return res
}

func Iterate(v starlark.Iterable, handler func(i int, el starlark.Value)) {
	it := v.Iterate()
	defer it.Done()
	var el starlark.Value
	for i := 0; it.Next(&el); i++ {
		handler(i, el)
	}
}

func AsGo(source starlark.Value, out any) error {
	b, err := Encode(source)
	if err != nil {
		return err
	}
	return jsoniter.Unmarshal(b, out)
}

func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return Decode(b, out)
}
