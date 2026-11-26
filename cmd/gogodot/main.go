// Gogodot creates and updates Go games on go-godot-kit made from the template
// github.com/shigabutdinoff/go-godot-tpl. Install it once:
//
//	go install github.com/shigabutdinoff/go-godot-kit/cmd/gogodot@latest
//
// gogodot new [flags] [name] creates a game. The name is the game folder, the Go module path and, in upper case
// with "_" for "-", the prefix of the game's environment variables (my-game: module my-game, MY_GAME_…). It
// holds a-z, 0-9 and single hyphens, starts with a letter and does not end with a hyphen; go and toolchain are
// reserved. Without a name the command asks for it in a terminal; with -n or without a terminal it fails
// instead. The name . creates the game in the current folder, which must be empty, and takes the name of the
// folder.
//
// new clones master of the template with git, or develop with -dev, without history, next to the game folder,
// renames cmd/game, the module, Name, EnvPrefix and the Godot project name, and moves the result into the game
// folder. README, LICENSE and the shared config files stay as in the template. It needs Go and git. An existing
// game folder is an error; with -force (-f) it is removed once the new game is ready.
//
// Without -git the game has no git repository. With -git new creates one on the branch from -branch, git config
// init.defaultBranch or main, and commits all files of the game once. -github also creates the repository,
// then a private repository on GitHub with the GitHub CLI, in -organization if given, and pushes the game
// there; -github="--public" passes other flags of gh repo create. Without gh or its login new warns and skips
// this step. In a terminal new prints its name in large letters and asks whether to build LibGodot for the game;
// -libgodot builds it without asking.
//
// gogodot update [version] runs in the game folder: it updates kit to version, vX.Y.Z, or to its latest release
// with go get and go mod tidy, and the shared files .github, .golangci.yml, .gitattributes, .gitignore and
// staticcheck.conf from master of the template, fetched by its address with no remote left behind. A shared file
// the template no longer has is removed if the game has it as some version of the template had it. It prints the
// kit versions and the shared files changed and commits nothing. A game with uncommitted changes or a folder
// without a game on kit is an error.
//
// Errors go to stderr as one line. Exit code 0 means success or -h, 1 is an error, 2 is an invalid name, version
// or arguments.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	templateURL  = "https://github.com/shigabutdinoff/go-godot-tpl.git"
	gitWaitDelay = 10 * time.Second
	usage        = "Usage: gogodot <command> [arguments]\n\nCommands:\n" +
		"  new     create a game from the template\n" +
		"  update  update kit and the shared files of the template in the game\n\n" +
		"Run gogodot <command> -h for the flags of a command.\n"
)

// message is an error shown as written.
type message string

func (m message) Error() string { return string(m) }

// usageError is an invalid name, version or arguments, exit code 2.
type usageError struct{ error }

// env is what run works with: the current folder, the template repository, the terminal and the environment of
// git and go, nil for the environment of the command.
type env struct {
	dir, template string
	in            io.Reader
	out, errOut   io.Writer
	interactive   bool
	environ       []string
	// gh is the GitHub CLI, libgodot the command that builds LibGodot in the game folder, goTool the go command.
	gh       string
	libgodot []string
	goTool   string
}

// main runs the command in the current folder until Ctrl+C, SIGTERM or SIGQUIT and exits with the code of run.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	// After the first signal the default handlers are back, so a second one kills the command while git stops.
	go func() { <-ctx.Done(); stop() }()
	stat, err := os.Stdin.Stat()
	code := run(ctx, os.Args[1:], env{
		dir: ".", template: templateURL, in: os.Stdin, out: os.Stdout, errOut: os.Stderr,
		interactive: err == nil && stat.Mode()&os.ModeCharDevice != 0,
		gh:          "gh", libgodot: []string{"go", "tool", "libgodot"}, goTool: "go",
	})
	stop()
	exit(code)
}

// exit ends the process; os.Exit is not allowed in main.main.
func exit(code int) { os.Exit(code) }

// run runs the command in args and returns the exit code; errors go to e.errOut.
func run(ctx context.Context, args []string, e env) int {
	var err error
	switch {
	case len(args) == 0:
		_, _ = fmt.Fprint(e.errOut, usage)
		err = usageError{errors.New("missing command")}
	case args[0] == "new":
		err = create(ctx, args[1:], e)
	case args[0] == "update":
		err = update(ctx, args[1:], e)
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help":
		_, _ = fmt.Fprint(e.errOut, usage)
	default:
		_, _ = fmt.Fprint(e.errOut, usage)
		err = usageError{fmt.Errorf("unknown command %q", args[0])}
	}
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	_, _ = fmt.Fprintf(e.errOut, "  ERROR  %s\n", err)
	if errors.As(err, new(usageError)) {
		return 2
	}
	return 1
}

// command returns the program with args in dir and the environment of e. Cancelling ctx interrupts it, as
// Ctrl+C would; after gitWaitDelay, or at once where an interrupt cannot be sent (Windows), it is killed.
func command(ctx context.Context, e env, dir, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir, cmd.Env = dir, e.environ
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	// A killed git may leave helpers such as git-remote-https holding the output pipes.
	cmd.WaitDelay = gitWaitDelay
	return cmd
}

// runShown runs cmd with its output on the terminal.
func runShown(e env, cmd *exec.Cmd) error {
	cmd.Stdout, cmd.Stderr = e.out, e.errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(cmd.Args[0]), err)
	}
	return nil
}

// git runs git in dir with its output on the terminal.
func git(ctx context.Context, e env, dir string, args ...string) error {
	return runShown(e, command(ctx, e, dir, "git", args...))
}
