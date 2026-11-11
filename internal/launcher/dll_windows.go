package launcher

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// useLibDir adds dir to the DLL search path, so graphics.gd loads libgodot.dll from it:
// go run puts the executable in a temporary directory, away from the project's lib/.
func useLibDir(dir string) error {
	if err := windows.SetDllDirectory(dir); err != nil {
		return fmt.Errorf("LibGodot directory %s: %w", dir, err)
	}
	return nil
}
