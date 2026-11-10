// Package logger builds a JSON slog logger on top of zap with OpenTelemetry resource attributes.
package logger

import (
	"cmp"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"

	"github.com/shigabutdinoff/go-godot-kit/internal/buildinfo"
)

// instanceID is service.instance.id: one random UUID per process, so records of one run can be told apart.
var instanceID = newUUID()

// New returns a logger that writes JSON to w at level or above; writes to w are serialized.
// Every record carries service.name, service.version and the run's service.instance.id.
func New(w io.Writer, service, version string, level slog.Leveler) *slog.Logger {
	enc := zap.NewProductionEncoderConfig()
	enc.TimeKey = "time"
	enc.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	enc.EncodeLevel = zapcore.CapitalLevelEncoder

	enabler := zap.LevelEnablerFunc(func(l zapcore.Level) bool { return slog.Level(4*int(l)) >= level.Level() })
	core := zapcore.NewCore(zapcore.NewJSONEncoder(enc), zapcore.Lock(zapcore.AddSync(w)), enabler)

	h := zapslog.NewHandler(core, zapslog.AddStacktraceAt(slog.LevelError+1))
	return slog.New(h).With("service.name", service, "service.version", version, "service.instance.id", instanceID)
}

// ModuleVersion returns the main module version from debug.BuildInfo or [buildinfo.NA].
func ModuleVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return buildinfo.NA
	}
	return cmp.Or(bi.Main.Version, buildinfo.NA)
}

// newUUID returns a random version 4 UUID in canonical text form.
func newUUID() string {
	var b [16]byte
	rand.Read(b[:]) // never returns an error since Go 1.24: on failure it crashes the program
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
