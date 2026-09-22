package usecases

import (
	"testing"
	"time"
)

func TestTheBreakerOpensCoolsAndProbesOnce(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(2, time.Minute)
	b.Now = func() time.Time { return now }
	b.Failure()
	if ok, _ := b.Allow(); !ok {
		t.Fatal("one failure must not open the breaker")
	}
	b.Failure()
	if ok, wait := b.Allow(); ok || wait <= 0 {
		t.Fatal("the threshold must open the breaker with a retry hint")
	}
	now = now.Add(2 * time.Minute)
	if ok, _ := b.Allow(); !ok {
		t.Fatal("after the cooldown one probe must be allowed")
	}
	if ok, _ := b.Allow(); ok {
		t.Fatal("only one probe may be in flight")
	}
	b.Success()
	if ok, _ := b.Allow(); !ok || b.Open() {
		t.Fatal("a successful probe must close the breaker")
	}
}

func TestAFailedProbeReopens(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(1, time.Minute)
	b.Now = func() time.Time { return now }
	b.Failure()
	now = now.Add(2 * time.Minute)
	b.Allow()
	b.Failure()
	if ok, _ := b.Allow(); ok {
		t.Fatal("a failed probe must reopen the breaker")
	}
}
