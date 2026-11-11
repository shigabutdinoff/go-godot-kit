//go:build (darwin || linux) && !archive && !musl

package main

// The development build finds LibGodot in lib/ at the kit root; a release embeds the engine.

// #cgo LDFLAGS: -Wl,-rpath,${SRCDIR}/../../lib
import "C"
