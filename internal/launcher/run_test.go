//go:build cgo && (darwin || linux || windows)

package launcher

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/go-godot-kit/internal/config"
)

var testGame = Game{Name: "game", EnvPrefix: "GAME"}

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("nowhere to write")
}

func TestPrepare(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "graphics"), 0o755))
	g := testGame
	g.Source = filepath.Join(dir, "main.go")
	exe := func() (string, error) { return "", errors.New("no path") }
	tests := []struct {
		name    string
		argv    []string
		env     map[string]string
		want    []string
		wantErr error
	}{
		{name: "-version: do not start the game", argv: []string{"game", "-version"}},
		{name: "unknown flag", argv: []string{"game", "-x"}, wantErr: config.ErrUsage},
		{name: "Godot gets its own --path", argv: []string{"game", "--", "--path", "/p"},
			want: []string{"game", "--path", "/p"}},
		{name: "no executable path: project next to the source", argv: []string{"game"},
			want: []string{"game", "--path", filepath.Join(dir, "graphics")}},
		{name: "GAME_PROJECT does not exist", argv: []string{"game"},
			env: map[string]string{"GAME_PROJECT": filepath.Join(dir, "missing")}, wantErr: os.ErrNotExist},
		{name: "another prefix is not read", argv: []string{"game"},
			env:  map[string]string{"DEMO_PROJECT": filepath.Join(dir, "missing")},
			want: []string{"game", "--path", filepath.Join(dir, "graphics")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			args, cleanup, err := prepare(tt.argv, envOf(tt.env), exe, g, &stdout, &stderr, &slog.LevelVar{})
			defer func() { assert.NoError(t, cleanup()) }()
			assert.True(t, strings.HasPrefix(stdout.String(), "Build version: "), "version comes first")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, args)
		})
	}
}

func TestPrepare_WriteError(t *testing.T) {
	t.Parallel()
	_, cleanup, err := prepare([]string{"game", "-version"}, envOf(nil), os.Executable, testGame,
		brokenWriter{}, io.Discard, &slog.LevelVar{})
	require.ErrorContains(t, err, "version")
	require.NoError(t, cleanup())
}

func TestWatchSignal(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errors.New("interrupt signal received"))
	stopped := false
	watchSignal(ctx, func() { stopped = true })
	assert.True(t, stopped, "handler removed after the first signal")
}

func TestFinish(t *testing.T) {
	t.Parallel()
	assert.Equal(t, exitOK, finish(nil))
	assert.Equal(t, exitOK, finish(flag.ErrHelp))
	assert.Equal(t, exitUsage, finish(config.ErrUsage))
	assert.Equal(t, exitError, finish(errors.New("failure")))
}

func TestPrepare_LogFile(t *testing.T) {
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	dir := t.TempDir()
	log := filepath.Join(dir, "game.log")
	argv := []string{"game", "-log-file", log, "--", "--path", "/p"}
	_, cleanup, err := prepare(argv, envOf(nil), os.Executable, testGame, io.Discard, io.Discard, &slog.LevelVar{})
	require.NoError(t, err)
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"msg":"start"`, "logs go to the file")
	assert.Contains(t, string(data), `"service.name":"game"`, "service name is the game name")
	assert.Contains(t, string(data), `"service.instance.id"`, "run id is in every record")
	require.NoError(t, cleanup(), "log file is closed explicitly")

	_, cleanup, err = prepare([]string{"game", "-log-file", filepath.Join(dir, "missing", "x.log")}, envOf(nil),
		os.Executable, testGame, io.Discard, io.Discard, &slog.LevelVar{})
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, exitError, exitCode(err))
	require.NoError(t, cleanup())
}

func TestRun_Version(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"game", "-version"}
	require.NoError(t, run(testGame))
	assert.Equal(t, []string{"game", "-version"}, os.Args, "Godot arguments are not substituted")
}

func TestRun_BadFlag(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"game", "-no-such-flag"}
	err := run(testGame)
	require.ErrorIs(t, err, config.ErrUsage)
	assert.Equal(t, exitUsage, exitCode(err))
}

func TestRun_ErrorInLogFile(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	log := filepath.Join(t.TempDir(), "game.log")
	os.Args = []string{"game", "-log-file", log, "-project", filepath.Join(t.TempDir(), "missing")}
	err := run(testGame)
	require.ErrorIs(t, err, os.ErrNotExist)
	data, readErr := os.ReadFile(log)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"msg":"run failed"`, "run error goes to the log file too")
}

func TestClosed(t *testing.T) {
	t.Parallel()
	ch := make(chan struct{})
	assert.False(t, closed(ch), "open channel")
	close(ch)
	assert.True(t, closed(ch), "closed channel")
}

func TestPrepare_Profiles(t *testing.T) {
	dir := t.TempDir()
	cpu, mem := filepath.Join(dir, "cpu.pprof"), filepath.Join(dir, "mem.pprof")
	argv := []string{"game", "-cpuprofile", cpu, "-memprofile", mem, "--", "--path", "/p"}
	_, cleanup, err := prepare(argv, envOf(nil), os.Executable, testGame, io.Discard, io.Discard, &slog.LevelVar{})
	require.NoError(t, err)
	require.NoError(t, cleanup(), "cleanup flushes profiles")
	for _, p := range []string{cpu, mem} {
		fi, statErr := os.Stat(p)
		require.NoError(t, statErr)
		assert.Positive(t, fi.Size(), p)
	}

	argv = []string{"game", "-cpuprofile", filepath.Join(dir, "missing", "cpu.pprof"), "--", "--path", "/p"}
	_, cleanup, err = prepare(argv, envOf(nil), os.Executable, testGame, io.Discard, io.Discard, &slog.LevelVar{})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, cleanup())
}

func TestShutdown(t *testing.T) {
	t.Parallel()
	t.Run("not set", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, shutdown(testGame, time.Second))
	})
	t.Run("gets a context with the timeout and its error is wrapped", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("save failed")
		g := testGame
		g.Shutdown = func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.WithinDuration(t, time.Now().Add(shutdownTimeout), deadline, time.Second)
			return boom
		}
		err := shutdown(g, shutdownTimeout)
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "shutdown")
	})
	t.Run("a Shutdown that ignores ctx is abandoned after the timeout", func(t *testing.T) {
		t.Parallel()
		unblock := make(chan struct{})
		t.Cleanup(func() { close(unblock) })
		g := testGame
		g.Shutdown = func(context.Context) error {
			<-unblock // ignores ctx, as a stuck game would
			return nil
		}
		start := time.Now()
		assert.ErrorIs(t, shutdown(g, 50*time.Millisecond), errShutdownTimeout)
		assert.Less(t, time.Since(start), shutdownTimeout)
	})
	t.Run("game context is canceled before Shutdown", func(t *testing.T) {
		t.Parallel()
		ctx, stop := context.WithCancel(t.Context())
		g := testGame
		g.Shutdown = func(context.Context) error { return ctx.Err() }
		assert.ErrorIs(t, stopThenShutdown(stop, g), context.Canceled, "Shutdown sees the game context canceled")
	})
}
