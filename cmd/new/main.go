// New creates a Go game on go-godot-kit from the template github.com/shigabutdinoff/go-godot-tpl. It runs
// without installation:
//
//	go run github.com/shigabutdinoff/go-godot-kit/cmd/new@<version> [flags] [name]
//
// The name is the game folder, the Go module path and, in upper case with "_" for "-", the prefix of the
// game's environment variables (my-game: module my-game, MY_GAME_…). It holds a-z, 0-9 and single hyphens,
// starts with a letter and does not end with a hyphen; go and toolchain are reserved. Without a name the
// command asks for it in a terminal; with -n or without a terminal it fails instead. The name . creates the
// game in the current folder, which must be empty, and takes the name of the folder.
//
// The command clones master of the template with git, or develop with -dev, without history, next to the game
// folder, renames cmd/game, the module, Name, EnvPrefix and the Godot project name, and moves the result into
// the game folder. README, LICENSE and the shared config files stay as in the template. It needs Go and git.
// An existing game folder is an error; with -force (-f) it is removed once the new game is ready.
//
// Without -git the game has no git repository. With -git the command creates one on the branch from -branch,
// git config init.defaultBranch or main, commits all files of the game once and adds the template as the
// remote template, so shared files can be updated from it:
//
//	git fetch template && git checkout template/master -- .github .golangci.yml .gitattributes .gitignore staticcheck.conf
//
// -github also creates the repository, then a private repository on GitHub with the GitHub CLI, in
// -organization if given, and pushes the game there; -github="--public" passes other flags of gh repo create.
// Without gh or its login the command warns and skips this step.
//
// In a terminal the command prints its name in large letters and asks whether to build LibGodot for the game; -libgodot builds it without asking.
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
	"time"
)

const (
	templateURL    = "https://github.com/shigabutdinoff/go-godot-tpl.git"
	templateModule = "github.com/shigabutdinoff/go-godot-tpl"
	gitWaitDelay   = 10 * time.Second
	nameRule       = "use lowercase letters a-z, digits 0-9 and single hyphens, start with a letter and do not " +
		"end with a hyphen; go and toolchain are reserved"
)

// logo is printed in a terminal before the questions.
const logo = "\n\x1b[36m" +
	"                                     _       _\n" +
	"    __ _  ___         __ _  ___   __| | ___ | |_\n" +
	"   / _` |/ _ \\ _____ / _` |/ _ \\ / _` |/ _ \\| __|\n" +
	"  | (_| | (_) |_____| (_| | (_) | (_| | (_) | |_\n" +
	"   \\__, |\\___/       \\__, |\\___/ \\__,_|\\___/ \\__|\n" +
	"   |___/             |___/\n" +
	"\x1b[0m\n"

// errMissingName is the error for a missing name.
const errMissingName message = `Not enough arguments (missing: "name").`

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// message is an error shown as written.
type message string

func (m message) Error() string { return string(m) }

// env is what run works with: the folder the game is created in, the template repository, the terminal and
// the environment of git, nil for the environment of the command.
type env struct {
	dir, template string
	in            io.Reader
	out, errOut   io.Writer
	interactive   bool
	environ       []string
	// gh is the GitHub CLI and libgodot the command that builds LibGodot in the game folder.
	gh       string
	libgodot []string
}

// options are the flags of the command.
type options struct {
	noInteraction, force, git, dev, libgodot bool
	branch, organization                     string
	github                                   githubFlag
}

// githubFlag is -github: given at all, and the flags of gh repo create instead of --private.
type githubFlag struct {
	set   bool
	flags string
}

func (g *githubFlag) String() string { return g.flags }

// Set takes "true" for -github without a value.
func (g *githubFlag) Set(v string) error {
	g.set = true
	if v != "true" {
		g.flags = v
	}
	return nil
}

// IsBoolFlag lets -github go without a value.
func (g *githubFlag) IsBoolFlag() bool { return true }

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
		gh:          "gh", libgodot: []string{"go", "tool", "libgodot"},
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

// create gets the name, makes the game from the template, sets up git, GitHub and LibGodot and prints the next
// steps.
func create(ctx context.Context, args []string, e env) error {
	o, names, err := parse(args, e.errOut)
	if err != nil {
		return err
	}
	if e.interactive && !o.noInteraction {
		_, _ = fmt.Fprint(e.out, logo)
	}
	lines := bufio.NewScanner(e.in)
	name, game, err := target(e, o, names, lines)
	if err != nil {
		return err
	}
	if err = fromTemplate(ctx, e, o, name, game); err != nil {
		return err
	}
	if o.git || o.github.set {
		if err = initRepo(ctx, e, game, o.branch); err != nil {
			return err
		}
	}
	if o.github.set {
		if err = pushToGitHub(ctx, e, o, name, game); err != nil {
			return err
		}
	}
	built, err := buildEngine(ctx, e, o, lines, game)
	if err != nil {
		return err
	}
	steps := ""
	if game != e.dir {
		steps += " ➜ cd " + name + "\n"
	}
	if !built {
		steps += " ➜ go tool libgodot\n"
	}
	_, err = fmt.Fprintf(e.out, "\n  Game ready in [%s]. You can start your local development using:\n\n%s ➜ go run ./cmd/%[1]s\n\n",
		name, steps)
	return err
}

