package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// write writes files, slash paths in dir to their text, creating folders.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
	}
}

// readLF reads a file of a game, which git on Windows may check out with CRLF.
func readLF(t *testing.T, path ...string) string {
	t.Helper()
	return strings.ReplaceAll(read(t, path...), "\r\n", "\n")
}

// sharedTemplate makes a template repository with all the shared files.
func sharedTemplate(t *testing.T) string {
	t.Helper()
	return templateRepo(t, func(dir string) {
		write(t, dir, map[string]string{".github/workflows/ci.yml": "ci\n", ".gitattributes": "* text=auto\n",
			".gitignore": "/lib/\n", "staticcheck.conf": "checks = [\"all\"]\n"})
	})
}

// commit commits all files in dir after edit.
func commit(t *testing.T, dir string, edit func(dir string)) {
	t.Helper()
	edit(dir)
	environ := gitEnv(t, "", author...)
	gitOut(t, dir, environ, "add", "-A")
	gitOut(t, dir, environ, "commit", "-q", "-m", "change")
}

// gameRepo makes a git repository of a game on kit version with the shared files of master of tpl, changed by
// edit if it is not nil, and returns its path.
func gameRepo(t *testing.T, tpl, version string, edit func(dir string)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mygame")
	write(t, dir, map[string]string{"go.mod": "module mygame\n\ngo 1.27\n\nrequire " + kitModule + " " + version + "\n"})
	environ := gitEnv(t, "", author...)
	gitOut(t, dir, environ, "init", "-q", "-b", "main")
	gitOut(t, dir, environ, "fetch", "-q", tpl, "master")
	gitOut(t, dir, environ, append([]string{"checkout", "FETCH_HEAD", "--"}, sharedFiles...)...)
	if edit == nil {
		edit = func(string) {}
	}
	commit(t, dir, edit)
	return dir
}

// runUpdate runs gogodot update in dir with the template, a fake go whose latest kit is latest and that fails
// on its first argument fail, and returns the result and the log of the fake go.
func runUpdate(t *testing.T, dir, tpl, latest, fail string, args ...string) (result, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "log")
	var out, errOut bytes.Buffer
	code := run(t.Context(), append([]string{"update"}, args...), env{
		dir: dir, template: tpl, in: strings.NewReader(""), out: &out, errOut: &errOut,
		environ: gitEnv(t, "", append([]string{"FAKE_TOOL_LOG=" + log, "FAKE_TOOL_FAIL=" + fail,
			"FAKE_LATEST=" + latest}, author...)...),
		goTool: os.Args[0],
	})
	b, err := os.ReadFile(log)
	if !errors.Is(err, fs.ErrNotExist) {
		require.NoError(t, err)
	}
	return result{code: code, out: out.String(), errOut: errOut.String()}, string(b)
}

const getLatest = "mygame|get " + kitModule + "@latest|\nmygame|mod tidy|\n"

func TestRun_Commands(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "no command", wantCode: 2, wantErr: "missing command"},
		{name: "unknown command", args: []string{"build"}, wantCode: 2, wantErr: `unknown command "build"`},
		{name: "help", args: []string{"-h"}, wantErr: "update  update kit"},
		{name: "update help", args: []string{"update", "-h"}, wantErr: "Usage: gogodot update [version]"},
		{name: "update unknown flag", args: []string{"update", "-bad"}, wantCode: 2, wantErr: "-bad"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut bytes.Buffer
			code := run(t.Context(), tt.args, env{dir: t.TempDir(), out: &out, errOut: &errOut})
			assert.Equal(t, tt.wantCode, code)
			assert.Contains(t, errOut.String(), tt.wantErr)
		})
	}
}

func TestUpdate_Latest(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	game := gameRepo(t, tpl, "v0.1.0", nil)
	commit(t, tpl, func(dir string) {
		write(t, dir, map[string]string{".golangci.yml": "version: \"2\"\n", ".github/dependabot.yml": "version: 2\n"})
	})
	environ := gitEnv(t, "")

	r, log := runUpdate(t, game, tpl, "v0.3.0", "")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, getLatest, log)
	assert.Equal(t, "kit v0.1.0 -> v0.3.0\nShared files changed:\n  .github/dependabot.yml\n  .golangci.yml\n"+
		"Review the changes and commit them.\n", r.out)
	assert.Contains(t, read(t, game, "go.mod"), kitModule+" v0.3.0")
	assert.Equal(t, "version: \"2\"\n", readLF(t, game, ".golangci.yml"))
	assert.Equal(t, "1", gitOut(t, game, environ, "rev-list", "--count", "--all"))
	assert.Empty(t, gitOut(t, game, environ, "remote"))
	assert.Equal(t, "main", gitOut(t, game, environ, "branch", "-a", "--format=%(refname:short)"))
}

func TestUpdate_Version(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	game := gameRepo(t, tpl, "v0.1.0", nil)

	r, log := runUpdate(t, game, tpl, "v0.3.0", "", "v0.2.0")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, "mygame|get "+kitModule+"@v0.2.0|\nmygame|mod tidy|\n", log)
	assert.Equal(t, "kit v0.1.0 -> v0.2.0\nReview the changes and commit them.\n", r.out)
	assert.Contains(t, read(t, game, "go.mod"), kitModule+" v0.2.0")
}

