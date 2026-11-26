package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
	code := run(t.Context(), append([]string{"new"}, args...), env{
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
			mainGo := read(t, dir, tt.wantGame, "cmd", tt.wantGame, "main.go")
			assert.Contains(t, mainGo, `EnvPrefix: "`+tt.wantPrefix+`"`)
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
	mainGo := read(t, game, "cmd", "mygame", "main.go")
	assert.Contains(t, mainGo, `"mygame/internal/app"`)
	assert.Contains(t, mainGo, `Name:      "mygame"`)
	assert.Contains(t, mainGo, `EnvPrefix: "MYGAME"`)
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

// gitEnv returns the environment of the command with an isolated git config: global config from config,
// no system config and no author from the parent environment.
func gitEnv(t *testing.T, config string, extra ...string) []string {
	t.Helper()
	home := t.TempDir()
	global := filepath.Join(home, "gitconfig")
	require.NoError(t, os.WriteFile(global, []byte(config), 0o644))
	environ := make([]string, 0, len(os.Environ())+len(extra)+4)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "GIT_") && !strings.HasPrefix(strings.ToUpper(kv), "EMAIL=") {
			environ = append(environ, kv)
		}
	}
	environ = append(environ, "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+global,
		"GIT_CONFIG_NOSYSTEM=1")
	return append(environ, extra...)
}

// author is the commit author for gitEnv.
var author = []string{"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
	"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com"}

// runGit runs the command in dir with the template and the environment given.
func runGit(t *testing.T, dir, template string, environ []string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(t.Context(), append([]string{"new"}, args...), env{
		dir: dir, template: template, in: strings.NewReader(""), out: &out, errOut: &errOut, environ: environ,
	})
	return result{code: code, out: out.String(), errOut: errOut.String()}
}

// gitOut runs git in dir with environ and returns its trimmed output.
func gitOut(t *testing.T, dir string, environ []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = environ
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestRun_Git(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	for _, tt := range []struct {
		name, config, wantBranch string
	}{
		{name: "default branch unset", wantBranch: "main"},
		{name: "default branch set", config: "[init]\n\tdefaultBranch = trunk\n", wantBranch: "trunk"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			environ := gitEnv(t, tt.config, author...)

			r := runGit(t, dir, tpl, environ, "mygame", "-git")

			require.Equal(t, 0, r.code, r.errOut)
			game := filepath.Join(dir, "mygame")
			assert.Equal(t, tt.wantBranch, gitOut(t, game, environ, "branch", "--show-current"))
			assert.Equal(t, "1", gitOut(t, game, environ, "rev-list", "--count", "HEAD"))
			assert.Equal(t, "Set up a fresh game", gitOut(t, game, environ, "log", "-1", "--format=%s"))
			assert.Empty(t, gitOut(t, game, environ, "status", "--porcelain"))
			assert.Empty(t, gitOut(t, game, environ, "remote"))
		})
	}
}

func TestRun_NoGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := runGit(t, dir, templateRepo(t, nil), gitEnv(t, "", author...), "mygame")

	require.Equal(t, 0, r.code, r.errOut)
	assert.NoDirExists(t, filepath.Join(dir, "mygame", ".git"))
}

func TestRun_GitNoAuthor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// useConfigOnly stops git from guessing the author from the user and host names.
	environ := gitEnv(t, "[user]\n\tuseConfigOnly = true\n")

	r := runGit(t, dir, templateRepo(t, nil), environ, "-git", "mygame")

	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.errOut, "  ERROR  make the first commit")
	assert.Equal(t, []string{"mygame"}, entries(t, dir))
	assert.FileExists(t, filepath.Join(dir, "mygame", "go.mod"))
}

