// Libgodot builds LibGodot for development from Godot sources on this machine and puts it into lib/ at the
// root of the Go module it runs in, under the name graphics.gd loads: libgodot.dylib, libgodot.so or libgodot.dll.
//
// It clones Godot at the tag kit is built for and runs scons with the platform and architecture of this
// machine, target=template_debug, library_type=shared_library, disable_path_overrides=no, plus d3d12=no on
// Windows as gd does. It needs git, Python, scons and a C++ compiler (macOS: Xcode, Linux: gcc or clang,
// Windows: MSVC or mingw), and on macOS MoltenVK for Vulkan (brew install molten-vk or the LunarG Vulkan SDK);
// it checks them and names what is missing, but installs nothing.
// Sources and the result stay in the user cache (go-godot-kit/godot-<version>), so the next project on the
// machine gets LibGodot without a new build. LibGodot already in lib/, built by hand or not, is kept
// unless -force is given.
//
// A game runs it with the tool directive "tool github.com/shigabutdinoff/go-godot-kit/cmd/libgodot" in its go.mod:
//
//	go tool libgodot [-force]
//
// Its progress messages start with "libgodot:"; the output of git and scons goes to the terminal as is.
// A failure is a JSON log record on stderr, as in kit and cmd/reset. Exit code 0 means LibGodot is in lib/,
// 1 is an error.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/shigabutdinoff/go-godot-kit/internal/buildinfo"
	"github.com/shigabutdinoff/go-godot-kit/internal/libgodot/build"
	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
)

// Build version, date and commit, set at build time:
// go build -ldflags "-X main.buildVersion=v1.2.3 -X main.buildDate=2026-01-02 -X main.buildCommit=abc1234".
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// main prints the build version and runs the tool with the command line arguments until Ctrl+C, SIGTERM or
// SIGQUIT; on error it logs it and exits with code 1.
func main() {
	err := buildinfo.Write(os.Stdout, buildVersion, buildDate, buildCommit)
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		// After the first signal the default handlers are back, so a second one kills the tool while git or scons stop.
		go func() { <-ctx.Done(); stop() }()
		err = run(ctx, os.Args[1:], os.Stderr, build.GoModRoot)
		stop()
	}
	if err != nil {
		l := logger.New(os.Stderr, "libgodot", logger.ModuleVersion(), slog.LevelInfo).With("exception.message", err)
		slog.NewLogLogger(l.Handler(), slog.LevelError).Fatal("LibGodot not installed")
	}
}

// run parses args and puts LibGodot into lib/ under the project root that moduleRoot returns. Messages and
// the output of git and scons go to out. On -h it prints usage and returns nil. Cancelling ctx stops git and
// scons, and run returns the error.
func run(ctx context.Context, args []string, out io.Writer, moduleRoot func(context.Context) (string, error)) error {
	flags := flag.NewFlagSet("libgodot", flag.ContinueOnError)
	flags.SetOutput(out)
	force := flags.Bool("force", false, "overwrite LibGodot already in lib/")
	switch err := flags.Parse(args); {
	case errors.Is(err, flag.ErrHelp):
		return nil
	case err != nil:
		return err
	case flags.NArg() > 0:
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	return build.Install(ctx, *force, out, moduleRoot)
}
