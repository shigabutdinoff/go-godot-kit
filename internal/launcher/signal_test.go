//go:build cgo && (darwin || linux)

package launcher

import (
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalOf(t *testing.T) {
	t.Parallel()
	ctx, stop := signal.NotifyContext(t.Context(), syscall.SIGUSR1)
	defer stop()
	assert.Empty(t, signalOf(ctx), "context is alive")
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		require.FailNow(t, "signal not delivered within 5 s")
	}
	stop()
	assert.Contains(t, signalOf(ctx), "user defined signal 1", "signal is visible after stop")

	stopped, stopOnly := signal.NotifyContext(t.Context(), syscall.SIGUSR2)
	stopOnly()
	assert.Empty(t, signalOf(stopped), "cancellation via stop is not a signal")
}
