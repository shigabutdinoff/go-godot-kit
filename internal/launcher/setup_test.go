package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/go-godot-kit/internal/config"
)

func TestProjectPath(t *testing.T) {
	t.Parallel()
	dist, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := t.TempDir()
	exe, src := filepath.Join(dist, "game"), filepath.Join(repo, "main.go")
	require.NoError(t, os.Mkdir(filepath.Join(repo, "graphics"), 0o755))
	require.NoError(t, os.WriteFile(exe, nil, 0o755))

	t.Run("no graphics/ in dist: use the repository", func(t *testing.T) {
		got, err := projectPath(exe, src)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(repo, "graphics"), got)
	})
	t.Run("graphics/ next to the executable wins", func(t *testing.T) {
		require.NoError(t, os.Mkdir(filepath.Join(dist, "graphics"), 0o755))
		got, err := projectPath(exe, src)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dist, "graphics"), got)
	})
	t.Run("not found anywhere", func(t *testing.T) {
		x := filepath.Join(t.TempDir(), "x")
		require.NoError(t, os.WriteFile(x, nil, 0o755))
		got, err := projectPath(x, filepath.Join(t.TempDir(), "y"))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("missing executable: EvalSymlinks error", func(t *testing.T) {
		_, err := projectPath(filepath.Join(t.TempDir(), "x"), src)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("symlink to the executable", func(t *testing.T) {
		realDir, links := t.TempDir(), t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(realDir, "graphics"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(realDir, "game"), nil, 0o755))
		link := filepath.Join(links, "game")
		require.NoError(t, os.Symlink(filepath.Join(realDir, "game"), link))
		got, err := projectPath(link, "")
		require.NoError(t, err)
		want, err := filepath.EvalSymlinks(filepath.Join(realDir, "graphics"))
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("graphics is a file, not a directory", func(t *testing.T) {
		d := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(d, "graphics"), nil, 0o644))
		got, err := projectPath("", filepath.Join(d, "main.go"))
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("no permission on the parent", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("mode 0 has no effect for root")
		}
		d := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(d, "graphics"), 0o755))
		require.NoError(t, os.Chmod(d, 0))
		t.Cleanup(func() { assert.NoError(t, os.Chmod(d, 0o755)) })
		got, err := projectPath("", filepath.Join(d, "main.go"))
		require.Error(t, err)
		assert.Empty(t, got)
	})
}

func TestGodotArgs(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	src := filepath.Join(repo, "main.go")
	gr := filepath.Join(repo, "graphics")
	require.NoError(t, os.Mkdir(gr, 0o755))
	file := filepath.Join(repo, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	missing := filepath.Join(repo, "missing")
	empty := filepath.Join(t.TempDir(), "main.go")

	tests := []struct {
		name    string
		cfg     config.Config
		src     string
		want    []string
		errHas  []string
		errWrap bool
	}{
		{name: "explicit --path is kept", cfg: config.Config{Godot: []string{"--path", "/g"}, Project: gr},
			src: src, want: []string{"game", "--path", "/g"}},
		{name: "-project", cfg: config.Config{Project: gr}, src: empty, want: []string{"game", "--path", gr}},
		{name: "-project does not exist", cfg: config.Config{Project: missing}, src: src,
			errHas: []string{missing}, errWrap: true},
		{name: "-project is a file", cfg: config.Config{Project: file}, src: src, errHas: []string{file}},
		{name: "search next to the source", src: src, want: []string{"game", "--path", gr}},
		{name: "Godot arguments before --path", cfg: config.Config{Godot: []string{"--verbose"}}, src: src,
			want: []string{"game", "--verbose", "--path", gr}},
		{name: "nothing found", src: empty, errHas: []string{"-project", "GAME_PROJECT"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := godotArgs("game", tt.cfg, "", Game{EnvPrefix: "GAME", Source: tt.src}, false)
			if tt.errHas != nil {
				require.Error(t, err)
				for _, s := range tt.errHas {
					assert.Contains(t, err.Error(), s)
				}
				if tt.errWrap {
					assert.ErrorIs(t, err, os.ErrNotExist)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("search error is propagated", func(t *testing.T) {
		t.Parallel()
		if os.Geteuid() == 0 {
			t.Skip("mode 0 has no effect for root")
		}
		d := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(d, "graphics"), 0o755))
		require.NoError(t, os.Chmod(d, 0))
		t.Cleanup(func() { assert.NoError(t, os.Chmod(d, 0o755)) })
		_, err := godotArgs("game", config.Config{}, "", Game{Source: filepath.Join(d, "main.go")}, false)
		assert.Error(t, err)
	})
}

func TestGodotArgs_NotFound(t *testing.T) {
	t.Parallel()
	_, err := godotArgs("demo", config.Config{}, "", Game{Name: "demo", EnvPrefix: "DEMO"}, false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "DEMO_PROJECT", "the hint names the game's own prefix")
	assert.NotContains(t, err.Error(), "GAME_PROJECT")
}

func TestGodotArgs_Source(t *testing.T) { // graphics/ next to Game.Source, not next to the kit package
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "graphics"), 0o755))
	args, err := godotArgs("game", config.Config{}, "", Game{Source: filepath.Join(dir, "main.go")}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "--path", filepath.Join(dir, "graphics")}, args)
}

