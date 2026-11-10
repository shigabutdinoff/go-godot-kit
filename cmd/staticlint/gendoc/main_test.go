package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	doc := "// head\n" + begin + "\n//   - old\n//\n" + end + "\n// tail\n"
	got, err := update(doc, []string{"SA1000 — Invalid regular expression", "S1000 — Use plain channel"})
	require.NoError(t, err)
	assert.Equal(t, "// head\n"+begin+"\n//\n"+
		"//   - SA1000 — Invalid regular expression\n//   - S1000 — Use plain channel\n//\n"+
		end+"\n// tail\n", got)

	again, err := update(got, []string{"SA1000 — Invalid regular expression", "S1000 — Use plain channel"})
	require.NoError(t, err)
	assert.Equal(t, got, again, "rerun changes nothing")

	_, err = update("// no markers\n", nil)
	assert.Error(t, err)
}

func TestRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.go")
	require.NoError(t, os.WriteFile(path, []byte(begin+"\n//\n"+end+"\n"), 0o644))
	require.NoError(t, run(path))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "//   - SA1000 — ", "staticcheck checks are listed")
	assert.Contains(t, string(got), "//   - S1000 — ", "simple checks are listed")
	assert.NotContains(t, string(got), `\'`, "staticcheck markup is replaced")

	assert.ErrorIs(t, run(filepath.Join(dir, "missing.go")), os.ErrNotExist)

	bad := filepath.Join(dir, "bad.go")
	require.NoError(t, os.WriteFile(bad, []byte("// no markers\n"), 0o644))
	assert.ErrorContains(t, run(bad), "bad.go")
}
