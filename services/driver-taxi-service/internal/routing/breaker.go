package routing

import (
	"sync"
	"time"
)

// breakerState is the classic three-state circuit breaker machine. No
// external circuit-breaker package exists anywhere in this monorepo
// (confirmed by search before writing this), so this is a small, self
// contained implementation rather than a new third-party dependency for
// something this simple.
type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// DefaultFailureThreshold is how many consecutive failures open the breaker.
const DefaultFailureThreshold = 5

// DefaultCooldown is how long the breaker stays open before allowing a single
// probe call through.
const DefaultCooldown = 30 * time.Second

// Breaker is a consecutive-failure circuit breaker guarding one downstream
// dependency (the Google Routes API, for this package's purposes). It is
// intentionally generic over nothing -- callers decide what "a failure"
// means for their own call and report it via Success/Failure.
type Breaker struct {
	mu               sync.Mutex
	state            breakerState
	failures         int
	failureThreshold int
	cooldown         time.Duration
	openedAt         time.Time
	now              func() time.Time
}

// NewBreaker builds a breaker with the given threshold and cooldown. A
// threshold or cooldown <= 0 uses the package defaults.
func NewBreaker(failureThreshold int, cooldown time.Duration) *Breaker {
	if failureThreshold <= 0 {
		failureThreshold = DefaultFailureThreshold
	}
	if cooldown <= 0 {
		cooldown = DefaultCooldown
	}
	return &Breaker{
		failureThreshold: failureThreshold,
		cooldown:         cooldown,
		now:              time.Now,
	}
}

// Allow reports whether a call should be attempted right now. When the
// breaker is OPEN and the cooldown has elapsed, this call transitions it to
// HALF_OPEN and becomes the probe -- the caller MUST report the outcome via
// Success or Failure, since that outcome decides whether the breaker closes
// again or reopens.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case breakerClosed, breakerHalfOpen:
		return true
	case breakerOpen:
		if b.now().Sub(b.openedAt) >= b.cooldown {
			b.state = breakerHalfOpen
			return true
		}
		return false
	default:
		return true
	}
}

// Success records a successful call. From HALF_OPEN this closes the breaker
// immediately (one good probe is enough to trust the dependency again); from
// CLOSED it resets the failure count.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = breakerClosed
}

// Failure records a failed call. A failed HALF_OPEN probe reopens the breaker
// immediately without waiting for the full threshold again -- a dependency
// that fails its very first probe after a cooldown has not actually
// recovered. From CLOSED, the breaker only opens once failureThreshold
// consecutive failures have been recorded.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == breakerHalfOpen {
		b.state = breakerOpen
		b.openedAt = b.now()
		b.failures = b.failureThreshold
		return
	}

	b.failures++
	if b.failures >= b.failureThreshold {
		b.state = breakerOpen
		b.openedAt = b.now()
	}
}

// IsOpen reports whether the breaker is currently refusing calls. Exposed
// mainly for tests and observability; production call sites should use Allow.
func (b *Breaker) IsOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == breakerOpen && b.now().Sub(b.openedAt) >= b.cooldown {
		return false
	}
	return b.state == breakerOpen
}
