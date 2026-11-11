package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/go-godot-kit/internal/config"
)

func TestExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"no error", nil, exitOK},
		{"help", flag.ErrHelp, exitOK},
		{"flag error", fmt.Errorf("%w: -x", config.ErrUsage), exitUsage},
		{"other error", errors.New("boom"), exitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, exitCode(tt.err))
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v, d, c string
		want    string
	}{
		{name: "all empty", want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A"},
		{name: "all set", v: "v1", d: "today", c: "def",
			want: "Build version: v1\nBuild date: today\nBuild commit: def"},
		{name: "version only", v: "v1", want: "Build version: v1\nBuild date: N/A\nBuild commit: N/A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			require.NoError(t, writeVersion(&buf, Build{Version: tt.v, Date: tt.d, Commit: tt.c}))
			assert.Equal(t, tt.want+"\n", buf.String())
		})
	}
}

func TestHostArch(t *testing.T) {
	t.Parallel()
	for goarch, want := range map[string]string{"386": "x86", "arm": "arm32", "amd64": "amd64", "s390x": "s390x"} {
		assert.Equal(t, want, hostArch(goarch), goarch)
	}
}

func TestMaskHome(t *testing.T) {
	t.Parallel()
	root := t.TempDir() // absolute on every OS, unlike /home on Windows
	home := filepath.Join(root, "player")
	for _, tt := range []struct {
		name, path, home, want string
	}{
		{name: "inside home", path: filepath.Join(home, "game", "graphics"), home: home, want: filepath.Join("~", "game", "graphics")},
		{name: "home itself", path: home, home: home, want: "~"},
		{name: "outside home", path: filepath.Join(root, "opt"), home: home, want: filepath.Join(root, "opt")},
		{name: "sibling with home prefix", path: home + "2", home: home, want: home + "2"},
		{name: "relative path", path: "graphics", home: home, want: "graphics"},
		{name: "error text", path: "stat " + filepath.Join(home, "game") + ": not found", home: home, want: "stat " + filepath.Join("~", "game") + ": not found"},
		{name: "no home", path: filepath.Join(home, "game"), home: "", want: filepath.Join(home, "game")},
		{name: "empty path", path: "", home: home, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, maskHome(tt.path, tt.home))
		})
	}
}

func TestHomeMasker(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "player")
	game := filepath.Join(home, "game")
	var buf bytes.Buffer
	l := slog.New(homeMasker{Handler: slog.NewJSONHandler(&buf, nil), home: home}).With("custom.base", game)
	l.Info("open "+game, "custom.path", game, "process.command_args", []string{"--path", game},
		"exception.message", errors.New("stat "+game+": not found"), slog.Group("custom.g", "path", game), "custom.n", 1)
	assert.NotContains(t, buf.String(), home)
	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	masked := filepath.Join("~", "game")
	assert.Equal(t, masked, rec["custom.base"])
	assert.Equal(t, "open "+masked, rec["msg"])
	assert.Equal(t, []any{"--path", masked}, rec["process.command_args"])
	assert.Equal(t, "stat "+masked+": not found", rec["exception.message"])
	assert.Equal(t, map[string]any{"path": masked}, rec["custom.g"])
	assert.InDelta(t, 1, rec["custom.n"], 0)
}