// TestMain makes the test binary a fake gh, LibGodot build or go when FAKE_TOOL_LOG is set: it appends its
// folder and arguments to that file and fails when its first argument is FAKE_TOOL_FAIL. As go get it sets the
// kit version in go.mod, FAKE_LATEST for latest.
func TestMain(m *testing.M) {
	if log := os.Getenv("FAKE_TOOL_LOG"); log != "" {
		wd, _ := os.Getwd()
		f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%s|%s|%s\n", filepath.Base(wd), strings.Join(os.Args[1:], " "),
				os.Getenv("GIT_TERMINAL_PROMPT"))
			_ = f.Close()
		}
		if len(os.Args) > 1 && os.Args[1] == os.Getenv("FAKE_TOOL_FAIL") {
			os.Exit(1)
		}
		if len(os.Args) > 2 && os.Args[1] == "get" {
			fakeGet(os.Args[2])
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeGet is go get for TestMain: it sets the kit version of arg, module@version, in go.mod.
func fakeGet(arg string) {
	_, v, _ := strings.Cut(arg, "@")
	if v == "latest" {
		v = os.Getenv("FAKE_LATEST")
	}
	b, err := os.ReadFile("go.mod")
	if err == nil {
		b = regexp.MustCompile(regexp.QuoteMeta(kitModule)+` v\S+`).ReplaceAll(b, []byte(kitModule+" "+v))
		err = os.WriteFile("go.mod", b, 0o644)
	}
	if err != nil {
		os.Exit(1)
	}
}

// fake runs the command in dir with a fake gh and LibGodot build that log to the returned file.
func fake(t *testing.T, dir, template, input string, interactive bool, fail string, args ...string) (result, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "log")
	var out, errOut bytes.Buffer
	code := run(t.Context(), append([]string{"new"}, args...), env{
		dir: dir, template: template, in: strings.NewReader(input), out: &out, errOut: &errOut,
		interactive: interactive,
		environ:     gitEnv(t, "", append([]string{"FAKE_TOOL_LOG=" + log, "FAKE_TOOL_FAIL=" + fail}, author...)...),
		gh:          os.Args[0], libgodot: []string{os.Args[0], "libgodot"},
	})
	b, err := os.ReadFile(log)
	if !errors.Is(err, fs.ErrNotExist) {
		require.NoError(t, err)
	}
	return result{code: code, out: out.String(), errOut: errOut.String()}, string(b)
}

func TestRun_Force(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	for _, tt := range []struct {
		name, template string
		args           []string
		wantCode       int
	}{
		{name: "-force", template: tpl, args: []string{"mygame", "--force"}},
		{name: "-f", template: tpl, args: []string{"-f", "mygame"}},
		{name: "clone fails", template: filepath.Join(t.TempDir(), "missing"), args: []string{"-f", "mygame"},
			wantCode: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			marker := filepath.Join(dir, "mygame", "keep.txt")
			require.NoError(t, os.Mkdir(filepath.Dir(marker), 0o755))
			require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o644))

			r := runIn(t, dir, tt.template, "", false, tt.args...)

			require.Equal(t, tt.wantCode, r.code, r.errOut)
			assert.Equal(t, []string{"mygame"}, entries(t, dir))
			if tt.wantCode == 0 {
				assert.NoFileExists(t, marker)
				assert.FileExists(t, filepath.Join(dir, "mygame", "go.mod"))
			} else {
				assert.Equal(t, []string{"keep.txt"}, entries(t, filepath.Join(dir, "mygame")))
			}
		})
	}
}

func TestRun_Here(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	t.Run("empty folder", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "space-race")
		require.NoError(t, os.Mkdir(dir, 0o755))

		r := runIn(t, dir, tpl, "", false, ".")

		require.Equal(t, 0, r.code, r.errOut)
		assert.Contains(t, read(t, dir, "cmd", "space-race", "main.go"), `EnvPrefix: "SPACE_RACE"`)
		assert.Regexp(t, `^module space-race\s`, read(t, dir, "go.mod"))
		assert.NotContains(t, strings.Join(entries(t, dir), " "), ".space-race-")
		assert.Contains(t, r.out, "Game ready in [space-race]")
		assert.NotContains(t, r.out, "➜ cd")
	})
	for _, tt := range []struct {
		name, folder, file, wantErr string
		args                        []string
		wantCode                    int
	}{
		{name: "not empty", folder: "space-race", file: "keep.txt", wantErr: "not empty", wantCode: 1},
		{name: "invalid folder name", folder: "Space_Race", wantErr: "a-z", wantCode: 2},
		{name: "-force", folder: "space-race", args: []string{"--force"}, wantErr: "-force", wantCode: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), tt.folder)
			require.NoError(t, os.Mkdir(dir, 0o755))
			if tt.file != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, tt.file), nil, 0o644))
			}

			r := runIn(t, dir, tpl, "", false, append([]string{"."}, tt.args...)...)

			assert.Equal(t, tt.wantCode, r.code)
			assert.Contains(t, r.errOut, tt.wantErr)
			if tt.file == "" {
				assert.Empty(t, entries(t, dir))
			} else {
				assert.Equal(t, []string{tt.file}, entries(t, dir))
			}
		})
	}
}

