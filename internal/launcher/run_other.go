//go:build !cgo || !(darwin || linux || windows)

package launcher

import (
	"log/slog"
	"os"
)

// Run prints the version and exits with code 1: kit runs the engine only on darwin, linux and windows with cgo.
func Run(g Game) {
	l := newLogger(os.Stderr, g, slog.LevelInfo)
	if err := writeVersion(os.Stdout, g.Build); err != nil {
		slog.NewLogLogger(l.With("exception.message", err).Handler(), slog.LevelError).Fatal("version")
	}
	slog.NewLogLogger(l.Handler(), slog.LevelError).
		Fatal("unsupported platform: kit runs the engine on darwin, linux and windows with cgo")
}