// parse returns the flags and the arguments; flags may follow the name.
func parse(args []string, errOut io.Writer) (options, []string, error) {
	var o options
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.BoolVar(&o.noInteraction, "n", false, "do not ask questions")
	flags.BoolVar(&o.force, "force", false, "replace the game folder if it exists")
	flags.BoolVar(&o.force, "f", false, "shorthand for -force")
	flags.BoolVar(&o.dev, "dev", false, "create the game from develop of the template")
	flags.BoolVar(&o.git, "git", false, "create a git repository with one commit and the template as remote template")
	flags.StringVar(&o.branch, "branch", "", "the branch of the git repository (default init.defaultBranch or main)")
	flags.Var(&o.github, "github", "create a private GitHub repository with gh and push the game; "+
		"-github=<flags> passes other flags of gh repo create")
	flags.StringVar(&o.organization, "organization", "", "the GitHub organization of the repository")
	flags.BoolVar(&o.libgodot, "libgodot", false, "build LibGodot without asking")
	var names []string
	for {
		if err := flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return o, nil, err
			}
			return o, nil, usageError{err}
		}
		if flags.NArg() == 0 {
			return o, names, nil
		}
		names, args = append(names, flags.Arg(0)), flags.Args()[1:]
	}
}

// target returns the name and the folder of the game: the name given, the current folder for ".", or the name
// asked for in a terminal.
func target(e env, o options, names []string, lines *bufio.Scanner) (string, string, error) {
	var name string
	var err error
	switch {
	case len(names) > 1:
		return "", "", usageError{fmt.Errorf("unexpected argument %q", names[1])}
	case len(names) == 1 && names[0] == ".":
		return here(e, o)
	case len(names) == 1:
		name = names[0]
		err = checkName(e.dir, name, o.force)
	case e.interactive && !o.noInteraction:
		name, err = ask(e, lines, o.force)
	default:
		err = errMissingName
	}
	return name, filepath.Join(e.dir, name), err
}

// here returns the name and the folder for a game in the current folder, which must be empty.
func here(e env, o options) (string, string, error) {
	if o.force {
		return "", "", errors.New("cannot use -force with the current folder")
	}
	abs, err := filepath.Abs(e.dir)
	if err != nil {
		return "", "", err
	}
	name := filepath.Base(abs)
	if !validName(name) {
		return "", "", usageError{fmt.Errorf("invalid name %q of the current folder: %s", name, nameRule)}
	}
	list, err := os.ReadDir(e.dir)
	if err != nil {
		return "", "", err
	}
	if len(list) > 0 {
		return "", "", errors.New("the current folder is not empty")
	}
	return name, e.dir, nil
}

// validName reports whether name can be a game folder, a Go module path and an env prefix.
func validName(name string) bool {
	return nameRE.MatchString(name) && name != "go" && name != "toolchain"
}