func TestRun_Dev(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	gitOut(t, tpl, nil, "switch", "-q", "-c", "develop")
	require.NoError(t, os.WriteFile(filepath.Join(tpl, "NEW.md"), []byte("new"), 0o644))
	gitOut(t, tpl, nil, "add", "NEW.md")
	gitOut(t, tpl, nil, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "new")
	gitOut(t, tpl, nil, "switch", "-q", "master")
	dir := t.TempDir()

	require.Equal(t, 0, runIn(t, dir, tpl, "", false, "stable").code)
	r := runIn(t, dir, tpl, "", false, "-dev", "fresh")

	require.Equal(t, 0, r.code, r.errOut)
	assert.NoFileExists(t, filepath.Join(dir, "stable", "NEW.md"))
	assert.FileExists(t, filepath.Join(dir, "fresh", "NEW.md"))
}

func TestRun_Branch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	environ := gitEnv(t, "[init]\n\tdefaultBranch = main\n", author...)

	r := runGit(t, dir, templateRepo(t, nil), environ, "mygame", "-git", "-branch", "trunk")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, "trunk", gitOut(t, filepath.Join(dir, "mygame"), environ, "branch", "--show-current"))
}

func TestRun_GitHub(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	for _, tt := range []struct {
		name, fail, wantLog, wantOut string
		args                         []string
		wantCode                     int
	}{
		{name: "private", args: []string{"mygame", "-github"},
			wantLog: "mygame|auth status|\nmygame|repo create mygame --source=. --push --private|0\n"},
		{name: "organization and flags", args: []string{"mygame", "--github=--public", "--organization=acme"},
			wantLog: "mygame|auth status|\nmygame|repo create acme/mygame --source=. --push --public|0\n"},
		{name: "no login", fail: "auth", args: []string{"mygame", "-github"},
			wantLog: "mygame|auth status|\n", wantOut: "  WARN  Make sure the \"gh\" CLI tool is installed"},
		{name: "create fails", fail: "repo", args: []string{"mygame", "-github"}, wantCode: 1,
			wantLog: "mygame|auth status|\nmygame|repo create mygame --source=. --push --private|0\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			r, log := fake(t, dir, tpl, "", false, tt.fail, tt.args...)

			require.Equal(t, tt.wantCode, r.code, r.errOut)
			assert.Equal(t, tt.wantLog, log)
			assert.Contains(t, r.out, tt.wantOut)
			assert.DirExists(t, filepath.Join(dir, "mygame", ".git"))
			if tt.wantCode != 0 {
				assert.Contains(t, r.errOut, "  ERROR  create the GitHub repository")
			}
		})
	}
}

func TestRun_LibGodot(t *testing.T) {
	t.Parallel()
	tpl := templateRepo(t, nil)
	const question = "Would you like to build LibGodot now?"
	for _, tt := range []struct {
		name, input, fail string
		args              []string
		interactive       bool
		wantBuild         bool
		wantCode          int
	}{
		{name: "flag", args: []string{"mygame", "-libgodot"}, wantBuild: true},
		{name: "yes by default", args: []string{"mygame"}, input: "\n", interactive: true, wantBuild: true},
		{name: "no", args: []string{"mygame"}, input: "no\n", interactive: true},
		{name: "end of input", args: []string{"mygame"}, interactive: true},
		{name: "-n", args: []string{"mygame", "-n"}, input: "yes\n", interactive: true},
		{name: "not a terminal", args: []string{"mygame"}, input: "yes\n"},
		{name: "build fails", args: []string{"mygame", "-libgodot"}, fail: "libgodot", wantBuild: true, wantCode: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, log := fake(t, t.TempDir(), tpl, tt.input, tt.interactive, tt.fail, tt.args...)

			require.Equal(t, tt.wantCode, r.code, r.errOut)
			assert.Equal(t, tt.interactive && !slices.Contains(tt.args, "-n"), strings.Contains(r.out, question))
			assert.Equal(t, tt.interactive && !slices.Contains(tt.args, "-n"), strings.Contains(r.out, logo))
			if !tt.wantBuild {
				assert.Empty(t, log)
				assert.Contains(t, r.out, "➜ go tool libgodot\n")
				return
			}
			assert.Equal(t, "mygame|libgodot|\n", log)
			if tt.wantCode == 0 {
				assert.NotContains(t, r.out, "go tool libgodot")
			} else {
				assert.Contains(t, r.errOut, "  ERROR  build LibGodot")
			}
		})
	}
}
