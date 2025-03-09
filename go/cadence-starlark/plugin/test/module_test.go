package test

import (
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"strings"
	"testing"
)

func TestTrue(t *testing.T) {

	var err error

	d := &starlark.Dict{}

	thread := &starlark.Thread{}

	_, err = _true(thread, nil, starlark.Tuple{d}, nil)
	require.Error(t, err)

	require.True(t, strings.HasPrefix(err.Error(), "assert"))

	err = d.SetKey(starlark.String("foo"), starlark.String("bar"))
	require.NoError(t, err)

	_, err = _true(thread, nil, starlark.Tuple{d}, nil)
	require.NoError(t, err)
}
