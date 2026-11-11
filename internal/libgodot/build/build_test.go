package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shigabutdinoff/go-godot-kit/internal/libgodot"
)

func TestSconsArgs(t *testing.T) {
	t.Parallel()
	common := []string{"target=template_debug", "library_type=shared_library", "disable_path_overrides=no"}
	for _, tt := range []struct {
		goos, goarch string
		want         []string
	}{
		{"darwin", "arm64", []string{"platform=macos", "arch=arm64"}},
		{"darwin", "amd64", []string{"platform=macos", "arch=x86_64"}},
		{"linux", "amd64", []string{"platform=linuxbsd", "arch=x86_64"}},
		{"windows", "arm64", []string{"platform=windows", "arch=arm64", "d3d12=no"}},
	} {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			t.Parallel()
			args, err := sconsArgs(tt.goos, tt.goarch)
			require.NoError(t, err)
			assert.Subset(t, args, append(tt.want, common...))
			assert.Equal(t, tt.goos == "windows", slices.Contains(args, "d3d12=no"))
			assert.NotContains(t, args, "vulkan=no")
		})
	}
	for _, target := range [][2]string{{"js", "wasm"}, {"linux", "386"}, {"android", "arm64"}} {
		t.Run(target[0]+"/"+target[1]+" unsupported", func(t *testing.T) {
			t.Parallel()
			_, err := sconsArgs(target[0], target[1])
			assert.Error(t, err)
		})
	}
}

func TestMissingTools(t *testing.T) {
	t.Parallel()
	onlyGit := func(name string) (string, error) {
		if name == "git" {
			return "/usr/bin/git", nil
		}
		return "", exec.ErrNotFound
	}
	missing := strings.Join(missingTools("linux", onlyGit), ", ")
	assert.Contains(t, missing, "scons")
	assert.Contains(t, missing, "python3")
	assert.Contains(t, missing, "gcc")
	assert.NotContains(t, missing, "git")

	all := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	for _, goos := range []string{"darwin", "linux", "windows"} {
		assert.Empty(t, missingTools(goos, all), goos)
	}
}

func TestMoltenVK(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	brew, sdk := filepath.Join(home, "brew"), filepath.Join(home, "VulkanSDK", "1.4.2", "macOS", "lib")
	for _, lib := range []string{
		filepath.Join(brew, "MoltenVK.xcframework", "macos-arm64", "libMoltenVK.a"),
		filepath.Join(sdk, "MoltenVK.xcframework", "macos-arm64_x86_64", "libMoltenVK.a"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(lib), 0o755))
		require.NoError(t, os.WriteFile(lib, nil, 0o644))
	}
	dirs := []string{brew, sdk}
	assert.Equal(t, filepath.Join(brew, "MoltenVK.xcframework"), moltenVK("arm64", dirs))
	assert.Equal(t, filepath.Join(sdk, "MoltenVK.xcframework"), moltenVK("x86_64", dirs), "universal SDK")
	assert.Empty(t, moltenVK("x86_64", dirs[:1]))
	assert.Contains(t, moltenVKDirs(home), sdk)
	assert.Contains(t, moltenVKDirs(home), "/usr/local/Frameworks")
	assert.Equal(t, []string{"/opt/homebrew/Frameworks", "/usr/local/Frameworks", "/opt/local/Frameworks"},
		moltenVKDirs(""), "no home: system directories only")
}

func TestClone_Cancel(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := clone(ctx, cache, filepath.Join(cache, "src"), io.Discard)
	assert.ErrorIs(t, err, context.Canceled)
	left, globErr := filepath.Glob(filepath.Join(cache, "*"))
	require.NoError(t, globErr)
	assert.Empty(t, left, "temporary clone directory is removed")
}

