package routing

import (
	"testing"
	"time"
)

func TestBreaker_StartsClosedAndAllowsCalls(t *testing.T) {
	b := NewBreaker(3, time.Second)
	if !b.Allow() {
		t.Fatal("a fresh breaker should allow calls")
	}
	if b.IsOpen() {
		t.Fatal("a fresh breaker should not be open")
	}
}

func TestBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	b := NewBreaker(3, time.Second)
	b.Failure()
	b.Failure()
	if b.IsOpen() {
		t.Fatal("should still be closed before the threshold is reached")
	}
	b.Failure() // 3rd consecutive failure
	if !b.IsOpen() {
		t.Fatal("should be open once the failure threshold is reached")
	}
	if b.Allow() {
		t.Fatal("an open breaker within its cooldown must refuse calls")
	}
}

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := NewBreaker(3, time.Second)
	b.Failure()
	b.Failure()
	b.Success() // resets before the threshold is reached
	b.Failure()
	b.Failure()
	if b.IsOpen() {
		t.Fatal("the counter should have reset on success; two more failures should not open it")
	}
}

func TestBreaker_HalfOpensAfterCooldownAndAllowsOneProbe(t *testing.T) {
	fakeNow := time.Now()
	b := NewBreaker(1, time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Failure() // opens immediately (threshold 1)
	if !b.IsOpen() {
		t.Fatal("should be open after a single failure with threshold 1")
	}
	if b.Allow() {
		t.Fatal("should refuse calls before the cooldown elapses")
	}

	fakeNow = fakeNow.Add(2 * time.Second) // past the 1s cooldown
	if !b.Allow() {
		t.Fatal("should allow exactly one probe call once the cooldown has elapsed")
	}
}

func TestBreaker_FailedProbeReopensImmediately(t *testing.T) {
	fakeNow := time.Now()
	b := NewBreaker(1, time.Second)
	b.now = func() time.Time { return fakeNow }

	b.Failure() // open
	fakeNow = fakeNow.Add(2 * time.Second)
	if !b.Allow() {
		t.Fatal("expected the cooldown probe to be allowed")
	}

	// The probe itself fails. A dependency that fails its very first call
	// after a cooldown has not actually recovered -- this must reopen
	// immediately, not require the full threshold to be hit again.
	b.Failure()
	if !b.IsOpen() {
		t.Fatal("a failed probe must reopen the breaker")
	}
	if b.Allow() {
		t.Fatal("should refuse calls again immediately after a failed probe")
	}
}

func TestBreaker_SuccessfulProbeClosesImmediately(t *testing.T) {
	fakeNow := time.Now()
	b := NewBreaker(5, time.Second) // higher threshold: proves ONE success is enough, not five
	b.now = func() time.Time { return fakeNow }

	for i := 0; i < 5; i++ {
		b.Failure()
	}
	if !b.IsOpen() {
		t.Fatal("should be open after reaching the threshold")
	}

	fakeNow = fakeNow.Add(2 * time.Second)
	if !b.Allow() {
		t.Fatal("expected the cooldown probe to be allowed")
	}
	b.Success()
	if b.IsOpen() {
		t.Fatal("a single successful probe should close the breaker, not require it to accumulate successes")
	}
	if !b.Allow() {
		t.Fatal("a closed breaker should allow calls")
	}
}
