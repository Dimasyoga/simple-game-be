package room

import "time"

// Clock is the server-authoritative source of time for barriers and claim
// windows. Never the client's — contract.md: the client only ever renders a
// countdown toward a deadline this package issues.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

// RealClock is the production Clock, backed by the OS clock.
func RealClock() Clock { return realClock{} }

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
