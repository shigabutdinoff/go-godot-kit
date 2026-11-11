package pool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type buffer struct {
	data   []byte
	resets int
}

func (b *buffer) Reset() {
	b.data = b.data[:0]
	b.resets++
}

func TestPool(t *testing.T) {
	t.Parallel()
	p := New(func() *buffer { return new(buffer) })

	b := p.Get()
	require.NotNil(t, b)
	b.data = append(b.data, "frame"...)
	p.Put(b)
	assert.Empty(t, b.data, "Put resets the object")
	assert.Equal(t, 1, b.resets)
	assert.NotNil(t, p.Get(), "Get never returns nil")
}

// counts is a map type: maps are not comparable, and a Pool takes any type with a Reset method.
type counts map[string]int

func (c counts) Reset() { clear(c) }

func TestPool_NotComparable(t *testing.T) {
	t.Parallel()
	p := New(func() counts { return counts{} })
	c := p.Get()
	c["frame"]++
	p.Put(c)
	assert.Empty(t, c, "Put resets a map object")
}

// TestPool_Concurrent checks under -race that Get and Put from many goroutines never share an object.
func TestPool_Concurrent(t *testing.T) {
	t.Parallel()
	p := New(func() *buffer { return new(buffer) })
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				b := p.Get()
				assert.Empty(t, b.data, "an object from the pool is reset")
				b.data = append(b.data, "frame"...)
				p.Put(b)
			}
		})
	}
	wg.Wait()
}

// BenchmarkPool measures a Get, a write and a Put of a pooled object; a steady state allocates nothing.
func BenchmarkPool(b *testing.B) {
	p := New(func() *buffer { return new(buffer) })
	b.ReportAllocs()
	for b.Loop() {
		x := p.Get()
		x.data = append(x.data, "frame"...)
		p.Put(x)
	}
}
