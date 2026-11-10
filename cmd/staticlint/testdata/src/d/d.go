package main

import x "os"

func main() {
	x.Exit(1) // want "direct os.Exit call in main"
}
