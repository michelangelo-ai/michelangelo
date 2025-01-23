package zapfx

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProvideLevel(t *testing.T) {
	in := In{
		Config: Config{
			Level: "info",
		},
	}
	out, err := provide(in)
	require.NoError(t, err)

	require.Equal(t, "info", out.Level.String())
	require.Equal(t, "info", out.Logger.Level().String())
	require.Equal(t, false, out.Config.Development)

	// json must be the default encoding for the non-development mode
	require.Equal(t, "json", out.Config.Encoding)
}

func TestProvideDevelopment(t *testing.T) {
	in := In{
		Config: Config{
			Level:       "info",
			Development: true,
		},
	}
	out, err := provide(in)
	require.NoError(t, err)

	require.Equal(t, true, out.Config.Development)

	// console must be the default encoding for the development mode
	require.Equal(t, "console", out.Config.Encoding)
}

func TestProvideEncoding(t *testing.T) {
	in := In{
		Config: Config{
			Level:       "info",
			Development: true,
			Encoding:    "json",
		},
	}
	out, err := provide(in)
	require.NoError(t, err)

	require.Equal(t, true, out.Config.Development)
	// console is the default encoding for the development mode,
	// however, it must be overridden by the provided encoding
	require.Equal(t, "json", out.Config.Encoding)
}

func TestProvideInvalidLevel(t *testing.T) {
	in := In{
		Config: Config{
			Level: "foo", // invalid log level
		},
	}
	_, err := provide(in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "foo")
}
