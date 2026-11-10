package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("nowhere to write")
}

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		want     Config
		usageErr bool
	}{
		{name: "empty", want: Config{LogLevel: slog.LevelInfo}},
		{name: "version", args: []string{"-version"}, want: Config{Version: true, LogLevel: slog.LevelInfo}},
		{name: "project", args: []string{"-project", "/p"}, want: Config{Project: "/p", LogLevel: slog.LevelInfo}},
		{name: "env overrides project flag", args: []string{"-project", "/p"},
			env:  map[string]string{"GAME_PROJECT": "/e"},
			want: Config{Project: "/e", LogLevel: slog.LevelInfo}},
		{name: "env overrides log-level flag", args: []string{"-log-level", "error"},
			env:  map[string]string{"GAME_LOG_LEVEL": "debug"},
			want: Config{LogLevel: slog.LevelDebug}},
		{name: "everything after -- goes to Godot", args: []string{"--", "--verbose", "--path", "/g"},
			want: Config{LogLevel: slog.LevelInfo, Godot: []string{"--verbose", "--path", "/g"}}},
		{name: "own flags and Godot", args: []string{"-project", "/p", "--", "--verbose"},
			want: Config{Project: "/p", LogLevel: slog.LevelInfo, Godot: []string{"--verbose"}}},
		{name: "unknown flag", args: []string{"--verbose"}, usageErr: true},
		{name: "positional argument without --", args: []string{"x"}, usageErr: true},
		{name: "log-file", args: []string{"-log-file", "/l"}, want: Config{LogLevel: slog.LevelInfo, LogFile: "/l"}},
		{name: "env overrides log-file flag", args: []string{"-log-file", "/l"},
			env:  map[string]string{"GAME_LOG_FILE": "/e"},
			want: Config{LogLevel: slog.LevelInfo, LogFile: "/e"}},
		{name: "profiles", args: []string{"-cpuprofile", "/cpu", "-memprofile", "/mem"},
			want: Config{LogLevel: slog.LevelInfo, CPUProfile: "/cpu", MemProfile: "/mem"}},
		{name: "env overrides profile flags", args: []string{"-cpuprofile", "/cpu", "-memprofile", "/mem"},
			env:  map[string]string{"GAME_CPU_PROFILE": "/ecpu", "GAME_MEM_PROFILE": "/emem"},
			want: Config{LogLevel: slog.LevelInfo, CPUProfile: "/ecpu", MemProfile: "/emem"}},
		{name: "bad log-level", args: []string{"-log-level", "loud"}, usageErr: true},
		{name: "bad log-level in env", env: map[string]string{"GAME_LOG_LEVEL": "loud"}, usageErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse("game", "GAME", tt.args, envOf(tt.env), &bytes.Buffer{})
			if tt.usageErr {
				assert.ErrorIs(t, err, ErrUsage)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("flag error is not printed, usage is", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		_, err := Parse("game", "GAME", []string{"-bogus"}, envOf(nil), &out)
		require.ErrorIs(t, err, ErrUsage)
		assert.NotContains(t, out.String(), "bogus", "main logs the error")
		assert.Contains(t, out.String(), "-log-level")
	})

	t.Run("help", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		_, err := Parse("game", "GAME", []string{"-h"}, envOf(nil), &out)
		require.ErrorIs(t, err, flag.ErrHelp)
		assert.Contains(t, out.String(), "Usage: game")
		assert.Contains(t, out.String(), "-cpuprofile")
	})

	t.Run("help write error", func(t *testing.T) {
		t.Parallel()
		_, err := Parse("game", "GAME", []string{"-h"}, envOf(nil), brokenWriter{})
		require.ErrorContains(t, err, "usage")
		assert.NotErrorIs(t, err, ErrUsage, "write failure is not a user error")
		assert.NotErrorIs(t, err, flag.ErrHelp)
	})
}

