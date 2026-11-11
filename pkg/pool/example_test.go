package pool_test

import (
	"fmt"

	"github.com/shigabutdinoff/go-godot-kit/pkg/pool"
)

// Bullet is a per-frame game object.
type Bullet struct {
	X, Y float64
	Hits []string
}

// Reset clears the bullet for reuse; cmd/reset generates such methods.
func (b *Bullet) Reset() {
	if b == nil {
		return
	}
	b.X, b.Y = 0, 0
	b.Hits = b.Hits[:0]
}

func Example() {
	bullets := pool.New(func() *Bullet { return new(Bullet) })

	b := bullets.Get()
	b.X, b.Hits = 10, append(b.Hits, "wall")
	bullets.Put(b)

	fmt.Println(b.X, len(b.Hits))
	// Output: 0 0
}
