package kit

import (
	"context"

	"github.com/shigabutdinoff/go-godot-kit/internal/launcher"
)

// Game describes a game for [Run].
type Game struct {
	Name      string                    // service.name in logs, flag set name, usage text
	EnvPrefix string                    // <EnvPrefix>_PROJECT, <EnvPrefix>_LOG_LEVEL, …
	Source    string                    // game main.go path (runtime.Caller(0)); graphics/ is looked up next to it after the executable
	Register  func(ctx context.Context) // registers Godot classes before LoadingScene; ctx is canceled on an OS quit signal and before Shutdown (see [Run])
	Setup     func(ctx context.Context) // builds the scene after LoadingScene and before Scene; ctx is the one passed to Register

	// Shutdown, if set, is called once the engine has stopped, before profiles and logs are flushed, so the game
	// can wait for its goroutines and save unsaved data. Its ctx expires after 5 seconds; if Shutdown has not
	// returned by then, Run stops waiting, flushes profiles and logs and exits with code 1. An error makes Run
	// exit with code 1 too. It runs whenever Register was called, even if the engine
	// then failed to start.
	Shutdown func(ctx context.Context) error

	// BuildVersion, BuildDate and BuildCommit are the game's buildVersion, buildDate and buildCommit variables
	// of package main, set with -ldflags "-X main.buildVersion=…"; empty values are printed as N/A.
	BuildVersion, BuildDate, BuildCommit string
}

// Run starts the game and exits the process: 0 on success or -h, 1 on error, 2 on invalid arguments.
// It must be called from main. The engine runs on darwin, linux and windows (arm64 and amd64) with cgo.
//
// Run prints the build version, parses flags (env <EnvPrefix>_* overrides a flag, a flag overrides
// the JSON file from -c/-config/CONFIG/<EnvPrefix>_CONFIG), writes JSON logs with service.name = g.Name to stderr or
// -log-file (appended), starts -cpuprofile and -memprofile, and then the engine. Godot arguments go
// after -- or, without them, in godot_args of the JSON file.
//
// Development build (go run, go build): LibGodot must lie in lib/ at the project root, two levels up
// from g.Source (lib/libgodot.dylib, lib/libgodot.so or lib/libgodot.dll; on Windows a DLL next to the
// executable works too), otherwise Run exits with code 1 before starting the engine. The Godot project is,
// in order: -- --path <dir>; -project or <EnvPrefix>_PROJECT; graphics/ next to the executable; graphics/ next
// to g.Source.
//
// Release build (gd build, tag archive or musl): the engine and the scene are embedded, so Run passes no
// --path, and -project, <EnvPrefix>_PROJECT or -- --path exit with code 2. A Windows release is a GUI
// application without a console; kit does not change that.
//
// If the engine does not start (for example, the project directory does not exist), Run exits with
// code 1 and "engine did not start".
//
// The game exits on SIGINT, SIGTERM and SIGQUIT (on Windows: Ctrl+C, Ctrl+Break and closing the console;
// Windows has no SIGQUIT), flushing profiles and logs; a second signal kills the process. The context passed
// to g.Register and g.Setup is canceled on that signal and, on any exit, before g.Shutdown; the game's
// goroutines must stop on it, and g.Shutdown waits for them before Run exits. A signal before the engine starts exits without starting it.
// On Windows, with the engine's debugger on (-- -d), Ctrl+C breaks into the debugger. See also the Bugs section.
//
// Shader cache: a development build (LibGodot, template_debug) and a release (template_release) of the
// same game share user://shader_cache, and shaders cached by one may not load in the other
// ("Shader was compiled for a newer version of Metal"). The game's project.godot therefore turns
// the cache off for debug builds, so only the release keeps one:
//
//	[rendering]
//	shader_compiler/shader_cache/enabled.debug=false
func Run(g Game) {
	launcher.Run(launcher.Game{
		Name: g.Name, EnvPrefix: g.EnvPrefix, Source: g.Source,
		Register: g.Register, Setup: g.Setup, Shutdown: g.Shutdown,
		Build: launcher.Build{Version: g.BuildVersion, Date: g.BuildDate, Commit: g.BuildCommit},
	})
}
