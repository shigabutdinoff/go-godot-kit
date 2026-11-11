//go:build !windows

package launcher

// useLibDir does nothing: on macOS and Linux the game's rpath.go points the loader at lib/.
func useLibDir(string) error { return nil }
