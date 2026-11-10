// Gendoc updates the list of enabled staticcheck checks in cmd/staticlint/doc.go.
// The text before and after the list markers is kept. Run it with go generate ./cmd/staticlint.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	staticchecks "github.com/shigabutdinoff/go-godot-kit/cmd/staticlint/internal/checks"
	"github.com/shigabutdinoff/go-godot-kit/internal/atomicfile"
	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
)

const (
	begin = "// All enabled staticcheck checks (the list is updated by go generate, do not edit by hand):"
	end   = "// # Third-party analyzers"
)

var titleFix = strings.NewReplacer(`\'`, "`", `\"`, `"`)

// main updates doc.go in the current directory and exits with an error if that fails.
func main() {
	if err := run("doc.go"); err != nil {
		l := logger.New(os.Stderr, "gendoc", logger.ModuleVersion(), slog.LevelInfo).With("exception.message", err)
		slog.NewLogLogger(l.Handler(), slog.LevelError).Fatal("generation failed")
	}
}

// run rewrites the file at path, inserting the staticcheck checks that the built-in config.json enables.
// It returns a read error, a write error or an error for missing list markers.
func run(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	as, err := staticchecks.Staticcheck(staticchecks.Default)
	if err != nil {
		return err
	}
	var checks []string
	for _, a := range as {
		title := titleFix.Replace(a.Doc.Title)
		checks = append(checks, a.Analyzer.Name+" — "+strings.Join(strings.Fields(title), " "))
	}
	out, err := update(string(src), checks)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := atomicfile.Write(path, []byte(out), 0o644); err != nil {
		return err
	}
	return nil
}

// update returns doc with the list between the begin and end markers replaced by checks.
// It returns an error if the markers are missing or out of order.
func update(doc string, checks []string) (string, error) {
	i := strings.Index(doc, begin+"\n")
	j := strings.Index(doc, "\n"+end+"\n")
	if i < 0 || j < i {
		return "", fmt.Errorf("doc.go lacks lines %q and %q (in this order)", begin, end)
	}
	var b strings.Builder
	b.WriteString(doc[:i+len(begin)+1])
	b.WriteString("//\n")
	for _, c := range checks {
		b.WriteString("//   - " + c + "\n")
	}
	b.WriteString("//")
	b.WriteString(doc[j:])
	return b.String(), nil
}
