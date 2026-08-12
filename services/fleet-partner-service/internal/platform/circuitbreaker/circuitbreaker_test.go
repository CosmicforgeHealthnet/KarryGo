package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		FailureThreshold: 3, ObservationWindow: time.Minute,
		OpenDuration: 20 * time.Millisecond, HalfOpenProbes: 1, SuccessThreshold: 2,
	}
}

func TestBreaker_ClosedUntilFailureThreshold(t *testing.T) {
	b := New("test", testConfig(), nil)
	for i := 0; i < 2; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("call %d: unexpected refusal: %v", i, err)
		}
		b.RecordFailure()
	}
	if b.State() != Closed {
		t.Fatalf("state = %s, want CLOSED before threshold", b.State())
	}
	if err := b.Allow(); err != nil {
		t.Fatal("third call should still be allowed")
	}
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("state = %s, want OPEN at threshold", b.State())
	}
}

func TestBreaker_OpenRefusesUntilOpenDurationElapses(t *testing.T) {
	b := New("test", testConfig(), nil)
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("Allow() = %v, want ErrOpen while OPEN", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() after OpenDuration elapsed = %v, want nil (HALF_OPEN probe)", err)
	}
	if b.State() != HalfOpen {
		t.Fatalf("state = %s, want HALF_OPEN", b.State())
	}
}

func TestBreaker_HalfOpenBoundsConcurrentProbes(t *testing.T) {
	cfg := testConfig()
	cfg.HalfOpenProbes = 1
	b := New("test", cfg, nil)
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	time.Sleep(30 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Fatalf("first probe should be allowed: %v", err)
	}
	if err := b.Allow(); !errors.Is(err, ErrOpen) {
		t.Fatalf("second concurrent probe should be refused, got %v", err)
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := New("test", testConfig(), nil)
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	time.Sleep(30 * time.Millisecond)
	_ = b.Allow()
	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("a HALF_OPEN failure must re-open the circuit, got %s", b.State())
	}
}

func TestBreaker_HalfOpenSuccessThresholdCloses(t *testing.T) {
	cfg := testConfig()
	cfg.SuccessThreshold = 2
	b := New("test", cfg, nil)
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	time.Sleep(30 * time.Millisecond)

	_ = b.Allow()
	b.RecordSuccess()
	if b.State() != HalfOpen {
		t.Fatalf("state after 1 of 2 successes = %s, want still HALF_OPEN", b.State())
	}
	_ = b.Allow()
	b.RecordSuccess()
	if b.State() != Closed {
		t.Fatalf("state after success threshold met = %s, want CLOSED", b.State())
	}
}

func TestBreaker_FailuresOutsideWindowDoNotAccumulate(t *testing.T) {
	cfg := testConfig()
	cfg.ObservationWindow = 15 * time.Millisecond
	b := New("test", cfg, nil)
	_ = b.Allow()
	b.RecordFailure()
	_ = b.Allow()
	b.RecordFailure()
	time.Sleep(25 * time.Millisecond) // both failures age out of the window
	_ = b.Allow()
	b.RecordFailure()
	if b.State() != Closed {
		t.Fatalf("state = %s, want CLOSED — earlier failures should have aged out", b.State())
	}
}

func TestBreaker_TransitionsAreReported(t *testing.T) {
	var transitions []Transition
	b := New("test-dep", testConfig(), func(tr Transition) { transitions = append(transitions, tr) })
	for i := 0; i < 3; i++ {
		_ = b.Allow()
		b.RecordFailure()
	}
	if len(transitions) != 1 || transitions[0].From != Closed || transitions[0].To != Open {
		t.Fatalf("expected one CLOSED->OPEN transition, got %+v", transitions)
	}
	if transitions[0].Name != "test-dep" {
		t.Fatalf("transition name = %s, want the breaker's own bounded name", transitions[0].Name)
	}
}

func TestRegistry_IndependentPerName(t *testing.T) {
	reg := NewRegistry(testConfig(), nil)
	wallet := reg.Get("payment-wallet-service")
	media := reg.Get("media-file-service")
	for i := 0; i < 3; i++ {
		_ = wallet.Allow()
		wallet.RecordFailure()
	}
	if wallet.State() != Open {
		t.Fatal("wallet breaker should be OPEN")
	}
	if media.State() != Closed {
		t.Fatal("an unrelated dependency's breaker must not be tripped by another dependency's failures")
	}
	if reg.Get("payment-wallet-service") != wallet {
		t.Fatal("Get must return the same breaker instance for the same name")
	}
}

func TestRegistry_Do_ClassifiesFailuresOnly(t *testing.T) {
	reg := NewRegistry(testConfig(), nil)
	validationErr := errors.New("validation failed")
	classify := func(err error) bool { return err != nil && err != validationErr }

	// Three "validation" errors must never trip the breaker.
	for i := 0; i < 5; i++ {
		err := reg.Do(context.Background(), "svc", func(context.Context) error { return validationErr }, classify)
		if err != validationErr {
			t.Fatalf("Do returned %v, want the underlying validation error", err)
		}
	}
	if reg.Get("svc").State() != Closed {
		t.Fatal("caller validation errors must not count as dependency failures")
	}

	// Three genuine failures do trip it.
	depErr := errors.New("dependency unavailable")
	for i := 0; i < 3; i++ {
		_ = reg.Do(context.Background(), "svc", func(context.Context) error { return depErr }, classify)
	}
	if reg.Get("svc").State() != Open {
		t.Fatal("genuine dependency failures must trip the breaker")
	}

	// Now Do must refuse without even calling fn.
	called := false
	err := reg.Do(context.Background(), "svc", func(context.Context) error { called = true; return nil }, classify)
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("Do() while OPEN = %v, want ErrOpen", err)
	}
	if called {
		t.Fatal("fn must not run while the breaker refuses the call")
	}
}

func TestRegistry_States(t *testing.T) {
	reg := NewRegistry(testConfig(), nil)
	reg.Get("a")
	reg.Get("b")
	states := reg.States()
	if len(states) != 2 || states["a"] != Closed || states["b"] != Closed {
		t.Fatalf("unexpected states snapshot: %+v", states)
	}
}
