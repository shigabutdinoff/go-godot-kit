package config_test

import (
	"fmt"
	"io"

	"github.com/shigabutdinoff/go-godot-kit/internal/config"
)

func ExampleParse() {
	env := map[string]string{"GAME_LOG_LEVEL": "debug"}
	lookupEnv := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	cfg, err := config.Parse("game", "GAME", []string{"-log-level", "error", "-project", "/p", "--", "--verbose"}, lookupEnv, io.Discard)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.LogLevel, cfg.Project, cfg.Godot)
	// Output: DEBUG /p [--verbose]
}
