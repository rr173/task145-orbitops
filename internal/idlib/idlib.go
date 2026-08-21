// Package idlib produces deterministic identifiers. No randomness is used so
// that event-stream replay reproduces identical IDs across restarts.
package idlib

import "sync"

// Generator emits monotonic, zero-prefixed decimal identifiers. It is safe
// for concurrent use. IDs are stable per call sequence, which is what replay
// requires: given the same input sequence, IDs come out identical.
type Generator struct {
	mu  sync.Mutex
	seq int64
}

func New() *Generator { return &Generator{} }

func (g *Generator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	// 16-digit zero-padded decimal id; easily sortable and collision-free
	// within a process. Replay stability comes from identical call order.
	return formatID(g.seq)
}

func formatID(n int64) string {
	const width = 16
	var buf [width]byte
	for i := width - 1; i >= 0; i-- {
		buf[i] = byte('0' + int(n%10))
		n /= 10
	}
	return string(buf[:])
}
