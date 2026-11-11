// Command game is the smallest game on kit: an empty 3D scene. The smoke test builds and runs it.
package main

import (
	"context"
	"log/slog"
	"runtime"

	kit "github.com/shigabutdinoff/go-godot-kit/pkg/kit"
)

// buildVersion, buildDate and buildCommit are set at build time via -ldflags "-X main.buildVersion=…".
var buildVersion, buildDate, buildCommit string

func main() {
	_, src, _, _ := runtime.Caller(0)
	kit.Run(kit.Game{
		Name: "game", EnvPrefix: "GAME", Source: src,
		Shutdown: func(context.Context) error {
			slog.Info("game saved") // the smoke test checks that Shutdown runs while the log is open
			return nil
		},
		BuildVersion: buildVersion, BuildDate: buildDate, BuildCommit: buildCommit,
	})
}
