package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const missingName = `Not enough arguments (missing: "name").`

// templateRepo makes a git repository with master from testdata/template, changed by edit if it is not nil,
// and returns its path.
func templateRepo(t *testing.T, edit func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "template"))))
	if edit != nil {
		edit(dir)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "master"},
		{"add", "-A"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "template"},
	} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

type result struct {
	code        int
	out, errOut string
}

// runIn runs the command in dir with the template, input and terminal flag given.
func runIn(t *testing.T, dir, template, input string, interactive bool, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(t.Context(), args, env{
		dir: dir, template: template, in: strings.NewReader(input), out: &out, errOut: &errOut,
		interactive: interactive,
	})
	return result{code: code, out: out.String(), errOut: errOut.String()}
}

// entries returns the names in dir.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(list))
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func read(t *testing.T, path ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(path...))
	require.NoError(t, err)
	return string(b)
}

func TestValidName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a", "mygame", "my-game", "game2", "a1-b2-c3"} {
		assert.True(t, validName(name), name)
	}
	for _, name := range []string{"", "MyGame", "my game", "1game", "my--game", "my-game-", "-game", "my_game",
		"go", "toolchain"} {
		assert.False(t, validName(name), name)
	}
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "upper case", args: []string{"MyGame"}, wantErr: "a-z"},
		{name: "space", args: []string{"my game"}, wantErr: "a-z"},
		{name: "digit first", args: []string{"1game"}, wantErr: "a-z"},
		{name: "double hyphen", args: []string{"my--game"}, wantErr: "a-z"},
		{name: "trailing hyphen", args: []string{"my-game-"}, wantErr: "a-z"},
		{name: "go", args: []string{"go"}, wantErr: "a-z"},
		{name: "toolchain", args: []string{"toolchain"}, wantErr: "a-z"},
		{name: "extra argument", args: []string{"mygame", "extra"}, wantErr: "extra"},
		{name: "unknown flag", args: []string{"-bad", "mygame"}, wantErr: "-bad"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			r := runIn(t, dir, "unused", "", true, tt.args...)
			assert.Equal(t, 2, r.code)
			assert.Contains(t, r.errOut, "  ERROR  ")
			assert.Contains(t, r.errOut, tt.wantErr)
			assert.Empty(t, entries(t, dir))
		})
	}
}

func TestRun_Help(t *testing.T) {
	t.Parallel()
	r := runIn(t, t.TempDir(), "unused", "", true, "-h")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.errOut, "-n")
}

func TestRun_MissingName(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		args        []string
		input       string
		interactive bool
	}{
		{name: "not a terminal", input: "mygame\n"},
		{name: "-n in a terminal", args: []string{"-n"}, input: "mygame\n", interactive: true},
		{name: "end of input", interactive: true},
		{name: "end of input after an invalid name", input: "My Game\n", interactive: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			r := runIn(t, dir, "unused", tt.input, tt.interactive, tt.args...)
			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.errOut, "  ERROR  "+missingName)
			assert.Empty(t, entries(t, dir))
		})
	}
}

func TestRun_Exists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "mygame", "keep.txt")
	require.NoError(t, os.Mkdir(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o644))

	r := runIn(t, dir, "unused", "", false, "mygame")

	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.errOut, "already exists")
	assert.Equal(t, []string{"mygame"}, entries(t, dir))
	assert.Equal(t, []string{"keep.txt"}, entries(t, filepath.Join(dir, "mygame")))
	assert.Equal(t, "keep", read(t, marker))
}

func TestRun_Prompt(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	for _, tt := range []struct {
		name, input, wantErr, wantGame, wantPrefix string
	}{
		{name: "invalid name", input: "My Game\nmy-game\n", wantErr: "a-z", wantGame: "my-game", wantPrefix: "MY_GAME"},
		{name: "taken folder", input: "taken\r\nmygame\r\n", wantErr: "already exists", wantGame: "mygame",
			wantPrefix: "MYGAME"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, "taken"), 0o755))

			r := runIn(t, dir, tpl, tt.input, true)

			require.Equal(t, 0, r.code, r.errOut)
			assert.Equal(t, 2, strings.Count(r.out, "What is the name of your game?"))
			assert.Contains(t, r.out, "E.g. my-game")
			assert.Contains(t, r.errOut, tt.wantErr)
			assert.ElementsMatch(t, []string{"taken", tt.wantGame}, entries(t, dir))
			main := read(t, dir, tt.wantGame, "cmd", tt.wantGame, "main.go")
			assert.Contains(t, main, `EnvPrefix: "`+tt.wantPrefix+`"`)
			assert.Contains(t, read(t, dir, tt.wantGame, "cmd", tt.wantGame, "doc.go"), tt.wantPrefix+"_CONFIG")
		})
	}
}

func TestRun_Create(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	dir := t.TempDir()

	r := runIn(t, dir, tpl, "", false, "mygame")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, []string{"mygame"}, entries(t, dir))
	game := filepath.Join(dir, "mygame")
	assert.NoDirExists(t, filepath.Join(game, ".git"))
	assert.NoDirExists(t, filepath.Join(game, "cmd", "game"))
	assert.Regexp(t, `^module mygame\s`, read(t, game, "go.mod"))
	main := read(t, game, "cmd", "mygame", "main.go")
	assert.Contains(t, main, `"mygame/internal/app"`)
	assert.Contains(t, main, `Name:      "mygame"`)
	assert.Contains(t, main, `EnvPrefix: "MYGAME"`)
	assert.Contains(t, read(t, game, "cmd", "mygame", "main_test.go"), `"MYGAME", g.EnvPrefix`)
	assert.Contains(t, read(t, game, "internal", "app", "example_test.go"), `"mygame/internal/app"`)
	assert.Contains(t, read(t, game, "cmd", "mygame", "graphics", "project.godot"), `config/name="mygame"`)
	doc := read(t, game, "cmd", "mygame", "doc.go")
	assert.NotContains(t, doc, "cmd/game")
	assert.NotRegexp(t, `[^A-Z]GAME_`, doc)
	assert.Contains(t, doc, "go run ./cmd/mygame")
	assert.Contains(t, doc, "MYGAME_LOG_LEVEL")
	for _, name := range []string{"README.md", ".golangci.yml", "LICENSE"} {
		assert.Equal(t, read(t, tpl, name), read(t, game, name), name)
	}
	assert.Contains(t, r.out, "Game ready in [mygame]. You can start your local development using:")
	assert.Contains(t, r.out, "➜ cd mygame\n")
	assert.Contains(t, r.out, "➜ go tool libgodot\n")
	assert.Contains(t, r.out, "➜ go run ./cmd/mygame\n")

	test := exec.Command("go", "test", "./...")
	test.Dir = game
	test.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local")
	out, err := test.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestRun_TemplateErrors(t *testing.T) {
	t.Parallel()
	noPrefix := templateRepo(t, func(dir string) {
		path := filepath.Join(dir, "cmd", "game", "main.go")
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, bytes.Replace(b, []byte("EnvPrefix: \"GAME\",\n"), nil, 1), 0o644))
	})
	for _, tt := range []struct {
		name, template, wantErr string
	}{
		{name: "replacement not found", template: noPrefix, wantErr: "cmd/game/main.go"},
		{name: "clone fails", template: filepath.Join(t.TempDir(), "missing"), wantErr: "clone"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			r := runIn(t, dir, tt.template, "", false, "mygame")
			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.errOut, "  ERROR  ")
			assert.Contains(t, r.errOut, tt.wantErr)
			assert.Empty(t, entries(t, dir))
		})
	}
}
