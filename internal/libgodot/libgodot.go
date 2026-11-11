// Package libgodot holds what kit and its tools share about LibGodot, the engine built as a library.
package libgodot

// Version is the Godot release that LibGodot for development is built from. It changes together with
// graphics.gd in go.mod.
const Version = "4.7.2-stable"

// Commit is the Godot commit that the Version tag points to. A clone is checked against it, so a moved tag
// cannot change what gets built.
const Commit = "ed1daf0bf001b61586d9930840f2f1394092c079"

// Name returns the LibGodot file name graphics.gd loads on goos, or "" if it loads none there.
func Name(goos string) string {
	switch goos {
	case "darwin":
		return "libgodot.dylib"
	case "linux":
		return "libgodot.so"
	case "windows":
		return "libgodot.dll"
	}
	return ""
}