func TestGodotArgs_Release(t *testing.T) { // the scene is embedded: no --path, explicit paths are a usage error
	t.Parallel()
	args, err := godotArgs("game", config.Config{Godot: []string{"--headless"}}, "/app/game",
		Game{EnvPrefix: "GAME", Source: "/nonexistent/main.go"}, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"game", "--headless"}, args, "release: no --path, no graphics/ lookup")
	for _, cfg := range []config.Config{{Project: "/p"}, {Godot: []string{"--path", "/p"}}} {
		_, err := godotArgs("game", cfg, "", Game{EnvPrefix: "GAME"}, true)
		require.ErrorIs(t, err, config.ErrUsage)
		assert.Equal(t, exitUsage, exitCode(err))
		assert.ErrorContains(t, err, "the scene is embedded")
		assert.ErrorContains(t, err, "GAME_PROJECT")
	}
}

func TestLibgodotPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := filepath.Join(root, "cmd", "game", "main.go")
	_, err := libgodotPath(Game{Source: src}, "", "linux")
	require.Error(t, err)
	assert.ErrorContains(t, err, filepath.Join(root, "lib", "libgodot.so"))
	assert.ErrorContains(t, err, "go tool libgodot")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "lib", "libgodot.so"), nil, 0o644))
	got, err := libgodotPath(Game{Source: src}, "", "linux")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "lib", "libgodot.so"), got)

	exeDir := t.TempDir() // windows: go run puts the exe in a temporary directory; a DLL next to it is enough
	require.NoError(t, os.WriteFile(filepath.Join(exeDir, "libgodot.dll"), nil, 0o644))
	got, err = libgodotPath(Game{}, filepath.Join(exeDir, "game.exe"), "windows")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(exeDir, "libgodot.dll"), got)
	_, err = libgodotPath(Game{}, filepath.Join(exeDir, "game"), "linux")
	assert.ErrorContains(t, err, "libgodot.so", "outside windows the executable directory is not searched")
}

// BenchmarkGodotArgs measures building the Godot arguments with the project lookup next to the executable.
func BenchmarkGodotArgs(b *testing.B) {
	dir := b.TempDir()
	require.NoError(b, os.Mkdir(filepath.Join(dir, "graphics"), 0o755))
	exe := filepath.Join(dir, "game")
	require.NoError(b, os.WriteFile(exe, nil, 0o755))
	cfg := config.Config{Godot: []string{"--verbose"}}
	g := Game{Name: "game", EnvPrefix: "GAME"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := godotArgs("game", cfg, exe, g, false); err != nil {
			b.Fatal(err)
		}
	}
}
