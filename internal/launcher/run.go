//go:build cgo && (darwin || linux || windows)

package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/startup"
	"graphics.gd/variant/Object"

	"github.com/shigabutdinoff/go-godot-kit/internal/config"
	"github.com/shigabutdinoff/go-godot-kit/internal/profile"
	"github.com/shigabutdinoff/go-godot-kit/internal/signals"
)

// init locks the main goroutine to its OS thread: LibGodot only works from that thread.
func init() { runtime.LockOSThread() }

// Run runs g and exits the process: 0 on success or -h, 1 on error, 2 on invalid arguments.
// It must be called from main; see pkg/kit for the behavior.
func Run(g Game) {
	exit(finish(run(g)))
}

// shutdownTimeout limits how long Game.Shutdown may wait for the game's work before the process exits.
const shutdownTimeout = 5 * time.Second

// exit terminates the process with code. It is called after run, whose defers have already flushed
// profiles and logs.
func exit(code int) { os.Exit(code) }

// finish logs the run error and returns the process exit code.
func finish(err error) int {
	logFailure(err)
	return exitCode(err)
}

// logFailure logs err unless it means a normal exit.
func logFailure(err error) {
	if exitCode(err) != exitOK {
		slog.Error("run failed", "exception.message", err)
	}
}

// errEngine means LibGodot refused to start, for example because the project directory does not exist.
var errEngine = errors.New("engine did not start")

// engineStarted is set once the engine has started: graphics.gd registers classes, calling
// [engineProbe.OnRegister], only when the engine reaches its scene level. Without the engine
// any call into it crashes the process, so this is checked before the first one.
var engineStarted atomic.Bool

// engineProbe is a class registered only to learn that the engine has started.
type engineProbe struct {
	Object.Extension[engineProbe] `gd:"KitEngineProbe"`
}

// OnRegister is called by graphics.gd when it registers the class with the started engine.
func (engineProbe) OnRegister() { engineStarted.Store(true) }

// run sets up logging and profiling, starts Godot and waits for the game to exit or for an OS signal.
func run(g Game) (err error) {
	level := &slog.LevelVar{}
	slog.SetDefault(newLogger(os.Stderr, g, level))
	baseLogger := slog.Default()
	defer func() { slog.SetDefault(baseLogger) }()
	// Signals are caught before prepare starts profiles and opens the log, so a signal cannot kill
	// the process before they are flushed.
	ctx, stop := signal.NotifyContext(context.Background(), signals.Quit...)
	defer stop()
	go watchSignal(ctx, stop)
	args, cleanup, err := prepare(os.Args, os.LookupEnv, os.Executable, g, os.Stdout, os.Stderr, level)
	defer func() { err = errors.Join(err, cleanup()) }()
	defer func() {
		if slog.Default() != baseLogger {
			logFailure(err) // the log file is still open, so the error lands there too; finish repeats it to stderr
		}
	}()
	if err != nil || args == nil {
		return err
	}
	if !release {
		if err = findLibgodot(g); err != nil {
			return err
		}
	}
	if sig := signalOf(ctx); sig != "" {
		slog.Info("OS signal before the engine started, exiting", "custom.signal", sig)
		return nil
	}
	os.Args = args
	classdb.Register[engineProbe]()
	// The game may start work in Register already, so Shutdown runs whenever Register did, before the
	// deferred cleanup closes profiles and the log.
	defer func() { err = errors.Join(err, stopThenShutdown(stop, g)) }()
	if g.Register != nil {
		g.Register(ctx)
	}
	// In LoadingScene LibGodot on macOS installs its own SIGINT handler that drops the signal, so while it loads
	// signals.Save keeps putting Go's handlers back, and Restore hands signals.Quit to Go after loading
	// (see the BUG note in pkg/kit).
	signals.Save()
	startup.LoadingScene()
	signals.Restore()
	if !engineStarted.Load() {
		return errEngine
	}
	quitOn(ctx.Done())
	slog.Debug("engine loaded")
	if g.Setup != nil {
		g.Setup(ctx)
	}
	startup.Scene()
	if sig := signalOf(ctx); sig != "" {
		slog.Info("OS signal, exiting", "custom.signal", sig)
	}
	slog.Info("exit")
	return nil
}

// stopThenShutdown cancels the context passed to Register and Setup, so the game's goroutines stop
// even when the engine stopped without a signal, and then waits for them in g.Shutdown.
func stopThenShutdown(stop context.CancelFunc, g Game) error {
	stop()
	return shutdown(g, shutdownTimeout)
}