// checkName returns a usageError for an invalid name and, without force, an error if the game folder already
// exists.
func checkName(dir, name string, force bool) error {
	if !validName(name) {
		return usageError{fmt.Errorf("invalid name %q: %s", name, nameRule)}
	}
	if force {
		return nil
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
func ask(e env, lines *bufio.Scanner, force bool) (string, error) {
	for {
		_, _ = fmt.Fprint(e.out, "What is the name of your game?\nE.g. my-game\n> ")
		if !lines.Scan() {
			_, _ = fmt.Fprintln(e.out)
			return "", errors.Join(lines.Err(), errMissingName)
		}
		name := strings.TrimSpace(lines.Text())
		err := checkName(e.dir, name, force)
		if err == nil {
			return name, nil
		}
		_, _ = fmt.Fprintf(e.errOut, "  ERROR  %s\n", err)
	}
}

// fromTemplate clones the template into a temporary folder next to the game folder, renames the game in it
// and moves it into place: into game, replacing it with -force, or into the current folder. On error the
// temporary folder is removed and the game folder is left as it was.
func fromTemplate(ctx context.Context, e env, o options, name, game string) error {
	tmp, err := os.MkdirTemp(e.dir, "."+name+"-")
	if err != nil {
		return fmt.Errorf("create temporary folder: %w", err)
	}
	// After the move tmp is gone; a failed cleanup must not hide the error.
	defer func() { _ = os.RemoveAll(tmp) }()
	if err = os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	branch := "master"
	if o.dev {
		branch = "develop"
	}
	if err = git(ctx, e, e.dir, "clone", "--quiet", "--depth", "1", "--branch", branch, e.template, tmp); err != nil {
		return fmt.Errorf("clone template: %w", err)
	}
	if err = os.RemoveAll(filepath.Join(tmp, ".git")); err != nil {
		return fmt.Errorf("remove template history: %w", err)
	}
	if err = replaceNames(tmp, name); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(tmp, "cmd", "game"), filepath.Join(tmp, "cmd", name)); err != nil {
		return fmt.Errorf("rename cmd/game: %w", err)
	}
	if game == e.dir {
		return moveEntries(tmp, game)
	}
	if o.force {
		if err = os.RemoveAll(game); err != nil {
			return fmt.Errorf("remove the existing game: %w", err)
		}
	}
	if err = os.Rename(tmp, game); err != nil {
		return fmt.Errorf("move the game into place: %w", err)
	}
	return nil
}

// moveEntries moves everything in from into dir.
func moveEntries(from, dir string) error {
	list, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, d := range list {
		if err = os.Rename(filepath.Join(from, d.Name()), filepath.Join(dir, d.Name())); err != nil {
			return fmt.Errorf("move the game into place: %w", err)
		}
	}
	return nil
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

// initRepo creates a git repository in game on branch, or on the default branch, commits all files and adds
// the template as the remote template.
func initRepo(ctx context.Context, e env, game, branch string) error {
	if branch == "" {
		// Exit code 1 means the option is not set.
		out, _ := command(ctx, e, game, "git", "config", "--get", "init.defaultBranch").Output()
		branch = strings.TrimSpace(string(out))
	}
	if branch == "" {
		branch = "main"
	}
	for _, step := range []struct {
		name string
		args []string
	}{
		{"create the git repository", []string{"init", "--quiet", "-b", branch}},
		{"add the files to git", []string{"add", "-A"}},
		{"make the first commit", []string{"commit", "--quiet", "-m", "Set up a fresh game"}},
		{"add the remote template", []string{"remote", "add", "template", e.template}},
	} {
		if err := git(ctx, e, game, step.args...); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

// pushToGitHub creates the GitHub repository of the game with gh and pushes the game there. Without gh or its
// login it warns and skips this step.
func pushToGitHub(ctx context.Context, e env, o options, name, game string) error {
	if command(ctx, e, game, e.gh, "auth", "status").Run() != nil {
		_, _ = fmt.Fprintln(e.out, `  WARN  Make sure the "gh" CLI tool is installed and that you're authenticated `+
			"to GitHub. Skipping...")
		return nil
	}
	repo := name
	if o.organization != "" {
		repo = o.organization + "/" + name
	}
	flags := strings.Fields(o.github.flags)
	if len(flags) == 0 {
		flags = []string{"--private"}
	}
	cmd := command(ctx, e, game, e.gh, append([]string{"repo", "create", repo, "--source=.", "--push"}, flags...)...)
	cmd.Env = e.environ
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	// gh must not wait for a password that nobody types.
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	if err := runShown(e, cmd); err != nil {
		return fmt.Errorf("create the GitHub repository: %w", err)
	}
	return nil
}

// buildEngine builds LibGodot in game with -libgodot or if the answer in a terminal is yes, and reports
// whether it did.
func buildEngine(ctx context.Context, e env, o options, lines *bufio.Scanner, game string) (bool, error) {
	if !o.libgodot && (!e.interactive || o.noInteraction || !confirm(e, lines,
		"Would you like to build LibGodot now? The first build takes 5 to 60 minutes")) {
		return false, nil
	}
	if err := runShown(e, command(ctx, e, game, e.libgodot[0], e.libgodot[1:]...)); err != nil {
		return false, fmt.Errorf("build LibGodot: %w", err)
	}
	return true, nil
}

// confirm asks question and reports whether the answer is yes, the default; the end of input means no.
func confirm(e env, lines *bufio.Scanner, question string) bool {
	_, _ = fmt.Fprintf(e.out, "%s? (yes/no) [yes]\n> ", question)
	if !lines.Scan() {
		_, _ = fmt.Fprintln(e.out)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(lines.Text())) {
	case "", "y", "yes":
		return true
	}
	return false
}

// replacement replaces old with new in file, a slash path in the template or "*.go" for every Go file.
type replacement struct{ file, old, new string }

// replaceNames applies the replacements for name in the template in dir. Each must match at least once, or the
// template does not match this version of the command.
func replaceNames(dir, name string) error {
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
