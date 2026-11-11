package launcher

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/shigabutdinoff/go-godot-kit/internal/buildinfo"
	"github.com/shigabutdinoff/go-godot-kit/internal/config"
	"github.com/shigabutdinoff/go-godot-kit/internal/libgodot"
)

// projectPath looks for the graphics directory next to the executable exe, then next to the source src.
// An empty string with a nil error means the project was not found.
func projectPath(exe, src string) (string, error) {
	if exe != "" {
		r, err := filepath.EvalSymlinks(exe)
		if err != nil {
			return "", fmt.Errorf("executable path %s: %w", exe, err)
		}
		exe = r
	}
	for _, f := range []string{exe, src} {
		if f == "" {
			continue
		}
		p := filepath.Join(filepath.Dir(f), "graphics")
		fi, err := os.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return "", fmt.Errorf("project %s: %w", p, err)
		case fi.IsDir():
			return p, nil
		}
	}
	return "", nil
}

// godotArgs builds the Godot arguments. In development it appends --path to the project unless one was passed
// explicitly. In a release the scene is embedded: no --path is added, and an explicit project is a usage error,
// since the release engine refuses --path.
func godotArgs(argv0 string, cfg config.Config, exe string, g Game, release bool) ([]string, error) {
	args := append([]string{argv0}, cfg.Godot...)
	explicit := slices.Contains(cfg.Godot, "--path")
	if release {
		if explicit || cfg.Project != "" {
			return nil, fmt.Errorf("%w: release build: the scene is embedded, -project, %s_PROJECT and -- --path "+
				"are for development", config.ErrUsage, g.EnvPrefix)
		}
		return args, nil
	}
	if explicit {
		return args, nil
	}
	p := cfg.Project
	if p != "" {
		if fi, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("project %s: %w", p, err)
		} else if !fi.IsDir() {
			return nil, fmt.Errorf("project %s: not a directory", p)
		}
	} else {
		var err error
		if p, err = projectPath(exe, g.Source); err != nil {
			return nil, err
		}
		if p == "" {
			return nil, errors.New("project not found: put graphics/ next to the executable, " +
				"set -project <dir> or " + g.EnvPrefix + "_PROJECT, or pass -- --path <dir> to Godot")
		}
	}
	return append(args, "--path", p), nil
}

// libgodotPath returns the LibGodot file of a development build on goos: lib/ at the project root, two levels
// up from g.Source (cmd/<name>/main.go); on windows also next to the executable exe.
func libgodotPath(g Game, exe, goos string) (string, error) {
	name := libgodot.Name(goos)
	var paths []string
	if g.Source != "" {
		paths = append(paths, filepath.Join(filepath.Dir(g.Source), "..", "..", "lib", name))
	}
	if goos == "windows" && exe != "" {
		paths = append(paths, filepath.Join(filepath.Dir(exe), name))
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	want := filepath.Join("lib", name)
	if len(paths) > 0 {
		want = paths[0]
	}
	return "", fmt.Errorf(`LibGodot not found: put it at %s or run "go tool libgodot" in the project`, want)
}

// writeVersion prints the build version, date and commit of b to w.
func writeVersion(w io.Writer, b Build) error {
	return buildinfo.Write(w, b.Version, b.Date, b.Commit)
}
