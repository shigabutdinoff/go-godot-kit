//go:build cgo && (darwin || linux)

package signals

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSignalCount(t *testing.T) {
	t.Parallel()
	assert.Equal(t, len(Quit), cSignalCount(), "the C list goSigs and Quit hold the same signals")
}

// TestSave_Guard replaces the SIGINT handler while Save guards it, as LibGodot does while loading, and checks
// that Go gets the next SIGINT. It is not parallel: signal handlers are shared by the whole process.
func TestSave_Guard(t *testing.T) {
	got := make(chan os.Signal, 1)
	signal.Notify(got, os.Interrupt)
	defer signal.Stop(got)

	Save()
	defer Restore()
	installSwallower()
	require.Eventually(t, goHandlersInstalled, time.Second, time.Millisecond, "the guard puts Go's handler back")

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGINT did not reach Go")
	}
}
