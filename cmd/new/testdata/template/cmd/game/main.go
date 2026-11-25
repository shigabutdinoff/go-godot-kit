package main

import (
	"github.com/shigabutdinoff/go-godot-tpl/internal/app"
)

// game stands in for kit.Game, so the copy of the template needs no modules.
type game struct {
	Name      string
	EnvPrefix string
	App       *app.App
}

func main() {
	_ = newGame()
}

func newGame() game {
	return game{
		Name:      "game",
		EnvPrefix: "GAME",
		App:       app.New(),
	}
}
