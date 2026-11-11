// Reset generates Reset() methods for structs marked with the comment
//
//	// generate:reset
//
// It walks all packages of the module from the root (the argument, "." by default) and writes
// reset.gen.go into every package with marked structs. Reset returns fields to their initial state:
// numbers, strings and bools to zero, slices to length 0 keeping capacity, maps are emptied with clear,
// and nested structs and non-nil pointers with a Reset method get it called. A stale reset.gen.go is removed.
//
// A game runs it via a go:generate directive in its package. With the tool directive
// "tool github.com/shigabutdinoff/go-godot-kit/cmd/reset" in the game's go.mod:
//
//	//go:generate go tool reset .
//
// Inside this module it runs as:
//
//	go run ./cmd/reset .
package main

import (
	"cmp"
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
	"github.com/shigabutdinoff/go-godot-kit/internal/errwriter"
	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
	"github.com/shigabutdinoff/go-godot-kit/internal/resetgen"
)

// Build version, date and commit, set at build time:
// go build -ldflags "-X main.buildVersion=v1.2.3 -X main.buildDate=2026-01-02 -X main.buildCommit=abc1234".
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// main prints the build version, parses the arguments and runs generation; on error it logs it and exits
// with code 1.
func main() {
	var root string
	err := buildinfo.Write(os.Stdout, buildVersion, buildDate, buildCommit)
	if err == nil {
		root, err = parseArgs(os.Args[1:], os.Stderr)
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err == nil {
		err = start(root)
	}
	if err != nil {
		l := logger.New(os.Stderr, "reset", logger.ModuleVersion(), slog.LevelInfo).With("exception.message", err)
		slog.NewLogLogger(l.Handler(), slog.LevelError).Fatal("generation failed")
	}
}

// parseArgs returns the module directory from args, "." by default, or an error on bad flags or an extra argument.
// On -h it prints usage to out and returns flag.ErrHelp, or the write error if usage could not be written.
func parseArgs(args []string, out io.Writer) (string, error) {
	flags := flag.NewFlagSet("reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	usageOut := errwriter.New(out)
	flags.Usage = func() {
		usageOut.Record(fmt.Fprintln(usageOut, "Usage: reset [module directory, default .]"))
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) && usageOut.Err() != nil {
			return "", fmt.Errorf("usage: %w", usageOut.Err())
		}
		return "", err
	}
	if len(flags.Args()) > 1 {
		return "", fmt.Errorf("unexpected argument %q", flags.Arg(1))
	}
	return cmp.Or(flags.Arg(0), "."), nil
}

// start calls [resetgen.Run] for root with a context canceled on SIGINT, SIGTERM and SIGQUIT.
func start(root string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	// After the first signal the default handlers are back, so a second one kills the tool at once.
	go func() { <-ctx.Done(); stop() }()
	return resetgen.Run(ctx, root)
}