func TestParse_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}
	full := file("full.json", `{"project": "/f", "log_level": "debug"}`)
	logs := file("logs.json", `{"log_file": "/f.log"}`)
	profiles := file("profiles.json", `{"cpu_profile": "/f.cpu", "mem_profile": "/f.mem"}`)
	part := file("part.json", `{"project": "/f"}`)
	other := file("other.json", `{"log_level": "error"}`)
	unknown := file("unknown.json", `{"projekt": "/f"}`)
	broken := file("broken.json", `{`)
	tail := file("tail.json", `{"project": "/f"} garbage`)
	bracket := file("bracket.json", `{"project": "/f"} ]`)
	spaces := file("spaces.json", "{\"project\": \"/f\"}\n\n")
	badLevel := file("badlevel.json", `{"log_level": "shout"}`)
	godot := file("godot.json", `{"godot_args": ["--headless", "--quit-after", "10"]}`)
	missing := filepath.Join(dir, "missing.json")

	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		want     Config
		usageErr bool
	}{
		{name: "-c", args: []string{"-c", full}, want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "-config", args: []string{"-config", full}, want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "env GAME_CONFIG overrides flag", args: []string{"-c", other}, env: map[string]string{"GAME_CONFIG": full},
			want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "env CONFIG overrides flag", args: []string{"-c", other}, env: map[string]string{"CONFIG": full},
			want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "env GAME_CONFIG overrides CONFIG", env: map[string]string{"CONFIG": other, "GAME_CONFIG": full},
			want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "flag overrides file", args: []string{"-c", full, "-project", "/p", "-log-level", "warn"},
			want: Config{Project: "/p", LogLevel: slog.LevelWarn}},
		{name: "env overrides file", args: []string{"-c", full},
			env:  map[string]string{"GAME_PROJECT": "/e", "GAME_LOG_LEVEL": "error"},
			want: Config{Project: "/e", LogLevel: slog.LevelError}},
		{name: "log_file from file", args: []string{"-c", logs}, want: Config{LogLevel: slog.LevelInfo, LogFile: "/f.log"}},
		{name: "log-file flag overrides file", args: []string{"-c", logs, "-log-file", "/l"},
			want: Config{LogLevel: slog.LevelInfo, LogFile: "/l"}},
		{name: "profiles from file", args: []string{"-c", profiles},
			want: Config{LogLevel: slog.LevelInfo, CPUProfile: "/f.cpu", MemProfile: "/f.mem"}},
		{name: "cpuprofile flag overrides file", args: []string{"-c", profiles, "-cpuprofile", "/c"},
			want: Config{LogLevel: slog.LevelInfo, CPUProfile: "/c", MemProfile: "/f.mem"}},
		{name: "missing field uses default", args: []string{"-c", part},
			want: Config{Project: "/f", LogLevel: slog.LevelInfo}},
		{name: "whitespace after JSON", args: []string{"-c", spaces},
			want: Config{Project: "/f", LogLevel: slog.LevelInfo}},
		{name: "garbage after JSON", args: []string{"-c", tail}, usageErr: true},
		{name: "bracket after JSON", args: []string{"-c", bracket}, usageErr: true},
		{name: "unknown field", args: []string{"-c", unknown}, usageErr: true},
		{name: "broken JSON", args: []string{"-c", broken}, usageErr: true},
		{name: "missing file", args: []string{"-c", missing}, usageErr: true},
		{name: "invalid log_level", args: []string{"-c", badLevel}, usageErr: true},
		{name: "invalid log_level, but flag overrides", args: []string{"-c", badLevel, "-log-level", "warn"},
			want: Config{LogLevel: slog.LevelWarn}},
		{name: "-c and -config are the same file", args: []string{"-c", full, "-config", full},
			want: Config{Project: "/f", LogLevel: slog.LevelDebug}},
		{name: "godot_args from file", args: []string{"-c", godot},
			want: Config{LogLevel: slog.LevelInfo, Godot: []string{"--headless", "--quit-after", "10"}}},
		{name: "arguments after -- override godot_args", args: []string{"-c", godot, "--", "--verbose"},
			want: Config{LogLevel: slog.LevelInfo, Godot: []string{"--verbose"}}},
		{name: "-c and -config are different files", args: []string{"-c", full, "-config", part}, usageErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse("game", "GAME", tt.args, envOf(tt.env), &bytes.Buffer{})
			if tt.usageErr {
				assert.ErrorIs(t, err, ErrUsage)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("file and env errors are joined", func(t *testing.T) {
		t.Parallel()
		_, err := Parse("game", "GAME", []string{"-c", badLevel}, envOf(map[string]string{"GAME_LOG_LEVEL": "loud"}),
			&bytes.Buffer{})
		require.ErrorIs(t, err, ErrUsage)
		assert.ErrorContains(t, err, "log_level")
		assert.ErrorContains(t, err, "GAME_LOG_LEVEL")
	})

	t.Run("syntax error after JSON is wrapped", func(t *testing.T) {
		t.Parallel()
		_, err := Parse("game", "GAME", []string{"-c", tail}, envOf(nil), &bytes.Buffer{})
		var syntaxErr *json.SyntaxError
		assert.ErrorAs(t, err, &syntaxErr)
	})
}

func TestParse_Prefix(t *testing.T) {
	t.Parallel()
	env := envOf(map[string]string{"GAME_PROJECT": "g", "DEMO_PROJECT": "d", "DEMO_LOG_LEVEL": "warn"})
	var out bytes.Buffer
	_, err := Parse("demo", "DEMO", []string{"-h"}, env, &out)
	require.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, out.String(), "Usage: demo [flags]")
	assert.Contains(t, out.String(), "env DEMO_PROJECT takes precedence")
	assert.NotContains(t, out.String(), "GAME")

	cfg, err := Parse("demo", "DEMO", nil, env, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "d", cfg.Project)
	assert.Equal(t, slog.LevelWarn, cfg.LogLevel)

	_, err = Parse("demo", "DEMO", nil, envOf(map[string]string{"DEMO_LOG_LEVEL": "loud"}), io.Discard)
	require.ErrorIs(t, err, ErrUsage)
	assert.ErrorContains(t, err, "DEMO_LOG_LEVEL")
}

// BenchmarkParse measures parsing flags, env and the JSON file at startup.
func BenchmarkParse(b *testing.B) {
	file := filepath.Join(b.TempDir(), "config.json")
	require.NoError(b, os.WriteFile(file, []byte(`{"log_level":"debug","cpu_profile":"cpu.out"}`), 0o600))
	args := []string{"-c", file, "-project", "graphics", "-memprofile", "mem.out", "--", "--verbose"}
	env := envOf(map[string]string{"GAME_LOG_FILE": "game.log"})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse("game", "GAME", args, env, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
