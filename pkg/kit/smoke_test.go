//go:build cgo && (darwin || linux || windows)

package kit

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/go-godot-kit/internal/libgodot"
)

// moduleRoot is the module root relative to this package, where lib/ and testdata/game lie.
const moduleRoot = "../.."

// TestSmoke builds testdata/game at the module root and runs Godot headless: the game exits on its own and on OS signals,
// writing the memory profile to a relative path, and exits with code 1 if the engine does not start.
// It needs LibGodot in lib/ at the module root (libgodot.dylib, libgodot.so or libgodot.dll), which CI lacks.
func TestSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test runs Godot: skipped in -short")
	}
	if _, err := os.Stat(filepath.Join(moduleRoot, "lib", libgodot.Name(runtime.GOOS))); err != nil {
		t.Skipf("LibGodot not found: %v", err)
	}
	project, err := filepath.Abs(filepath.Join(moduleRoot, "testdata", "game", "graphics"))
	require.NoError(t, err)
	bin := filepath.Join(t.TempDir(), "game")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "./testdata/game")
	build.Dir = moduleRoot
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	// start runs the game in a new directory and returns the directory, the process and a channel of stderr log lines.
	start := func(t *testing.T, ctx context.Context, quitAfter string) (string, *exec.Cmd, <-chan string) {
		t.Helper()
		dir := t.TempDir()
		cmd := exec.CommandContext(ctx, bin, "-log-level", "debug", "-memprofile", "mem.pprof",
			"-project", project, "--", "--headless", "--quit-after", quitAfter)
		cmd.Dir = dir
		stderr, err := cmd.StderrPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())
		lines := make(chan string, 64)
		go func() {
			defer close(lines)
			s := bufio.NewScanner(stderr)
			for s.Scan() {
				lines <- s.Text()
			}
			if err := s.Err(); err != nil {
				lines <- "stderr read error: " + err.Error()
			}
		}()
		// On a failed check the test ends before draining: ctx cancels the game, and reading the rest lets
		// the scanner goroutine finish instead of blocking on a full channel.
		t.Cleanup(func() {
			for range lines {
			}
		})
		return dir, cmd, lines
	}
	// waitFor reads the log up to the line with msg and returns all text read.
	waitFor := func(t *testing.T, lines <-chan string, msg string) string {
		t.Helper()
		var b strings.Builder
		for l := range lines {
			b.WriteString(l + "\n")
			if strings.Contains(l, `"msg":"`+msg+`"`) {
				return b.String()
			}
		}
		require.FailNow(t, "log record not found", "%s; log:\n%s", msg, b.String())
		return ""
	}
	// drain reads stderr to the end: Wait must not be called before that (see exec.Cmd.StderrPipe).
	drain := func(lines <-chan string) {
		for range lines {
		}
	}

	t.Run("exit on --quit-after", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		dir, cmd, lines := start(t, ctx, "10")
		log := waitFor(t, lines, "game saved")
		drain(lines)
		require.NoError(t, cmd.Wait())
		assert.Contains(t, log, `"service.name":"game"`)
		assert.FileExists(t, filepath.Join(dir, "mem.pprof"), "profile is in the start directory, not the project directory")
	})
	t.Run("engine does not start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-log-level", "debug", "--", "--headless",
			"--path", filepath.Join(t.TempDir(), "missing"))
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit, string(out))
		assert.Equal(t, 1, exit.ExitCode(), string(out))
		assert.Contains(t, string(out), "engine did not start")
		assert.NotContains(t, string(out), "SIGSEGV")
	})
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT} {
		t.Run("exit on "+sig.String(), func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Process.Signal cannot send Ctrl+C on Windows: check it by hand with go run")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			dir, cmd, lines := start(t, ctx, "1000000")
			waitFor(t, lines, "engine loaded") // SIGINT now belongs to Go, not LibGodot
			require.NoError(t, cmd.Process.Signal(sig))
			log := waitFor(t, lines, "exit")
			drain(lines)
			require.NoError(t, cmd.Wait(), "exit code 0")
			assert.Contains(t, log, `"msg":"OS signal, exiting"`)
			assert.FileExists(t, filepath.Join(dir, "mem.pprof"))
		})
	}
}
