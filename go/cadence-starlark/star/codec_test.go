package star

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"testing"
)

func TestEncode(t *testing.T) {

	t.Run("bytes", func(t *testing.T) {
		// bytes are encoded as quoted base64 string
		input := starlark.Bytes("abc")
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`"YWJj"`), res)
	})

	t.Run("bytes-0", func(t *testing.T) {
		// bytes edge case: zero value
		var input starlark.Bytes
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`""`), res)
	})

	t.Run("keywords", func(t *testing.T) {
		// keywords are encoded as list of lists
		input := []starlark.Tuple{
			{starlark.String("pi"), starlark.Float(3.14)},
			{starlark.String("test"), starlark.True},
		}
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`[["pi",3.14],["test",true]]`), res)
	})

	t.Run("keywords-0", func(t *testing.T) {
		// keywords edge case: zero value
		var input []starlark.Tuple
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`null`), res)
	})

	t.Run("dataclass", func(t *testing.T) {
		// bytes are encoded as quoted base64 string
		input := &starlark.Dict{}
		require.NoError(t, input.SetKey(starlark.String("__codec__"), starlark.String("dataclass")))
		require.NoError(t, input.SetKey(starlark.String("pi"), starlark.Float(3.14)))
		require.NoError(t, input.SetKey(starlark.String("test"), starlark.True))
		res, err := Encode(&Dataclass{Dict: input})
		require.NoError(t, err)
		require.Equal(t, []byte(`{"__codec__":"dataclass","pi":3.14,"test":true}`), res)
	})

	t.Run("string", func(t *testing.T) {
		// string is encoded as quoted string
		input := starlark.String("abc")
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`"abc"`), res)
	})

	t.Run("string-0", func(t *testing.T) {
		// string edge case: zero value
		var input starlark.String
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`""`), res)
	})

	t.Run("dict", func(t *testing.T) {
		// dict is encoded as object
		input := &starlark.Dict{}
		require.NoError(t, input.SetKey(starlark.String("pi"), starlark.Float(3.14)))
		require.NoError(t, input.SetKey(starlark.String("test"), starlark.True))
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`{"pi":3.14,"test":true}`), res)
	})

	t.Run("dict-0", func(t *testing.T) {
		// dict edge case: zero value
		var input *starlark.Dict
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`null`), res)
	})

	t.Run("tuple", func(t *testing.T) {
		// tuple is encoded as list
		input := &starlark.Tuple{starlark.String("pi"), starlark.Float(3.14), starlark.False}
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`["pi",3.14,false]`), res)
	})

	t.Run("tuple-0", func(t *testing.T) {
		// tuple edge case: zero value
		var input *starlark.Tuple
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`null`), res)
	})

	t.Run("none", func(t *testing.T) {
		// none is null
		input := starlark.None
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`null`), res)
	})

	t.Run("value", func(t *testing.T) {
		// value is encoded according to the underlying type
		var input starlark.Value = starlark.Float(3.14)
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`3.14`), res)
	})

	t.Run("value-0", func(t *testing.T) {
		// value edge case: zero value
		var input starlark.Value
		res, err := Encode(input)
		require.NoError(t, err)
		require.Equal(t, []byte(`null`), res)
	})

	t.Run("go-string", func(t *testing.T) {
		// go interfaces are not supported
		input := "abc"
		_, err := Encode(input)
		require.Error(t, err)
		_, ok := err.(UnsupportedTypeError)
		require.True(t, ok, fmt.Sprintf("bad error type: %T", err))
	})

	t.Run("go-slice", func(t *testing.T) {
		// go interfaces are not supported
		input := []any{"pi", 3.14}
		_, err := Encode(input)
		require.Error(t, err)
		_, ok := err.(UnsupportedTypeError)
		require.True(t, ok, fmt.Sprintf("bad error type: %T", err))
	})

}

