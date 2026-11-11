// SPDX-License-Identifier: MIT

// Package kit runs a Go game with Godot embedded through LibGodot (graphics.gd).
//
// A game's main calls [Run] with a [Game]: kit parses flags, <EnvPrefix>_* variables and the
// JSON config, sets up JSON logs, profiles and the build version, finds the Godot project and
// starts the engine. The game only registers its classes and builds its scene.
//
// The engine runs on the targets of gd's libgodot mode: darwin, linux and windows on arm64 and amd64,
// built with cgo. A development build loads LibGodot from lib/ at the project root; a release built by
// gd embeds the engine and the scene. Elsewhere, or without cgo, [Run] prints the version and exits with code 1.
//
// The build version, date and commit are variables of the game's package main, set with -ldflags
// and passed in [Game]:
//
//	-X main.buildVersion=v1.2.3 -X main.buildDate=2026-01-02 -X main.buildCommit=abc1234
package kit

// BUG(bulat): On macOS, while the engine loads, LibGodot replaces the SIGINT handler with one that drops
// the signal. kit puts Go's handler back within 0.1 ms; a SIGINT in that gap is lost, so Ctrl+C must be
// pressed again. Closing the gap fully needs a LibGodot patch.
