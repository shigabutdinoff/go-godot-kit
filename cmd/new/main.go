// New creates a Go game on go-godot-kit from the template github.com/shigabutdinoff/go-godot-tpl, the way
// laravel new creates a Laravel application. It runs without installation:
//
//	go run github.com/shigabutdinoff/go-godot-kit/cmd/new@<version> [-n] [name]
//
// The name is the game folder, the Go module path and, in upper case with "_" for "-", the prefix of the
// game's environment variables (my-game: module my-game, MY_GAME_…). It holds a-z, 0-9 and single hyphens,
// starts with a letter and does not end with a hyphen; go and toolchain are reserved. Without a name the
// command asks for it in a terminal; with -n or without a terminal it fails instead.
//
// The command clones master of the template with git, without history, next to the game folder, renames
// cmd/game, the module, Name, EnvPrefix and the Godot project name, and moves the result into the game folder.
// README, LICENSE and the shared config files stay as in the template. It needs Go and git.
//
// Errors go to stderr as one line. Exit code 0 means the game is created or -h was given, 1 is an error,
// 2 is an invalid name or arguments.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const (
	templateURL    = "https://github.com/shigabutdinoff/go-godot-tpl.git"
	templateModule = "github.com/shigabutdinoff/go-godot-tpl"
	nameRule       = "use lowercase letters a-z, digits 0-9 and single hyphens, start with a letter and do not " +
		"end with a hyphen; go and toolchain are reserved"
)

// errMissingName is the text of laravel new for a missing name.
const errMissingName message = `Not enough arguments (missing: "name").`

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// message is an error shown as written.
type message string

func (m message) Error() string { return string(m) }

// env is what run works with: the folder the game is created in, the template repository and the terminal.
type env struct {
	dir, template string
	in            io.Reader
	out, errOut   io.Writer
	interactive   bool
}

// usageError is an invalid name or invalid arguments, exit code 2.
type usageError struct{ error }

// main creates the game in the current folder until Ctrl+C, SIGTERM or SIGQUIT and exits with the code of run.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	// After the first signal the default handlers are back, so a second one kills the command while git stops.
	go func() { <-ctx.Done(); stop() }()
	stat, err := os.Stdin.Stat()
	code := run(ctx, os.Args[1:], env{
		dir: ".", template: templateURL, in: os.Stdin, out: os.Stdout, errOut: os.Stderr,
		interactive: err == nil && stat.Mode()&os.ModeCharDevice != 0,
	})
	stop()
	exit(code)
}

// exit ends the process; os.Exit is not allowed in main.main.
func exit(code int) { os.Exit(code) }

// run creates the game for args and returns the exit code; errors go to e.errOut.
func run(ctx context.Context, args []string, e env) int {
	err := create(ctx, args, e)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	_, _ = fmt.Fprintf(e.errOut, "  ERROR  %s\n", err)
	if errors.As(err, new(usageError)) {
		return 2
	}
	return 1
}

// create gets the name, makes the game from the template and prints the next steps.
func create(ctx context.Context, args []string, e env) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(e.errOut)
	noInteraction := flags.Bool("n", false, "do not ask for the name")
	var names []string
	// Flags may follow the name, as in laravel new.
	for {
		if err := flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return err
			}
			return usageError{err}
		}
		if flags.NArg() == 0 {
			break
		}
		names, args = append(names, flags.Arg(0)), flags.Args()[1:]
	}
	var name string
	var err error
	switch {
	case len(names) > 1:
		return usageError{fmt.Errorf("unexpected argument %q", names[1])}
	case len(names) == 1:
		name, err = names[0], checkName(e.dir, names[0])
	case e.interactive && !*noInteraction:
		name, err = ask(e)
	default:
		err = errMissingName
	}
	if err != nil {
		return err
	}
	if err = fromTemplate(ctx, e, name); err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.out, "\n  Game ready in [%[1]s]. You can start your local development using:\n\n"+
		" ➜ cd %[1]s\n ➜ go tool libgodot\n ➜ go run ./cmd/%[1]s\n\n", name)
	return err
}

// validName reports whether name can be a game folder, a Go module path and an env prefix.
func validName(name string) bool {
	return nameRE.MatchString(name) && name != "go" && name != "toolchain"
}

