package main

import "os"

func main() {
	os.Exit(1) // want "direct os.Exit call in main"
	func() {
		os.Exit(2) // want "direct os.Exit call in main"
	}()
	exit := os.Exit // want "os.Exit used as a value in main"
	exit(3)
	defer os.Exit(4) // want "direct os.Exit call in main"
}

func f() { os.Exit(1) }
