// Package launcher runs a game with LibGodot for pkg/kit: it parses settings, sets up logs and profiles,
// finds the Godot project and LibGodot, starts the engine and exits on OS signals.
package launcher

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shigabutdinoff/go-godot-kit/internal/buildinfo"
	"github.com/shigabutdinoff/go-godot-kit/internal/config"
	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
)

// Game describes a game for [Run]; pkg/kit documents the fields.
type Game struct {
	Name      string                          // service.name in logs, flag set name, usage text
	EnvPrefix string                          // <EnvPrefix>_PROJECT, <EnvPrefix>_LOG_LEVEL, …
	Source    string                          // game main.go path (runtime.Caller(0)); graphics/ is looked up next to it after the executable
	Register  func(ctx context.Context)       // registers Godot classes before LoadingScene; ctx is canceled on an OS quit signal and before Shutdown (see [Run])
	Setup     func(ctx context.Context)       // builds the scene after LoadingScene and before Scene; ctx as in Register
	Shutdown  func(ctx context.Context) error // waits for the game's work after the engine stops; ctx has shutdownTimeout
	Build     Build                           // build version, date and commit
}

// Build is the build version, date and commit; empty values are printed as N/A.
type Build struct {
	Version string // build version, e.g. v1.2.3
	Date    string // build date, e.g. 2026-01-02
	Commit  string // build commit, e.g. abc1234
}

// Process exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// exitCode maps err to an exit code: 0 for success or -h, 2 for invalid arguments, 1 otherwise.
func exitCode(err error) int {
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return exitOK
	case errors.Is(err, config.ErrUsage):
		return exitUsage
	}
	return exitError
}

// info returns the build version, date and commit, with N/A in place of empty values.
func (b Build) info() (ver, date, commit string) {
	return cmp.Or(b.Version, buildinfo.NA), cmp.Or(b.Date, buildinfo.NA), cmp.Or(b.Commit, buildinfo.NA)
}

// newLogger returns the JSON logger of g writing to w, with the build version, the deployment environment
// and the platform; the home directory is masked in every record (see maskHome).
func newLogger(w io.Writer, g Game, level slog.Leveler) *slog.Logger {
	ver, _, _ := g.Build.info()
	home, err := os.UserHomeDir()
	if err != nil {
		home = "" // no home directory: nothing to mask
	}
	l := slog.New(homeMasker{Handler: logger.New(w, g.Name, ver, level).Handler(), home: home})
	return l.With("deployment.environment.name", environment,
		"os.type", runtime.GOOS, "host.arch", hostArch(runtime.GOARCH))
}

// hostArch maps GOARCH to the host.arch value from the OpenTelemetry semantic conventions.
func hostArch(goarch string) string {
	switch goarch {
	case "386":
		return "x86"
	case "arm":
		return "arm32"
	}
	return goarch
}

// maskHome replaces the home directory with ~ wherever s holds a path inside it, so a log shipped off the
// player's machine does not carry their OS user name. s may be a path or an error text with paths in it.
func maskHome(s, home string) string {
	if home == "" {
		return s
	}
	if s == home {
		return "~"
	}
	sep := string(filepath.Separator)
	return strings.ReplaceAll(s, home+sep, "~"+sep)
}

// homeMasker is a slog.Handler that applies maskHome to the message and to every string, string slice and
// error attribute before passing the record on.
type homeMasker struct {
	slog.Handler
	home string
}

var _ slog.Handler = homeMasker{}

// Handle masks r and passes it to the wrapped handler.
func (h homeMasker) Handle(ctx context.Context, r slog.Record) error {
	m := slog.NewRecord(r.Time, r.Level, maskHome(r.Message, h.home), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		m.AddAttrs(h.attr(a))
		return true
	})
	return h.Handler.Handle(ctx, m)
}

// WithAttrs masks attrs and returns a masking handler with them.
func (h homeMasker) WithAttrs(attrs []slog.Attr) slog.Handler {
	masked := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		masked[i] = h.attr(a)
	}
	return homeMasker{Handler: h.Handler.WithAttrs(masked), home: h.home}
}

// WithGroup returns a masking handler with the group.
func (h homeMasker) WithGroup(name string) slog.Handler {
	return homeMasker{Handler: h.Handler.WithGroup(name), home: h.home}
}

// attr returns a with home masked in its string, string slice or error value; groups are masked recursively.
func (h homeMasker) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, maskHome(v.String(), h.home))
	case slog.KindGroup:
		group := v.Group()
		masked := make([]any, len(group))
		for i, g := range group {
			masked[i] = h.attr(g)
		}
		return slog.Group(a.Key, masked...)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case error:
			return slog.String(a.Key, maskHome(x.Error(), h.home))
		case []string:
			masked := make([]string, len(x))
			for i, s := range x {
				masked[i] = maskHome(s, h.home)
			}
			return slog.Any(a.Key, masked)
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}
