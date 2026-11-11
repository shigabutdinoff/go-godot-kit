package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartProfiles(t *testing.T) {
	dir := t.TempDir()
	cpu, mem := filepath.Join(dir, "cpu.pprof"), filepath.Join(dir, "mem.pprof")

	t.Run("both profiles", func(t *testing.T) {
		stop, err := Start(cpu, mem)
		require.NoError(t, err)
		_, again := Start(filepath.Join(dir, "again.pprof"), "")
		require.ErrorContains(t, again, "CPU profile", "a second CPU profile does not start")
		require.NoError(t, stop())
		for _, p := range []string{cpu, mem} {
			fi, err := os.Stat(p)
			require.NoError(t, err)
			assert.Positive(t, fi.Size(), p)
		}
	})
	t.Run("nothing set", func(t *testing.T) {
		stop, err := Start("", "")
		require.NoError(t, err)
		assert.NoError(t, stop())
	})
	t.Run("missing CPU directory", func(t *testing.T) {
		_, err := Start(filepath.Join(dir, "missing", "cpu.pprof"), "")
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("relative memory path resolves against the start directory despite Godot changing it", func(t *testing.T) {
		t.Chdir(dir)
		stop, err := Start("", "rel.pprof")
		require.NoError(t, err)
		require.NoError(t, os.Chdir(t.TempDir()))
		require.NoError(t, stop())
		assert.FileExists(t, filepath.Join(dir, "rel.pprof"))
	})
	t.Run("missing memory directory: error on stop", func(t *testing.T) {
		stop, err := Start("", filepath.Join(dir, "missing", "mem.pprof"))
		require.NoError(t, err)
		assert.ErrorIs(t, stop(), os.ErrNotExist)
	})
}
