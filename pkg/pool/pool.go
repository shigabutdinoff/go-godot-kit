// SPDX-License-Identifier: MIT

// Package pool keeps reusable objects of one type, such as per-frame game objects, on top of [sync.Pool].
// Objects are reset before they return to the pool; cmd/reset generates such Reset methods.
package pool

import "sync"

// Resetter constrains [Pool] objects: types, usually pointers, with a Reset method.
type Resetter interface {
	// Reset clears the object for reuse; [Pool.Put] calls it before the object returns to the pool.
	Reset()
}

// Pool holds objects of type T. It is safe for concurrent use.
type Pool[T Resetter] struct {
	p sync.Pool
}

// New returns an empty pool; Get on it calls newT for a new object when the pool is empty.
func New[T Resetter](newT func() T) *Pool[T] {
	return &Pool[T]{p: sync.Pool{New: func() any { return newT() }}}
}

// Get returns an object from the pool, or a new one from newT if the pool is empty.
func (p *Pool[T]) Get() T {
	return p.p.Get().(T)
}

// Put resets x and puts it into the pool. As with [sync.Pool], x must not be nil: Get would return it.
func (p *Pool[T]) Put(x T) {
	x.Reset()
	p.p.Put(x)
}
