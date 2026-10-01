// Package usecases holds the edge's resilience rules.
package usecases

import (
	"sync"
	"time"
)

// Breaker is a per-upstream circuit breaker.
//
// Closed, it passes everything. After Threshold consecutive failures it
// opens and refuses at once for Cooldown -- a service that is down should
// not also be queued up for by every client until each one times out. When
// the cooldown ends it lets exactly one request through: success closes it,
// failure opens it again.
//
// A failure is the upstream being unreachable or answering 502/503/504. An
// upstream that answers 4xx or 500 is up and deciding; that is not the
// edge's business.
type Breaker struct {
	Threshold int
	Cooldown  time.Duration
	Now       func() time.Time

	mu        sync.Mutex
	failures  int
	openUntil time.Time
	probing   bool
}

// NewBreaker builds a breaker with defaults for zero values.
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &Breaker{Threshold: threshold, Cooldown: cooldown, Now: time.Now}
}

// Allow reports whether a request may go through, and when a refused caller
// may retry.
func (b *Breaker) Allow() (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.Now()
	if b.failures < b.Threshold {
		return true, 0
	}
	if now.Before(b.openUntil) {
		return false, b.openUntil.Sub(now)
	}
	if b.probing {
		// Half-open: one probe is already in flight.
		return false, time.Second
	}
	b.probing = true
	return true, 0
}

// Success records a healthy answer and closes the breaker.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures, b.probing = 0, false
}

// Failure records an unhealthy answer, opening the breaker at the threshold.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.probing = false
	if b.failures >= b.Threshold {
		b.openUntil = b.Now().Add(b.Cooldown)
	}
}

// Open reports whether the breaker is refusing requests right now.
func (b *Breaker) Open() bool {
	allowedNow := false
	b.mu.Lock()
	if b.failures < b.Threshold || (!b.Now().Before(b.openUntil) && !b.probing) {
		allowedNow = true
	}
	b.mu.Unlock()
	return !allowedNow
}
