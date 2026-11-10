package logger_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/shigabutdinoff/go-godot-kit/internal/logger"
)

func ExampleNew() {
	var buf bytes.Buffer
	log := logger.New(&buf, "demo", "v1.0.0", slog.LevelInfo)
	log.Debug("dropped: below Info")
	log.Info("start", "custom.port", 8080)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		panic(err)
	}
	fmt.Println(rec["level"], rec["msg"], rec["service.name"], rec["service.version"], rec["custom.port"])
	// Output: INFO start demo v1.0.0 8080
}