// errShutdownTimeout means Game.Shutdown did not return before its context expired.
var errShutdownTimeout = errors.New("shutdown did not return in time")

// shutdown calls g.Shutdown, if set, with a context that expires after timeout. If Shutdown has not returned
// by then, shutdown stops waiting and returns errShutdownTimeout, so profiles and logs are still flushed;
// Shutdown keeps running in its goroutine until the process exits.
func shutdown(g Game, timeout time.Duration) error {
	if g.Shutdown == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	slog.Debug("shutdown")
	done := make(chan error, 1) // buffered: a late Shutdown must not block forever on send
	go func() { done <- g.Shutdown(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w after %s", errShutdownTimeout, timeout)
	}
}

// findLibgodot checks that LibGodot of a development build is in place and, on Windows, adds its directory
// to the DLL search path.
func findLibgodot(g Game) error {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	lib, err := libgodotPath(g, exe, runtime.GOOS)
	if err != nil {
		return err
	}
	slog.Debug("LibGodot", "custom.libgodot", lib)
	return useLibDir(filepath.Dir(lib))
}

// quitOn quits the scene tree on the first frame after quit is closed.
func quitOn(quit <-chan struct{}) {
	if tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop()); ok {
		tree.OnProcessFrame(func() {
			if closed(quit) {
				tree.Quit()
			}
		})
	}
}

// closed reports without blocking whether ch is closed.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// prepare prints the version, parses the config, opens the log file, starts profiling
// and returns the Godot arguments; nil args with a nil error mean the game must not be started.
// cleanup is never nil and must be called even on error: it closes whatever is already open.
func prepare(
	argv []string,
	lookupEnv func(string) (string, bool),
	executable func() (string, error),
	g Game,
	stdout, stderr io.Writer,
	level *slog.LevelVar,
) ([]string, func() error, error) {
	cleanup := func() error { return nil }
	if err := writeVersion(stdout, g.Build); err != nil {
		return nil, cleanup, err
	}
	cfg, err := config.Parse(g.Name, g.EnvPrefix, argv[1:], lookupEnv, stderr)
	if err != nil {
		return nil, cleanup, err
	}
	level.Set(cfg.LogLevel)
	if cfg.LogFile != "" {
		f, openErr := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			return nil, cleanup, fmt.Errorf("log file: %w", openErr)
		}
		cleanup = func() error { return errors.Join(f.Sync(), f.Close()) }
		slog.SetDefault(newLogger(f, g, level))
	}
	slog.Debug("config", "custom.config.project", cfg.Project, "custom.config.log_level", cfg.LogLevel,
		"custom.config.log_file", cfg.LogFile, "custom.config.cpu_profile", cfg.CPUProfile,
		"custom.config.mem_profile", cfg.MemProfile, "custom.config.godot", cfg.Godot)
	if cfg.Version {
		return nil, cleanup, nil
	}
	exe, err := executable()
	if err != nil {
		slog.Debug("executable path unknown, looking for the project next to the source", "exception.message", err)
		exe = ""
	}
	args, err := godotArgs(argv[0], cfg, exe, g, release)
	if err != nil {
		return nil, cleanup, err
	}
	slog.Debug("Godot arguments", "process.command_args", args)
	stopProfiles, err := profile.Start(cfg.CPUProfile, cfg.MemProfile)
	if err != nil {
		return nil, cleanup, err
	}
	closeLogs := cleanup
	cleanup = func() error { return errors.Join(stopProfiles(), closeLogs()) }
	var project string
	if i := slices.Index(args, "--path"); i >= 0 && i+1 < len(args) {
		project = args[i+1]
	}
	_, date, commit := g.Build.info()
	slog.Info("start", "custom.build_date", date, "vcs.ref.head.revision", commit, "custom.project", project)
	return args, cleanup, nil
}

// watchSignal waits for ctx to be canceled and removes the signal handler so a second Ctrl+C kills the process.
func watchSignal(ctx context.Context, stop context.CancelFunc) {
	<-ctx.Done()
	stop()
}

// signalOf returns the name of the OS signal that canceled ctx, or an empty string.
func signalOf(ctx context.Context) string {
	// Deliberately != rather than errors.Is: the NotifyContext cause itself satisfies errors.Is(…, context.Canceled),
	// so the signal would be lost. Only a bare cancellation via stop is filtered out.
	if cause := context.Cause(ctx); cause != nil && cause != context.Canceled { //nolint:errorlint // see above: errors.Is would drop the signal
		return cause.Error()
	}
	return ""
}
