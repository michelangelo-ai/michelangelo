package star

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
	"reflect"
)

var (
	_encode = starlarkjson.Module.Members["encode"].(*starlark.Builtin)
	_decode = starlarkjson.Module.Members["decode"].(*starlark.Builtin)
	b64     = base64.StdEncoding
)

type UnsupportedTypeError error

func Encode(v any) ([]byte, error) {
	switch v := v.(type) {
	case starlark.Bytes:
		// bytes: base 64
		src := []byte(v)
		res := make([]byte, b64.EncodedLen(len(src))+2)
		b64.Encode(res[1:len(res)-1], src)
		res[0] = '"'
		res[len(res)-1] = '"'
		return res, nil
	case []starlark.Tuple:
		// keywords: list
		if v == nil {
			return []byte("null"), nil
		}
		var res bytes.Buffer
		res.WriteRune('[')
		for i, tuple := range v {
			r, err := Encode(tuple)
			if err != nil {
				return nil, err
			}
			res.Write(r)
			if i < len(v)-1 {
				res.WriteRune(',')
			}
		}
		res.WriteRune(']')
		return res.Bytes(), nil
	default:
		if isNilValue(v) {
			return []byte("null"), nil
		}
		switch v := v.(type) {
		case starlark.Value:
			r, err := _encode.CallInternal(nil, starlark.Tuple{v}, nil)
			if err != nil {
				return nil, err
			}
			return []byte(r.(starlark.String)), err
		default:
			return nil, UnsupportedTypeError(
				fmt.Errorf("unsupported value type: expected: starlark type, actual: (%T) %v", v, v),
			)
		}
	}
}

func Decode(line []byte, out any) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			switch rec := rec.(type) {
			case error:
				err = fmt.Errorf("decode-panic: %w", rec)
			default:
				err = fmt.Errorf("decode-panic: %v", rec)
			}
		}
	}()
	_out := reflect.ValueOf(out)
	if _out.Kind() != reflect.Pointer {
		return fmt.Errorf("illegal output kind: expected: pointer, actual: %s", _out.Kind().String())
	}

	switch out := out.(type) {
	case *[]byte:
		var decodedString string

		// Decode JSON string
		err = json.Unmarshal(line, &decodedString)
		if err != nil {
			fmt.Println("Error decoding JSON:", err)
			return err
		}
		rawBytes := []byte(decodedString)
		res := make([]byte, b64.DecodedLen(len(rawBytes)))
		if n, decodeErr := b64.Decode(res, rawBytes); decodeErr != nil {
			return decodeErr
		} else {
			res = res[:n]
		}
		*out = res
	case *starlark.Bytes:
		// bytes: base64
		if len(line) < 2 || line[0] != '"' || line[len(line)-1] != '"' {
			return fmt.Errorf("incompatible data: output type: %T", out)
		}
		line = line[1 : len(line)-1]
		res := make([]byte, b64.DecodedLen(len(line)))
		if n, decodeErr := b64.Decode(res, line); decodeErr != nil {
			return decodeErr
		} else {
			res = res[:n]
		}
		*out = starlark.Bytes(res)
	case *[]starlark.Tuple:
		// keywords:
		var dst starlark.Value
		if err = Decode(line, &dst); err != nil {
			return err
		}
		switch dst := dst.(type) {
		case starlark.NoneType:
			*out = nil
		case *starlark.List:
			*out = keywordsFromList(dst)
		default:
			return fmt.Errorf("incompatible data: output type: %T", out)
		}
	default:
		_out = _out.Elem()
		starlarkValueType := reflect.TypeOf((*starlark.Value)(nil)).Elem()
		if !_out.Type().AssignableTo(starlarkValueType) {
			fmt.Printf("unsupported output type: expected: %v assignable, actual: %v", starlarkValueType, _out.Type())
			return UnsupportedTypeError(
				fmt.Errorf("unsupported output type: expected: %v assignable, actual: %v", starlarkValueType, _out.Type()),
			)
		}
		var value starlark.Value
		if value, err = _decode.CallInternal(nil, starlark.Tuple{starlark.String(line)}, nil); err != nil {
			return err
		}
		if value, err = replaceDict(value, codecDictReplacer); err != nil {
			return err
		}

		switch out := out.(type) {
		case *starlark.Tuple:
			switch value := value.(type) {
			case *starlark.List:
				*out = tupleFromList(value)
			case starlark.NoneType:
				*out = nil
			default:
				return fmt.Errorf("incompatible data: output type: %T", out)
			}
		default:
			_out.Set(reflect.ValueOf(value))
		}
	}
	return nil
}

// codecDictReplacer replaces special "codec dictionaries" with custom starlark objects, such us Dataclass.
// See also: replaceDict
func codecDictReplacer(dict *starlark.Dict) (starlark.Value, error) {
	codec, found, err := dict.Get(starlark.String("__codec__"))
	if err != nil {
		return nil, err
	}
	if !found {
		return dict, nil
	}
	if codec == starlark.String("dataclass") {
		return &Dataclass{Dict: dict}, nil
	}
	return dict, nil
}

// replaceDict traverses JSON decoded data structure (starlark.Value) and replaces each JSON object (starlark.Dict) with
// a value returned by the replacer function. This is used to enable custom JSON decoding, for example to support
// Dataclass. The idea is similar to Python's JSON decoder object_hook https://docs.python.org/3/library/json.html#encoders-and-decoders
// See also: codecDictReplacer
func replaceDict(value starlark.Value, replacer func(d *starlark.Dict) (starlark.Value, error)) (starlark.Value, error) {
	switch value := value.(type) {
	case *starlark.Dict:
		for _, item := range value.Items() {
			v0 := item[1]
			v1, err := replaceDict(v0, replacer)
			if err != nil {
				return nil, err
			}
			if v0 != v1 {
				if err := value.SetKey(item[0], v1); err != nil {
					return nil, err
				}
			}
		}
		return replacer(value)

	case *starlark.List:
		it := value.Iterate()
		var v0 starlark.Value
		replacements := map[int]starlark.Value{}
		for i := 0; it.Next(&v0); i++ {
			v1, err := replaceDict(v0, replacer)
			if err != nil {
				it.Done()
				return nil, err
			}
			if v0 != v1 {
				replacements[i] = v1
			}
		}
		it.Done()
		for i, v := range replacements {
			if err := value.SetIndex(i, v); err != nil {
				return nil, err
			}
		}
		return value, nil

	case starlark.String, starlark.Bool, starlark.Int, starlark.Float, starlark.NoneType:
		// For the rest of JSON types, just return value as-is
		return value, nil

	default:
		// This function assumes input value contains only standard JSON types
		// All expected types are handled in the type-switch cases above.
		return nil, fmt.Errorf("unexpected-type: %T: %v", value, value)
	}
}

func keywordsFromList(l *starlark.List) []starlark.Tuple {
	t := make([]starlark.Tuple, l.Len())
	Iterate(l, func(i int, el starlark.Value) {
		k := el.(*starlark.List)
		t[i] = tupleFromList(k)
	})
	return t
}

func tupleFromList(l *starlark.List) starlark.Tuple {
	t := make(starlark.Tuple, l.Len())
	Iterate(l, func(i int, el starlark.Value) {
		t[i] = el
	})
	return t
}

func isNilValue(input any) bool {
	if input == nil {
		return true
	}
	v := reflect.ValueOf(input)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
