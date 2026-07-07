package room

import (
	"sync"
	"time"
)

// fakeClock lets tests deterministically trigger "the deadline has passed"
// instead of sleeping real wall-clock time. Every channel handed out by
// After since the last Fire receives the new instant when Fire is called.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	f.mu.Lock()
	f.pending = append(f.pending, ch)
	f.mu.Unlock()
	return ch
}

// Fire delivers the current instant to every channel registered since the
// last Fire, simulating every outstanding deadline elapsing at once.
func (f *fakeClock) Fire() {
	f.mu.Lock()
	pending := f.pending
	f.pending = nil
	f.now = f.now.Add(time.Hour)
	now := f.now
	f.mu.Unlock()

	for _, ch := range pending {
		ch <- now
	}
}
