//go:build !cgo || !(darwin || linux || windows)

package launcher

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_Other runs Run in a child copy of the test binary, since Run exits the process.
func TestRun_Other(t *testing.T) {
	if os.Getenv("KIT_TEST_RUN") == "1" {
		Run(Game{Name: "game", EnvPrefix: "GAME"})
		return
	}
	t.Parallel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRun_Other$")
	cmd.Env = append(os.Environ(), "KIT_TEST_RUN=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit, "stderr: %s", stderr.String())
	assert.Equal(t, 1, exit.ExitCode())
	assert.Contains(t, stdout.String(), "Build version: N/A")
	assert.Contains(t, stderr.String(), "unsupported platform")
	assert.Contains(t, stderr.String(), `"service.name":"game"`)
}
