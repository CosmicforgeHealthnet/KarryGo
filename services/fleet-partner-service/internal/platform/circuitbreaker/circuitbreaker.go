// Package circuitbreaker implements a per-dependency circuit breaker.
//
// One Breaker instance guards exactly one dependency/operation class (a
// downstream service name, or a service+operation pair when a caller wants
// finer isolation) — there is deliberately no shared global breaker across
// unrelated services, so one degraded downstream cannot trip protection for
// every other one. State is in-memory only: a process restart resets every
// breaker to CLOSED, which is the safe default and needs no durable
// persistence (see config.Config's own doc comment on the Section 29
// circuit-breaker fields for why this is the deliberate choice).
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State is one of the three standard circuit states.
type State string

const (
	Closed   State = "CLOSED"
	Open     State = "OPEN"
	HalfOpen State = "HALF_OPEN"
)

// ErrOpen is returned by Allow when the breaker is refusing calls.
var ErrOpen = errors.New("circuitbreaker: circuit is open")

// Config tunes one breaker.
type Config struct {
	// FailureThreshold is how many classified failures within
	// ObservationWindow trip the breaker from CLOSED to OPEN.
	FailureThreshold int
	// ObservationWindow bounds how far back failures are counted; a failure
	// older than this is no longer held against the dependency.
	ObservationWindow time.Duration
	// OpenDuration is how long the breaker stays OPEN before allowing a
	// bounded number of HALF_OPEN probes.
	OpenDuration time.Duration
	// HalfOpenProbes bounds how many trial calls are allowed through while
	// HALF_OPEN before another failure re-opens the circuit.
	HalfOpenProbes int
	// SuccessThreshold is how many consecutive HALF_OPEN successes are
	// required to fully close the circuit again.
	SuccessThreshold int
}

func (c Config) normalized() Config {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.ObservationWindow <= 0 {
		c.ObservationWindow = 30 * time.Second
	}
	if c.OpenDuration <= 0 {
		c.OpenDuration = 30 * time.Second
	}
	if c.HalfOpenProbes <= 0 {
		c.HalfOpenProbes = 1
	}
	if c.SuccessThreshold <= 0 {
		c.SuccessThreshold = 1
	}
	return c
}

// Transition is reported to an optional observer on every state change, for
// structured logging and metrics — never invoked with tenant or resource
// identifiers, only the breaker's own bounded name.
type Transition struct {
	Name string
	From State
	To   State
}

// Breaker guards one dependency/operation class.
type Breaker struct {
	name         string
	cfg          Config
	onTransition func(Transition)

	mu               sync.Mutex
	state            State
	failures         []time.Time // failure timestamps within the observation window
	openedAt         time.Time
	halfOpenInFlight int
	halfOpenSuccess  int
}

// New builds a breaker for one bounded, low-cardinality name (a downstream
// service name, or "service.operation" — never a raw ID or URL).
func New(name string, cfg Config, onTransition func(Transition)) *Breaker {
	return &Breaker{name: name, cfg: cfg.normalized(), state: Closed, onTransition: onTransition}
}

// Name returns the breaker's bounded identity.
func (b *Breaker) Name() string { return b.name }

// State reports the current state (for readiness/metrics reads).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentStateLocked(time.Now())
}

// Allow reports whether a call may proceed. OPEN refuses every call until
// OpenDuration elapses; HALF_OPEN allows up to HalfOpenProbes concurrent
// trial calls and refuses the rest so a recovering dependency is not
// immediately re-flooded.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	state := b.currentStateLocked(now)
	switch state {
	case Open:
		return ErrOpen
	case HalfOpen:
		if b.halfOpenInFlight >= b.cfg.HalfOpenProbes {
			return ErrOpen
		}
		b.halfOpenInFlight++
		return nil
	default:
		return nil
	}
}

