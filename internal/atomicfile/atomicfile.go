// Package atomicfile writes files so that a failure never leaves one half written.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write writes data to path with perm through a temporary file in the same directory, renamed over path at
// the end: path holds either its old content or all of data, even after a power loss. A symlink at path is
// replaced, not followed.
func Write(path string, data []byte, perm fs.FileMode) error {
	return WriteFrom(path, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// WriteFrom is [Write] with the content written by write instead of a byte slice.
func WriteFrom(path string, perm fs.FileMode, write func(io.Writer) error) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	// After a successful rename tmp is gone; a failed cleanup must not hide the write error.
	defer func() { _ = os.Remove(tmp.Name()) }()
	err = write(tmp)
	if err == nil {
		// Sync before rename: otherwise after a power loss the rename may be on disk while the data is not.
		err = tmp.Sync()
	}
	if err = errors.Join(err, tmp.Close(), os.Chmod(tmp.Name(), perm)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
