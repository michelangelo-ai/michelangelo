package star

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/ext"
	"os"
	"path"
	"path/filepath"
)

var ErrNotExist = errors.New("file does not exist")

type tarFS struct {
	files map[string][]byte
}

var _ FS = (*tarFS)(nil)

func (r *tarFS) Read(p string) ([]byte, error) {
	p = path.Clean("/" + p) // TODO: andrii: force absolute path: this line to be removed once uniflow migrated to a new format
	if !path.IsAbs(p) {
		return nil, fmt.Errorf("400: absolute path required: %s", p)
	}
	if content, found := r.files[p]; found {
		return content, nil
	} else {
		return nil, fmt.Errorf("404: file not found: %s, %w", p, ErrNotExist)
	}
}

func TarFS(tar []byte) (FS, error) {
	files := map[string][]byte{}
	br := bytes.NewReader(tar)
	if err := ext.TarRead(br, func(p string, content []byte) error {
		p = path.Clean("/" + p)          // sanitize path: make it absolute and clean
		if _, found := files[p]; found { // validate file collisions
			return fmt.Errorf("400: tar file collision: %s", p)
		}
		files[p] = content
		return nil
	}); err != nil {
		return nil, err
	}
	return &tarFS{files: files}, nil
}

type LocalFS struct {
	Root string
}

var _ FS = &LocalFS{}

func (r *LocalFS) Read(path string) ([]byte, error) {
	path = filepath.Join(r.Root, path)
	return os.ReadFile(path)
}
