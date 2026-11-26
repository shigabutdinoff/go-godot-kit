package main

import "testing"

func TestNewGame(t *testing.T) {
	g := newGame()

	equal(t, "game", g.Name)
	equal(t, "GAME", g.EnvPrefix)
}

func equal(t *testing.T, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
