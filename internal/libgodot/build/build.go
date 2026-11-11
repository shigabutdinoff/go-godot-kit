// Package build puts LibGodot for development into lib/ of a Go module: it clones Godot at [libgodot.Version],
// builds it with scons into the user cache and copies the result from there. cmd/libgodot runs it.
package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/shigabutdinoff/go-godot-kit/internal/atomicfile"
	"github.com/shigabutdinoff/go-godot-kit/internal/libgodot"
)

const (
	repository = "https://github.com/godotengine/godot"

	docs = "https://docs.godotengine.org/en/stable/engine_details/development/compiling/"
)

// Godot platform and architecture names by GOOS and GOARCH.
var (
	platforms = map[string]string{"darwin": "macos", "linux": "linuxbsd", "windows": "windows"}
	arches    = map[string]string{"amd64": "x86_64", "arm64": "arm64"}
)

// sconsArgs returns the scons arguments that build LibGodot for development on goos/goarch, or an error
// if gd's libgodot mode does not support that target. Like gd, it drops Direct3D 12 on Windows.
func sconsArgs(goos, goarch string) ([]string, error) {
	platform, arch := platforms[goos], arches[goarch]
	if platform == "" || arch == "" {
		return nil, fmt.Errorf("LibGodot is not built for %s/%s: only darwin, linux and windows on amd64 and arm64", goos, goarch)
	}
	args := []string{"platform=" + platform, "arch=" + arch, "target=template_debug",
		"library_type=shared_library", "disable_path_overrides=no"}
	if goos == "windows" {
		args = append(args, "d3d12=no")
	}
	return args, nil
}

// moltenVKDirs returns where Homebrew (Apple silicon and Intel), MacPorts and the LunarG Vulkan SDK in home
// put MoltenVK.xcframework. With an empty home it returns only the system directories.
func moltenVKDirs(home string) []string {
	dirs := []string{"/opt/homebrew/Frameworks", "/usr/local/Frameworks", "/opt/local/Frameworks"}
	if home == "" {
		return dirs
	}
	// Glob fails only on a bad pattern, and this one is fixed.
	sdks, _ := filepath.Glob(filepath.Join(home, "VulkanSDK", "*", "macOS", "lib"))
	return append(dirs, sdks...)
}

// moltenVK returns the first MoltenVK.xcframework in dirs with a static library for the Godot arch, or "".
func moltenVK(arch string, dirs []string) string {
	for _, dir := range dirs {
		mvk := filepath.Join(dir, "MoltenVK.xcframework")
		for _, slice := range []string{"macos-arm64_x86_64", "macos-" + arch} {
			if _, err := os.Stat(filepath.Join(mvk, slice, "libMoltenVK.a")); err == nil {
				return mvk
			}
		}
	}
	return ""
}

// missingTools returns the tools the build needs on goos that lookPath does not find; alternatives are joined
// with "or". On Windows the compiler is not checked: scons finds MSVC without PATH and reports it itself.
func missingTools(goos string, lookPath func(string) (string, error)) []string {
	need := [][]string{{"git"}, {"python3", "python", "py"}, {"scons"}}
	switch goos {
	case "darwin":
		need = append(need, []string{"clang"})
	case "linux":
		need = append(need, []string{"gcc", "clang"})
	}
	var missing []string
	for _, alts := range need {
		found := slices.ContainsFunc(alts, func(name string) bool {
			_, err := lookPath(name)
			return err == nil
		})
		if !found {
			missing = append(missing, strings.Join(alts, " or "))
		}
	}
	return missing
}

// findBuilt returns the LibGodot file scons put into binDir for goos: template_debug with the shared library
// extension of goos.
func findBuilt(binDir, goos string) (string, error) {
	ext := filepath.Ext(libgodot.Name(goos))
	found, err := filepath.Glob(filepath.Join(binDir, "libgodot.*.template_debug.*"+ext))
	if err != nil {
		return "", fmt.Errorf("find built LibGodot: %w", err)
	}
	if len(found) != 1 {
		return "", fmt.Errorf("want one libgodot.*.template_debug.*%s in %s, found %d", ext, binDir, len(found))
	}
	return found[0], nil
}

// cacheDir returns the directory with Godot sources and the built LibGodot of this kit version.
func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache directory: %w", err)
	}
	return filepath.Join(dir, "go-godot-kit", "godot-"+libgodot.Version), nil
}

// report writes a tool message made from format and args to out.
func report(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, "libgodot: "+format+"\n", args...); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// Install puts LibGodot for this machine into lib/ at the root that moduleRoot returns: it keeps a file already
// there unless force, takes it from the cache or builds it there first. Cancelling ctx stops git and scons.
func Install(ctx context.Context, force bool, out io.Writer, moduleRoot func(context.Context) (string, error)) error {
	name := libgodot.Name(runtime.GOOS)
	args, err := sconsArgs(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	root, err := moduleRoot(ctx)
	if err != nil {
		return err
	}
	dst := filepath.Join(root, "lib", name)
	if _, statErr := os.Lstat(dst); statErr == nil && !force {
		return report(out, "%s already exists, -force overwrites it", dst)
	}
	cache, err := cacheDir()
	if err != nil {
		return err
	}
	cached := filepath.Join(cache, name)
	_, err = os.Stat(cached)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err = compile(ctx, cache, args, out); err != nil {
			return err
		}
		var built string
		if built, err = findBuilt(filepath.Join(cache, "src", "bin"), runtime.GOOS); err != nil {
			return err
		}
		if err = copyFile(cached, built); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("cached LibGodot: %w", err)
	default:
		if err = report(out, "using %s", cached); err != nil {
			return err
		}
	}
	if err = copyFile(dst, cached); err != nil {
		return err
	}
	return report(out, "Godot %s is at %s", libgodot.Version, dst)
}

