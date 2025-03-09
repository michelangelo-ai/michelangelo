package ext

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func TarWrite(src map[string][]byte, out io.Writer) error {
	gw := gzip.NewWriter(out)
	tw := tar.NewWriter(gw)
	for path, body := range src {
		hdr := &tar.Header{
			Name: path,
			Mode: 0600,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(body); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gw.Close(); err != nil {
		return err
	}
	return nil
}

func TarRead(src io.Reader, callback func(path string, content []byte) error) error {
	gr, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		var bb bytes.Buffer
		if _, err := io.Copy(&bb, tr); err != nil {
			return err
		}
		if err := callback(hdr.Name, bb.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func TarDir(root string, out io.Writer) error {
	var err error
	if root, err = filepath.Abs(root); err != nil {
		return err
	}
	files := map[string][]byte{}
	if err := ReadDir(root, func(path string, content []byte) error {
		files[path[len(root)+1:]] = content
		return nil
	}); err != nil {
		return err
	}
	return TarWrite(files, out)
}

func ReadDir(root string, callback func(path string, content []byte) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if b, err := os.ReadFile(path); err != nil {
			return err
		} else {
			return callback(path, b)
		}
	})
}
