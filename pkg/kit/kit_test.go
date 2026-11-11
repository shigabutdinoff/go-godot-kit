package kit

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun runs Run with -version in a child copy of the test binary, since Run exits the process:
// the build version from Game reaches stdout and the game is not started.
func TestRun(t *testing.T) {
	if os.Getenv("KIT_TEST_RUN") == "1" {
		os.Args = []string{"game", "-version"}
		Run(Game{Name: "game", EnvPrefix: "GAME", BuildVersion: "v1.2.3"})
		return
	}
	t.Parallel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRun$")
	cmd.Env = append(os.Environ(), "KIT_TEST_RUN=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "-version exits with code 0; stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "Build version: v1.2.3")
}