// currentStateLocked resolves OPEN -> HALF_OPEN once OpenDuration has
// elapsed. Caller must hold mu.
func (b *Breaker) currentStateLocked(now time.Time) State {
	if b.state == Open && now.Sub(b.openedAt) >= b.cfg.OpenDuration {
		b.setStateLocked(HalfOpen)
		b.halfOpenInFlight = 0
		b.halfOpenSuccess = 0
	}
	return b.state
}

// RecordSuccess reports a classified success. In HALF_OPEN, SuccessThreshold
// consecutive successes close the circuit; in CLOSED it simply prunes old
// failure timestamps.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	switch b.currentStateLocked(now) {
	case HalfOpen:
		b.halfOpenInFlight--
		if b.halfOpenInFlight < 0 {
			b.halfOpenInFlight = 0
		}
		b.halfOpenSuccess++
		if b.halfOpenSuccess >= b.cfg.SuccessThreshold {
			b.setStateLocked(Closed)
			b.failures = nil
		}
	case Closed:
		b.pruneLocked(now)
	}
}

// RecordFailure reports a classified failure (transient/permanent — never a
// caller validation error, authorization denial or business conflict; those
// are not dependency failures and must not be passed here). In HALF_OPEN any
// failure immediately re-opens the circuit; in CLOSED, FailureThreshold
// failures within ObservationWindow trip it open.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	switch b.currentStateLocked(now) {
	case HalfOpen:
		b.halfOpenInFlight--
		if b.halfOpenInFlight < 0 {
			b.halfOpenInFlight = 0
		}
		b.openedAt = now
		b.setStateLocked(Open)
		b.failures = nil
	case Closed:
		b.pruneLocked(now)
		b.failures = append(b.failures, now)
		if len(b.failures) >= b.cfg.FailureThreshold {
			b.openedAt = now
			b.setStateLocked(Open)
			b.failures = nil
		}
	}
}

func (b *Breaker) pruneLocked(now time.Time) {
	cutoff := now.Add(-b.cfg.ObservationWindow)
	kept := b.failures[:0]
	for _, t := range b.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	b.failures = kept
}

func (b *Breaker) setStateLocked(to State) {
	if b.state == to {
		return
	}
	from := b.state
	b.state = to
	if b.onTransition != nil {
		b.onTransition(Transition{Name: b.name, From: from, To: to})
	}
}

// Registry holds one Breaker per bounded name, created on first use.
type Registry struct {
	mu       sync.Mutex
	cfg      Config
	onEvent  func(Transition)
	breakers map[string]*Breaker
}

// NewRegistry builds a registry sharing one Config/observer across every
// breaker it creates.
func NewRegistry(cfg Config, onTransition func(Transition)) *Registry {
	return &Registry{cfg: cfg.normalized(), onEvent: onTransition, breakers: map[string]*Breaker{}}
}

// Get returns the named breaker, creating it on first access.
func (r *Registry) Get(name string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.breakers[name]; ok {
		return b
	}
	b := New(name, r.cfg, r.onEvent)
	r.breakers[name] = b
	return b
}

// States returns a snapshot of every known breaker's state, for a readiness
// or diagnostics read. Names are the same bounded identities passed to Get.
func (r *Registry) States() map[string]State {
	r.mu.Lock()
	names := make([]*Breaker, 0, len(r.breakers))
	for _, b := range r.breakers {
		names = append(names, b)
	}
	r.mu.Unlock()
	out := make(map[string]State, len(names))
	for _, b := range names {
		out[b.Name()] = b.State()
	}
	return out
}

// Do runs fn guarded by the named breaker: refuses immediately with ErrOpen
// when the circuit will not allow the call, otherwise runs fn and records
// the outcome via classify. classify must return true only for a genuine
// dependency failure (transient/permanent/unknown) — never for a caller
// validation error, an authorization denial or a normal business conflict.
func (r *Registry) Do(ctx context.Context, name string, fn func(context.Context) error, classify func(error) bool) error {
	b := r.Get(name)
	if err := b.Allow(); err != nil {
		return err
	}
	err := fn(ctx)
	if err != nil && classify(err) {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}
	return err
}