// checkName returns a usageError for an invalid name and an error if the game folder already exists.
func checkName(dir, name string) error {
	if !validName(name) {
		return usageError{fmt.Errorf("invalid name %q: %s", name, nameRule)}
	}
	switch _, err := os.Lstat(filepath.Join(dir, name)); {
	case err == nil:
		return fmt.Errorf("game %q already exists", name)
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	return nil
}

// ask asks for the name until it is valid and free; the end of input means no name.
func ask(e env) (string, error) {
	lines := bufio.NewScanner(e.in)
	for {
		_, _ = fmt.Fprint(e.out, "What is the name of your game?\nE.g. my-game\n> ")
		if !lines.Scan() {
			_, _ = fmt.Fprintln(e.out)
			return "", errors.Join(lines.Err(), errMissingName)
		}
		name := strings.TrimSpace(lines.Text())
		err := checkName(e.dir, name)
		if err == nil {
			return name, nil
		}
		_, _ = fmt.Fprintf(e.errOut, "  ERROR  %s\n", err)
	}
}

// fromTemplate clones the template into a temporary folder next to the game folder, renames the game in it
// and moves it into place. On error the temporary folder is removed and the game folder does not appear.
func fromTemplate(ctx context.Context, e env, name string) error {
	tmp, err := os.MkdirTemp(e.dir, "."+name+"-")
	if err != nil {
		return fmt.Errorf("create temporary folder: %w", err)
	}
	// After the move tmp is gone; a failed cleanup must not hide the error.
	defer func() { _ = os.RemoveAll(tmp) }()
	if err = os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err = git(ctx, e, e.dir, "clone", "--quiet", "--depth", "1", "--branch", "master", e.template, tmp); err != nil {
		return fmt.Errorf("clone template: %w", err)
	}
	if err = os.RemoveAll(filepath.Join(tmp, ".git")); err != nil {
		return fmt.Errorf("remove template history: %w", err)
	}
	if err = rename(tmp, name); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(tmp, "cmd", "game"), filepath.Join(tmp, "cmd", name)); err != nil {
		return fmt.Errorf("rename cmd/game: %w", err)
	}
	if err = os.Rename(tmp, filepath.Join(e.dir, name)); err != nil {
		return fmt.Errorf("move the game into place: %w", err)
	}
	return nil
}

// git runs git in dir with its output on the terminal.
func git(ctx context.Context, e env, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, e.out, e.errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git: %w", err)
	}
	return nil
}

// replacement replaces old with new in file, a slash path in the template or "*.go" for every Go file.
type replacement struct{ file, old, new string }

// rename applies the replacements for name in the template in dir. Each must match at least once, or the
// template does not match this version of the command.
func rename(dir, name string) error {
	prefix := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	for _, r := range []replacement{
		{"go.mod", "module " + templateModule, "module " + name},
		{"*.go", `"` + templateModule + "/", `"` + name + "/"},
		{"cmd/game/main.go", `Name:      "game"`, `Name:      "` + name + `"`},
		{"cmd/game/main.go", `EnvPrefix: "GAME"`, `EnvPrefix: "` + prefix + `"`},
		{"cmd/game/main_test.go", `"game", g.Name`, `"` + name + `", g.Name`},
		{"cmd/game/main_test.go", `"GAME", g.EnvPrefix`, `"` + prefix + `", g.EnvPrefix`},
		{"cmd/game/doc.go", "cmd/game", "cmd/" + name},
		{"cmd/game/doc.go", "GAME_", prefix + "_"},
		{"cmd/game/graphics/project.godot", `config/name="Game"`, `config/name="` + name + `"`},
	} {
		n, err := r.apply(dir)
		if err == nil && n == 0 {
			err = fmt.Errorf("%q not found", r.old)
		}
		if err != nil {
			return fmt.Errorf("the template does not match this version of the command: %s: %w", r.file, err)
		}
	}
	return nil
}

// apply makes the replacement in dir and returns the number of matches.
func (r replacement) apply(dir string) (int, error) {
	files := []string{filepath.Join(dir, filepath.FromSlash(r.file))}
	if r.file == "*.go" {
		files = nil
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
				files = append(files, path)
			}
			return err
		})
		if err != nil {
			return 0, err
		}
	}
	total := 0
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		if n := strings.Count(string(b), r.old); n > 0 {
			total += n
			if err = os.WriteFile(path, []byte(strings.ReplaceAll(string(b), r.old, r.new)), 0o644); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}
