package main

import . "os"

func main() {
	Exit(1) // want "direct os.Exit call in main"
}