func TestDecode(t *testing.T) {

	t.Run("bytes", func(t *testing.T) {
		// bytes are decoded from the base64 quoted string
		var out starlark.Bytes
		require.NoError(t, Decode([]byte(`"YWJj"`), &out))
		require.Equal(t, starlark.Bytes("abc"), out)
	})

	t.Run("bytes-0", func(t *testing.T) {
		// bytes edge case: zero value
		var out starlark.Bytes
		require.NoError(t, Decode([]byte(`""`), &out))
		var expected starlark.Bytes
		require.Equal(t, expected, out)
	})

	t.Run("keywords", func(t *testing.T) {
		// keywords are decoded from the list of lists
		var out []starlark.Tuple
		require.NoError(t, Decode([]byte(`[["pi",3.14],["test",true]]`), &out))
		expected := []starlark.Tuple{
			{starlark.String("pi"), starlark.Float(3.14)},
			{starlark.String("test"), starlark.True},
		}
		require.Equal(t, expected, out)
	})

	t.Run("keywords-0", func(t *testing.T) {
		// keywords edge case: zero value
		var out []starlark.Tuple
		require.NoError(t, Decode([]byte(`null`), &out))
		var expected []starlark.Tuple
		require.Equal(t, expected, out)
	})

	t.Run("keywords-blank", func(t *testing.T) {
		// keywords edge case: blank list
		var out []starlark.Tuple
		require.NoError(t, Decode([]byte(`[]`), &out))
		require.Equal(t, []starlark.Tuple{}, out)
	})

	t.Run("tuple", func(t *testing.T) {
		// tuple is decoded from the list
		var out starlark.Tuple
		require.NoError(t, Decode([]byte(`["pi",3.14]`), &out))
		expected := starlark.Tuple{
			starlark.String("pi"), starlark.Float(3.14),
		}
		require.Equal(t, expected, out)
	})

	t.Run("tuple-0", func(t *testing.T) {
		// tuple edge case: zero value
		var out starlark.Tuple
		require.NoError(t, Decode([]byte(`null`), &out))
		var expected starlark.Tuple
		require.Equal(t, expected, out)
	})

	t.Run("tuple-blank", func(t *testing.T) {
		// tuple edge case: blank list
		var out starlark.Tuple
		require.NoError(t, Decode([]byte(`[]`), &out))
		require.Equal(t, starlark.Tuple{}, out)
	})

	t.Run("dict", func(t *testing.T) {
		// dict is decoded from the object
		var out *starlark.Dict
		require.NoError(t, Decode([]byte(`{"foo":"bar"}`), &out))
		expected := &starlark.Dict{}
		require.NoError(t, expected.SetKey(starlark.String("foo"), starlark.String("bar")))
		require.Equal(t, expected, out)
	})

	t.Run("value-none", func(t *testing.T) {
		var out starlark.Value
		require.NoError(t, Decode([]byte(`null`), &out))
		require.Equal(t, starlark.None, out)
	})

	t.Run("value-string", func(t *testing.T) {
		// value can accept any starlark type
		var out starlark.Value
		require.NoError(t, Decode([]byte(`"abc"`), &out))
		require.Equal(t, starlark.String("abc"), out)
	})

	t.Run("value-dataclass", func(t *testing.T) {
		// value can accept any starlark type
		var out starlark.Value
		require.NoError(t, Decode([]byte(`{"__codec__":"dataclass","pi":3.14,"test":true}`), &out))
		expected := &starlark.Dict{}
		require.NoError(t, expected.SetKey(starlark.String("__codec__"), starlark.String("dataclass")))
		require.NoError(t, expected.SetKey(starlark.String("pi"), starlark.Float(3.14)))
		require.NoError(t, expected.SetKey(starlark.String("test"), starlark.True))
		require.Equal(t, &Dataclass{Dict: expected}, out)
	})

	t.Run("go-interface", func(t *testing.T) {
		// go interfaces are not supported
		var out any
		err := Decode([]byte(`"abc"`), &out)
		require.Error(t, err)
		_, ok := err.(UnsupportedTypeError)
		require.True(t, ok, fmt.Sprintf("bad error type: %T", err))
	})
}
