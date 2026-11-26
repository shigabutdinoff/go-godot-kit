package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// kitModule is the module path of kit.
const kitModule = "github.com/shigabutdinoff/go-godot-kit"

// sharedFiles are the files and folders of the template that update takes from its master.
var sharedFiles = []string{".github", ".golangci.yml", ".gitattributes", ".gitignore", "staticcheck.conf"}

// update updates kit and the shared files of the game in e.dir and prints what changed.
func update(ctx context.Context, args []string, e env) error {
	version, err := parseUpdate(args, e)
	if err != nil {
		return err
	}
	before, err := kitVersion(e.dir)
	if err != nil {
		return err
	}
	status, err := gitLines(ctx, e, "status", "--porcelain")
	if err != nil {
		return err
	}
	if len(status) > 0 {
		return errors.New("the game has uncommitted changes: commit or stash them and run update again")
	}
	// The template comes first: if it cannot be fetched, nothing is changed.
	if err = git(ctx, e, e.dir, "fetch", "--quiet", "--no-tags", e.template, "master"); err != nil {
		return fmt.Errorf("fetch the template: %w", err)
	}
	if err = runShown(e, command(ctx, e, e.dir, e.goTool, "get", kitModule+"@"+version)); err != nil {
		return fmt.Errorf("update kit: %w", err)
	}
	if err = runShown(e, command(ctx, e, e.dir, e.goTool, "mod", "tidy")); err != nil {
		return fmt.Errorf("tidy go.mod: %w", err)
	}
	after, err := kitVersion(e.dir)
	if err != nil {
		return err
	}
	if err = sharedFromTemplate(ctx, e); err != nil {
		return err
	}
	return report(ctx, e, before, after)
}

// parseUpdate returns the version to update kit to: the argument or latest.
func parseUpdate(args []string, e env) (string, error) {
	flags := flag.NewFlagSet("gogodot update", flag.ContinueOnError)
	flags.SetOutput(e.errOut)
	flags.Usage = func() {
		_, _ = fmt.Fprint(flags.Output(), "Usage: gogodot update [version]\n\n"+
			"Updates kit in the game to version vX.Y.Z, or to the latest release without it, and the shared files\n"+
			"from master of the template. Run it in the game folder without uncommitted changes; nothing is committed.\n")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", err
		}
		return "", usageError{err}
	}
	switch flags.NArg() {
	case 0:
		return "latest", nil
	case 1:
	default:
		return "", usageError{fmt.Errorf("unexpected argument %q", flags.Arg(1))}
	}
	v := flags.Arg(0)
	if !semver.IsValid(v) || semver.Canonical(v) != v {
		return "", usageError{fmt.Errorf("invalid version %q: use vX.Y.Z, for example v0.2.0", v)}
	}
	return v, nil
}

// kitVersion returns the version of kit in the go.mod in dir.
func kitVersion(dir string) (string, error) {
	path := filepath.Join(dir, "go.mod")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("no game here: go.mod not found, run update in the game folder")
	}
	if err != nil {
		return "", err
	}
	f, err := modfile.ParseLax(path, b, nil)
	if err != nil {
		return "", err
	}
	for _, r := range f.Require {
		if r.Mod.Path == kitModule {
			return r.Mod.Version, nil
		}
	}
	return "", errors.New("no game on kit here: go.mod does not require " + kitModule)
}

// sharedFromTemplate takes the shared files from the fetched master of the template and removes those it no
// longer has if the game has them as some commit of the template had them. Files of the game itself stay.
func sharedFromTemplate(ctx context.Context, e env) error {
	files, err := gitLines(ctx, e, append([]string{"ls-tree", "-r", "--name-only", "FETCH_HEAD", "--"}, sharedFiles...)...)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		if err = git(ctx, e, e.dir, append([]string{"checkout", "FETCH_HEAD", "--"}, files...)...); err != nil {
			return fmt.Errorf("take the shared files: %w", err)
		}
	}
	tracked, err := gitLines(ctx, e, append([]string{"ls-files", "-s", "--"}, sharedFiles...)...)
	if err != nil {
		return err
	}
	inTemplate := make(map[string]bool, len(files))
	for _, f := range files {
		inTemplate[f] = true
	}
	for _, line := range tracked {
		// <mode> <object> <stage>\t<file>
		info, file, _ := strings.Cut(line, "\t")
		if inTemplate[file] {
			continue
		}
		// Every object the template had for file, as the old and new object of each change.
		history, err := gitLines(ctx, e, "log", "--root", "--format=", "--raw", "--no-abbrev", "FETCH_HEAD", "--", file)
		if err != nil {
			return err
		}
		if !strings.Contains(strings.Join(history, "\n"), " "+strings.Fields(info)[1]+" ") {
			continue
		}
		if err = git(ctx, e, e.dir, "rm", "--quiet", "--", file); err != nil {
			return fmt.Errorf("remove %s: %w", file, err)
		}
	}
	return nil
}

// report prints the kit versions and the shared files changed, or that there was nothing to update.
func report(ctx context.Context, e env, before, after string) error {
	status, err := gitLines(ctx, e, "status", "--porcelain")
	if err != nil {
		return err
	}
	if len(status) == 0 {
		_, err = fmt.Fprintf(e.out, "Nothing to update: kit %s and the shared files are up to date.\n", before)
		return err
	}
	files, err := gitLines(ctx, e, append([]string{"diff", "--name-only", "HEAD", "--"}, sharedFiles...)...)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("kit %s -> %s\n", before, after)
	if len(files) > 0 {
		text += "Shared files changed:\n  " + strings.Join(files, "\n  ") + "\n"
	}
	_, err = fmt.Fprint(e.out, text+"Review the changes and commit them.\n")
	return err
}

// gitLines runs git in e.dir and returns the lines of its output; its errors go to the terminal.
func gitLines(ctx context.Context, e env, args ...string) ([]string, error) {
	cmd := command(ctx, e, e.dir, "git", args...)
	cmd.Stderr = e.errOut
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.FieldsFunc(string(out), func(r rune) bool { return r == '\n' || r == '\r' }), nil
}
