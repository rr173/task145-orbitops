// Package clock provides an injectable clock so orbit math stays deterministic
// while wall-clock-driven operations (event ordering, smoke test) can be faked.
package clock

import (
	"sync"
	"time"

	"orbitops/internal/model"
)

// Clock reports the current epoch (seconds since J2000).
type Clock interface {
	Now() model.Epoch
}

// Real converts wall time to the J2000 epoch. Used by main only.
type Real struct{}

// J2000 instant 2000-01-01T12:00:00Z.
var j2000 = time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

func (Real) Now() model.Epoch {
	return model.Epoch(time.Since(j2000).Seconds())
}

// Fake is a manually advanced clock for deterministic tests and smoke checks.
type Fake struct {
	mu  sync.Mutex
	now model.Epoch
}

func NewFake(start model.Epoch) *Fake { return &Fake{now: start} }

func (f *Fake) Now() model.Epoch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) Advance(d model.Epoch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now += d
}

func (f *Fake) Set(t model.Epoch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
