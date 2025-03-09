package hashlib

import (
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/star"
	"go.starlark.net/starlark"
	"go.temporal.io/sdk/workflow"
	"golang.org/x/crypto/blake2b"
)

type Module struct{}

var _ starlark.HasAttrs = &Module{}

func (f *Module) String() string                        { return "hashlib" }
func (f *Module) Type() string                          { return "hashlib" }
func (f *Module) Freeze()                               {}
func (f *Module) Truth() starlark.Bool                  { return true }
func (f *Module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (f *Module) Attr(n string) (starlark.Value, error) { return star.Attr(f, n, builtins, properties) }
func (f *Module) AttrNames() []string                   { return star.AttrNames(builtins, properties) }

var properties = map[string]star.PropertyFactory{}

var builtins = map[string]*starlark.Builtin{
	"blake2b_hex": starlark.NewBuiltin("blake2b_hex", blake2b_hex),
}

// blake2b_hex calculates the hash value given a string
// Arguments:
//   - data: the origin string to calculate the hash
//   - digest_size: the byte size of the return hash value. Note that the actual length of the hash hex will be digest_size * 2
//
// Return: The calculated hash value
func blake2b_hex(t *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := cadstar.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var data string
	var digestSize int
	if err := starlark.UnpackArgs("blake2b_hex", args, kwargs, "data", &data, "digest_size", &digestSize); err != nil {
		logger.Error("hashlib-blake2b_hex-error", "error", err)
		return nil, err
	}

	// Calculate hash value for the raw string
	disgestVal, err := hashWithBlake2b(data, digestSize)
	if err != nil {
		logger.Error("hashlib-blake2b_hex-error", "error", err)
		return nil, err
	}
	return starlark.String(disgestVal), nil
}

func hashWithBlake2b(input string, outputSize int) (string, error) {
	// Create a BLAKE2b hasher with the specified output size
	hash, err := blake2b.New(outputSize, nil) // `nil` key means no key (not using MAC mode)
	if err != nil {
		return "", err
	}

	// Write data to the hasher
	hash.Write([]byte(input))

	// Compute the hash
	sum := hash.Sum(nil)

	// Return the hash as a hexadecimal string
	// Each byte in sum will be represented as two chars.
	return fmt.Sprintf("%x", sum), nil
}
