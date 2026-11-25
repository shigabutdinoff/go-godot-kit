package app_test

import (
	"fmt"

	"github.com/shigabutdinoff/go-godot-tpl/internal/app"
)

func Example() {
	fmt.Println(app.New() != nil)
	// Output: true
}
