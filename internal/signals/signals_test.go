//go:build cgo && (darwin || linux || windows)

package signals

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuit(t *testing.T) {
	t.Parallel()
	assert.Contains(t, Quit, os.Interrupt)
	assert.Contains(t, Quit, syscall.SIGTERM)
}

func TestSaveRestore(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() {
		Save()
		Restore()
	}, "restoring the saved handlers leaves Go's handlers in place")
}
