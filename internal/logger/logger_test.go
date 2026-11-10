package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()
	level := &slog.LevelVar{}
	level.Set(slog.LevelWarn)

	var buf bytes.Buffer
	l := New(&buf, "game", "v1.2.3", level)
	l.Info("below level")
	assert.Empty(t, buf.String(), "info is dropped at warn level")
	l.Error("startup", "exception.message", errors.New("no such file"))
	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	assert.Equal(t, "game", rec["service.name"])
	assert.Equal(t, "v1.2.3", rec["service.version"])
	assert.Equal(t, instanceID, rec["service.instance.id"])
	assert.Equal(t, "startup", rec["msg"])
	assert.Equal(t, "ERROR", rec["level"])
	assert.Equal(t, "no such file", rec["exception.message"])
	assert.Contains(t, rec, "time")
	assert.NotContains(t, rec, "stacktrace")

	buf.Reset()
	level.Set(slog.LevelDebug)
	l.Debug("debug")
	assert.Contains(t, buf.String(), `"level":"DEBUG"`)
}

func TestNewUUID(t *testing.T) {
	t.Parallel()
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := newUUID(), newUUID()
	assert.Regexp(t, v4, a)
	assert.Regexp(t, v4, instanceID)
	assert.NotEqual(t, a, b, "each call returns a new id")
}

func TestModuleVersion(t *testing.T) {
	t.Parallel()
	assert.NotEmpty(t, ModuleVersion())
}

// BenchmarkLogger measures a log record with fields, the logger's hot path while a game runs.
func BenchmarkLogger(b *testing.B) {
	l := New(io.Discard, "game", "v1.2.3", slog.LevelInfo)
	b.ReportAllocs()
	for b.Loop() {
		l.Info("frame", "custom.frame", 42, "custom.scene", "main", "custom.fps", 60.0)
	}
}

// BenchmarkLogger_Disabled measures a record below the level, which must cost next to nothing.
func BenchmarkLogger_Disabled(b *testing.B) {
	l := New(io.Discard, "game", "v1.2.3", slog.LevelInfo)
	b.ReportAllocs()
	for b.Loop() {
		l.Debug("frame", "custom.frame", 42)
	}
}
