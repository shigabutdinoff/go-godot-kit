//go:build cgo

package signals

import (
	"os"
	"syscall"
)

// Quit are the OS signals the game exits on: Ctrl+C and Ctrl+Break (os.Interrupt), closing the console,
// logoff and shutdown (SIGTERM). Windows has no SIGQUIT.
var Quit = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Save does nothing on Windows: Godot's console handler passes Ctrl+C on to Go
// unless the engine's debugger is active (-- -d or --remote-debug), and then Ctrl+C breaks into the debugger.
func Save() {}

// Restore does nothing on Windows, see Save.
func Restore() {}
