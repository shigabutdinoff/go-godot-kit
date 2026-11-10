package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.go")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	require.NoError(t, Write(path, []byte("new"), 0o644))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}
	left, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, left, 1, "no temporary file is left")
}

func TestWrite_MissingDir(t *testing.T) {
	t.Parallel()
	assert.Error(t, Write(filepath.Join(t.TempDir(), "missing", "doc.go"), nil, 0o644))
}

func TestWriteFrom_Error(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.go")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	errWrite := errors.New("write failed")

	err := WriteFrom(path, 0o644, func(w io.Writer) error {
		_, _ = w.Write([]byte("half"))
		return errWrite
	})
	require.ErrorIs(t, err, errWrite)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old", string(got), "a failed write keeps the old content")
	left, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, left, 1, "no temporary file is left")
}