func TestUpdate_UsageErrors(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	game := gameRepo(t, tpl, "v0.1.0", nil)
	for _, tt := range []struct {
		name, wantErr string
		args          []string
	}{
		{name: "no v", args: []string{"0.2.0"}, wantErr: "use vX.Y.Z"},
		{name: "short", args: []string{"v0.2"}, wantErr: "use vX.Y.Z"},
		{name: "latest", args: []string{"latest"}, wantErr: "use vX.Y.Z"},
		{name: "two versions", args: []string{"v0.2.0", "v0.3.0"}, wantErr: `unexpected argument "v0.3.0"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, log := runUpdate(t, game, tpl, "v0.3.0", "", tt.args...)
			assert.Equal(t, 2, r.code)
			assert.Contains(t, r.errOut, "  ERROR  ")
			assert.Contains(t, r.errOut, tt.wantErr)
			assert.Empty(t, log)
		})
	}
	assert.Contains(t, read(t, game, "go.mod"), kitModule+" v0.1.0")
}

func TestUpdate_NothingToUpdate(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	game := gameRepo(t, tpl, "v0.3.0", nil)

	r, log := runUpdate(t, game, tpl, "v0.3.0", "")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, getLatest, log)
	assert.Equal(t, "Nothing to update: kit v0.3.0 and the shared files are up to date.\n", r.out)
	assert.Empty(t, gitOut(t, game, gitEnv(t, ""), "status", "--porcelain"))
}

func TestUpdate_Errors(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	for _, tt := range []struct {
		name, template, fail, wantErr string
		game                          func(t *testing.T) string
	}{
		{name: "changed file", wantErr: "the game has uncommitted changes", game: func(t *testing.T) string {
			game := gameRepo(t, tpl, "v0.1.0", nil)
			write(t, game, map[string]string{".gitignore": "changed\n"})
			return game
		}},
		{name: "new file", wantErr: "the game has uncommitted changes", game: func(t *testing.T) string {
			game := gameRepo(t, tpl, "v0.1.0", nil)
			write(t, game, map[string]string{"new.txt": "new\n"})
			return game
		}},
		{name: "no go.mod", wantErr: "no game here", game: func(t *testing.T) string { return t.TempDir() }},
		{name: "no kit", wantErr: "no game on kit here", game: func(t *testing.T) string {
			dir := t.TempDir()
			write(t, dir, map[string]string{"go.mod": "module other\n\ngo 1.27\n"})
			return dir
		}},
		{name: "not a git repository", wantErr: "git status", game: func(t *testing.T) string {
			dir := t.TempDir()
			write(t, dir, map[string]string{"go.mod": "module mygame\n\nrequire " + kitModule + " v0.1.0\n"})
			return dir
		}},
		{name: "fetch fails", template: filepath.Join(t.TempDir(), "missing"), wantErr: "fetch the template",
			game: func(t *testing.T) string { return gameRepo(t, tpl, "v0.1.0", nil) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			game := tt.game(t)
			template := tt.template
			if template == "" {
				template = tpl
			}
			// A missing go.mod must stay missing.
			before, _ := os.ReadFile(filepath.Join(game, "go.mod"))

			r, log := runUpdate(t, game, template, "v0.3.0", tt.fail)

			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.errOut, "  ERROR  "+tt.wantErr)
			assert.Empty(t, log)
			after, _ := os.ReadFile(filepath.Join(game, "go.mod"))
			assert.Equal(t, before, after)
		})
	}
}

func TestUpdate_GoFails(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	for _, tt := range []struct{ fail, wantErr string }{{"get", "update kit"}, {"mod", "tidy go.mod"}} {
		t.Run(tt.fail, func(t *testing.T) {
			t.Parallel()
			r, _ := runUpdate(t, gameRepo(t, tpl, "v0.1.0", nil), tpl, "v0.3.0", tt.fail)
			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.errOut, "  ERROR  "+tt.wantErr)
		})
	}
}

func TestUpdate_Removed(t *testing.T) {
	t.Parallel()
	tpl := sharedTemplate(t)
	game := gameRepo(t, tpl, "v0.3.0", func(dir string) {
		write(t, dir, map[string]string{".github/workflows/deploy.yml": "deploy\n", ".gitattributes": "* text=auto eol=lf\n"})
	})
	commit(t, tpl, func(dir string) {
		require.NoError(t, os.Remove(filepath.Join(dir, "staticcheck.conf")))
		require.NoError(t, os.Remove(filepath.Join(dir, ".gitattributes")))
	})

	r, _ := runUpdate(t, game, tpl, "v0.3.0", "")

	require.Equal(t, 0, r.code, r.errOut)
	assert.Equal(t, "kit v0.3.0 -> v0.3.0\nShared files changed:\n  staticcheck.conf\n"+
		"Review the changes and commit them.\n", r.out)
	assert.NoFileExists(t, filepath.Join(game, "staticcheck.conf"))
	assert.Equal(t, "* text=auto eol=lf\n", readLF(t, game, ".gitattributes"))
	assert.Equal(t, "deploy\n", readLF(t, game, ".github", "workflows", "deploy.yml"))
	assert.Equal(t, "ci\n", readLF(t, game, ".github", "workflows", "ci.yml"))
}
