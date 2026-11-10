// Package buildinfo formats the build version, date and commit that kit and its tools print at start.
package buildinfo

import (
	"cmp"
	"fmt"
	"io"
)

// NA replaces an empty build value in output.
const NA = "N/A"

// String returns the lines "Build version: …", "Build date: …" and "Build commit: …", with [NA] in place of
// empty values.
func String(version, date, commit string) string {
	return fmt.Sprintf("Build version: %s\nBuild date: %s\nBuild commit: %s",
		cmp.Or(version, NA), cmp.Or(date, NA), cmp.Or(commit, NA))
}

// Write prints the [String] lines and a newline to w.
func Write(w io.Writer, version, date, commit string) error {
	if _, err := fmt.Fprintln(w, String(version, date, commit)); err != nil {
		return fmt.Errorf("version: %w", err)
	}
	return nil
}