// compile clones Godot into cache/src unless it is there and runs scons with args in it. On macOS it adds
// the MoltenVK it finds for Vulkan.
func compile(ctx context.Context, cache string, args []string, out io.Writer) error {
	missing := missingTools(runtime.GOOS, exec.LookPath)
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "" // no home: look for MoltenVK only in the system directories
		}
		if mvk := moltenVK(arches[runtime.GOARCH], moltenVKDirs(home)); mvk != "" {
			args = append(args, "vulkan_sdk_path="+mvk)
		} else {
			missing = append(missing, "MoltenVK (brew install molten-vk)")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s; the build needs git, Python, scons, a C++ compiler and on macOS MoltenVK, see %s",
			strings.Join(missing, ", "), docs)
	}
	src := filepath.Join(cache, "src")
	switch _, err := os.Stat(src); {
	case errors.Is(err, fs.ErrNotExist):
		if err = clone(ctx, cache, src, out); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("godot sources: %w", err)
	}
	if err := report(out, "scons %s in %s", strings.Join(args, " "), src); err != nil {
		return err
	}
	return command(ctx, src, out, out, "scons", args...)
}

// clone clones Godot at libgodot.Version into src through a temporary directory, so an interrupted clone
// is not taken for sources.
func clone(ctx context.Context, cache, src string, out io.Writer) error {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return fmt.Errorf("create cache: %w", err)
	}
	tmp, err := os.MkdirTemp(cache, "src-")
	if err != nil {
		return fmt.Errorf("create clone directory: %w", err)
	}
	// After a successful rename tmp is gone; a failed cleanup must not hide the clone error.
	defer func() { _ = os.RemoveAll(tmp) }()
	err = retry(ctx, cloneDelays, out, func() error {
		// A failed attempt may leave files behind, and git clones only into an empty directory.
		if clearErr := clearDir(tmp); clearErr != nil {
			return clearErr
		}
		var stderr bytes.Buffer
		cloneErr := command(ctx, cache, out, io.MultiWriter(out, &stderr),
			"git", "clone", "--depth", "1", "--branch", libgodot.Version, repository, tmp)
		return classifyClone(cloneErr, stderr.String())
	})
	if err != nil {
		return err
	}
	if err := checkCommit(ctx, tmp, libgodot.Commit); err != nil {
		return err
	}
	if err := os.Rename(tmp, src); err != nil {
		return fmt.Errorf("move clone to sources: %w", err)
	}
	return nil
}

// cloneDelays are the pauses before the repeated git clone attempts: a network error is often temporary.
var cloneDelays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}

// errTemporary marks an error that may not happen again on a repeated attempt. Any other error is permanent.
var errTemporary = errors.New("temporary error")

// networkFailures are git messages about a network failure, the only clone failure worth repeating.
var networkFailures = []string{
	"Could not resolve host", "Failed to connect", "Connection refused", "Connection reset", "Connection timed out",
	"Operation timed out", "unable to access", "early EOF", "RPC failed", "remote end hung up",
}

// classifyClone marks err of git clone with errTemporary if stderr of git reports a network failure.
// Other failures, such as a missing tag, are permanent.
func classifyClone(err error, stderr string) error {
	if err == nil {
		return nil
	}
	for _, msg := range networkFailures {
		if strings.Contains(stderr, msg) {
			return fmt.Errorf("%w: %w", errTemporary, err)
		}
	}
	return err
}

// retry calls f, and calls it again after each of delays while it fails with errTemporary. It stops waiting
// when ctx is canceled and returns the last error of f.
func retry(ctx context.Context, delays []time.Duration, out io.Writer, f func() error) error {
	err := f()
	for _, d := range delays {
		if !errors.Is(err, errTemporary) || ctx.Err() != nil {
			return err
		}
		if repErr := report(out, "%v; retrying in %s", err, d); repErr != nil {
			return errors.Join(err, repErr)
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(d):
		}
		err = f()
	}
	return err
}

// clearDir removes everything inside dir, keeping dir itself.
func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("clear %s: %w", dir, err)
		}
	}
	return nil
}

// checkCommit returns an error unless the git checkout in dir is at commit want.
func checkCommit(ctx context.Context, dir, want string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		return fmt.Errorf("godot %s is at commit %s, want %s", libgodot.Version, got, want)
	}
	return nil
}

// commandWaitDelay is how long a cancelled command has to stop its children after an interrupt before it is
// killed and its output pipes are closed.
const commandWaitDelay = 10 * time.Second

// command runs name with args in dir, its output going to stdout and stderr. Cancelling ctx interrupts it, as
// Ctrl+C would, so scons stops its compilers too; after commandWaitDelay it is killed. Where an interrupt cannot
// be sent (Windows), it is killed at once.
func command(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, stdout, stderr
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = commandWaitDelay
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// copyFile copies src to dst through a temporary file next to dst, replacing dst itself even if it is a symlink.
func copyFile(dst, src string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", dst, err)
	}
	return atomicfile.WriteFrom(dst, 0o755, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	})
}

// GoModRoot returns the directory of the main module's go.mod, as go env GOMOD reports it.
// Cancelling ctx stops go env.
func GoModRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", errors.New("no go.mod found: run inside a Go module")
	}
	return filepath.Dir(gomod), nil
}