func TestRetry(t *testing.T) {
	t.Parallel()
	errTemp := fmt.Errorf("%w: network", errTemporary)
	delays := []time.Duration{time.Millisecond, time.Millisecond}
	t.Run("succeeds on a later attempt", func(t *testing.T) {
		t.Parallel()
		calls := 0
		var out bytes.Buffer
		err := retry(t.Context(), delays, &out, func() error {
			calls++
			if calls < 3 {
				return errTemp
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
		assert.Contains(t, out.String(), "temporary error: network; retrying in 1ms")
	})
	t.Run("returns the last error after all attempts", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := retry(t.Context(), delays, io.Discard, func() error { calls++; return errTemp })
		require.ErrorIs(t, err, errTemp)
		assert.Equal(t, len(delays)+1, calls)
	})
	t.Run("does not retry a permanent error", func(t *testing.T) {
		t.Parallel()
		errPerm := errors.New("no such tag")
		calls := 0
		err := retry(t.Context(), delays, io.Discard, func() error { calls++; return errPerm })
		require.ErrorIs(t, err, errPerm)
		assert.Equal(t, 1, calls)
	})
	t.Run("does not retry after cancel", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := retry(ctx, []time.Duration{time.Hour}, io.Discard, func() error { calls++; cancel(); return errTemp })
		require.ErrorIs(t, err, errTemp)
		assert.Equal(t, 1, calls)
	})
}

func TestClassifyClone(t *testing.T) {
	t.Parallel()
	errGit := errors.New("exit status 128")
	require.NoError(t, classifyClone(nil, "fatal: unable to access"))
	err := classifyClone(errGit, "fatal: unable to access 'https://github.com/godotengine/godot/': Could not resolve host")
	require.ErrorIs(t, err, errGit)
	assert.ErrorIs(t, err, errTemporary, "a network failure is retried")
	err = classifyClone(errGit, "warning: Could not find remote branch 9.9 to clone.\nfatal: Remote branch 9.9 not found in upstream origin")
	require.ErrorIs(t, err, errGit)
	assert.NotErrorIs(t, err, errTemporary, "a missing tag is permanent")
}

func TestClearDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), nil, 0o644))
	require.NoError(t, clearDir(dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Error(t, clearDir(filepath.Join(dir, "missing")))
}

func TestCheckCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.Output()
		require.NoError(t, err, "git %v", args)
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "c")
	head := git("rev-parse", "HEAD")

	require.NoError(t, checkCommit(t.Context(), dir, head))
	assert.ErrorContains(t, checkCommit(t.Context(), dir, "0000"), "want 0000")
	assert.ErrorContains(t, checkCommit(t.Context(), t.TempDir(), head), "git rev-parse HEAD")
}

func TestFindBuilt(t *testing.T) {
	t.Parallel()
	bin := t.TempDir()
	for _, name := range []string{"libgodot.linuxbsd.template_debug.x86_64.so", "libgodot.linuxbsd.template_debug.x86_64.a"} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), nil, 0o644))
	}
	got, err := findBuilt(bin, "linux")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(bin, "libgodot.linuxbsd.template_debug.x86_64.so"), got)

	_, err = findBuilt(t.TempDir(), "linux")
	assert.Error(t, err)
}

// project returns a temporary project root and a moduleRoot function that reports it.
func project(t *testing.T) (string, func(context.Context) (string, error)) {
	t.Helper()
	root := t.TempDir()
	return root, func(context.Context) (string, error) { return root, nil }
}

// emptyCache points the user cache at a temporary directory and returns the tool's cache directory there.
func emptyCache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "AppData", "Local"))
	dir, err := cacheDir()
	require.NoError(t, err)
	return dir
}

// writeLib writes LibGodot of this machine with content to dir and returns its path.
func writeLib(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, libgodot.Name(runtime.GOOS))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestInstall_Exists(t *testing.T) {
	root, moduleRoot := project(t)
	lib := writeLib(t, filepath.Join(root, "lib"), "manual")
	var out bytes.Buffer
	require.NoError(t, Install(t.Context(), false, &out, moduleRoot))
	assert.Contains(t, out.String(), "already exists")
	got, err := os.ReadFile(lib)
	require.NoError(t, err)
	assert.Equal(t, "manual", string(got))
}

func TestInstall_Cache(t *testing.T) {
	root, moduleRoot := project(t)
	writeLib(t, emptyCache(t), "cached")
	lib := filepath.Join(root, "lib", libgodot.Name(runtime.GOOS))

	var out bytes.Buffer
	require.NoError(t, Install(t.Context(), false, &out, moduleRoot), out.String())
	got, err := os.ReadFile(lib)
	require.NoError(t, err)
	assert.Equal(t, "cached", string(got))

	writeLib(t, filepath.Dir(lib), "manual")
	require.NoError(t, Install(t.Context(), true, &out, moduleRoot), out.String())
	got, err = os.ReadFile(lib)
	require.NoError(t, err)
	assert.Equal(t, "cached", string(got), "-force overwrites lib/")
}

func TestInstall_MissingTools(t *testing.T) {
	_, moduleRoot := project(t)
	emptyCache(t)
	t.Setenv("PATH", "")
	err := Install(t.Context(), false, io.Discard, moduleRoot)
	assert.ErrorContains(t, err, "git")
	assert.ErrorContains(t, err, "scons")
}

func TestGoModRoot(t *testing.T) {
	root, err := GoModRoot(t.Context())
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "go.mod"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = GoModRoot(ctx)
	assert.ErrorContains(t, err, "go env GOMOD", "a canceled ctx stops go env")

	t.Chdir(t.TempDir())
	_, err = GoModRoot(t.Context())
	assert.ErrorContains(t, err, "run inside a Go module")
}

func TestCommand_Cancel(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no sleep command")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := command(ctx, t.TempDir(), io.Discard, io.Discard, "sleep", "30")
	assert.ErrorContains(t, err, "sleep")
	assert.Less(t, time.Since(start), commandWaitDelay, "an interrupt stops the command before the kill")
}
