package kit_test

import (
	"context"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	kit "github.com/shigabutdinoff/go-godot-kit/pkg/kit"
)

// buildVersion, buildDate and buildCommit are set at build time via -ldflags "-X main.buildVersion=…".
var buildVersion, buildDate, buildCommit string

// A game's cmd/<name>/main.go. The game's goroutines run in an errgroup on the context passed to Register,
// so they stop on an OS quit signal or when the engine stops, and Shutdown waits for them.
// Run exits the process, so the example is compiled but not run.
func Example() {
	_, src, _, _ := runtime.Caller(0)
	var game *errgroup.Group
	kit.Run(kit.Game{
		Name:      "demo",
		EnvPrefix: "DEMO",
		Source:    src,
		Register: func(ctx context.Context) {
			// Register Godot classes here: classdb.Register[Player]().
			// ctx is canceled on SIGINT, SIGTERM or SIGQUIT and before Shutdown.
			game, ctx = errgroup.WithContext(ctx)
			game.Go(func() error { return autosave(ctx) })
		},
		Setup: func(ctx context.Context) {
			// Build the scene here, after the engine has loaded; ctx is the one passed to Register.
		},
		Shutdown: func(ctx context.Context) error {
			// ctx expires after 5 seconds; Run stops waiting then.
			return game.Wait()
		},
		BuildVersion: buildVersion,
		BuildDate:    buildDate,
		BuildCommit:  buildCommit,
	})
}

// autosave saves the game every minute until ctx is canceled.
func autosave(ctx context.Context) error {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			// Save the game here.
		}
	}
}
