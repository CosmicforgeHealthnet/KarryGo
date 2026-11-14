package models

import (
	"testing"
	"time"
)

func TestTrackingFreshnessAndGap(t *testing.T) {
	now := time.Now()
	if got := Freshness(now.Add(-time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Fresh {
		t.Fatal(got)
	}
	if got := Freshness(now.Add(-5*time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Aging {
		t.Fatal(got)
	}
	if got := Freshness(now.Add(-20*time.Minute), now, 2*time.Minute, 10*time.Minute, false); got != Stale {
		t.Fatal(got)
	}
	if got := Freshness(time.Time{}, now, 2*time.Minute, 10*time.Minute, false); got != Unknown {
		t.Fatal(got)
	}
	if got := Freshness(now, now, 2*time.Minute, 10*time.Minute, true); got != Gapped {
		t.Fatal(got)
	}
}

func TestProjectionKeyRoundTrips(t *testing.T) {
	source, trip, ok := SplitProjectionKey(ProjectionKey("driver-hauling-service", "trip|1"))
	if !ok || source != "driver-hauling-service" || trip != "trip|1" {
		t.Fatalf("split = %q %q %v", source, trip, ok)
	}
}
