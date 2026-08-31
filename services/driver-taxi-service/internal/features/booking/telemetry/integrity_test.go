package bookingtelemetry

import (
	"testing"
	"time"
)

func testConfig() Config {
	return Config{MaxSpeedKph: 180, StaleThreshold: 120 * time.Second}
}

func hasFlag(flags []string, want IntegrityFlag) bool {
	for _, f := range flags {
		if f == string(want) {
			return true
		}
	}
	return false
}

func TestClassify_FirstPointOfTripNoFlags(t *testing.T) {
	now := time.Now()
	next := Point{Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: now}
	flags := Classify(nil, next, now, testConfig())
	if len(flags) != 0 {
		t.Errorf("flags = %v, want none for the first point of a trip", flags)
	}
}

func TestClassify_CleanConsecutivePointNoFlags(t *testing.T) {
	now := time.Now()
	prev := Point{Sequence: 1, Lat: 6.5000, Lng: 3.4000, RecordedAt: now.Add(-10 * time.Second)}
	// ~100m in 10s = 36kph, well under the 180kph threshold.
	next := Point{Sequence: 2, Lat: 6.5009, Lng: 3.4000, RecordedAt: now}
	flags := Classify(&prev, next, now, testConfig())
	if len(flags) != 0 {
		t.Errorf("flags = %v, want none for a plausible consecutive fix", flags)
	}
}

func TestClassify_StaleWhenRecordedFarBehindNow(t *testing.T) {
	now := time.Now()
	next := Point{Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: now.Add(-10 * time.Minute)}
	flags := Classify(nil, next, now, testConfig())
	if !hasFlag(flags, FlagStale) {
		t.Errorf("flags = %v, want stale", flags)
	}
}

func TestClassify_NotStaleWithinThreshold(t *testing.T) {
	now := time.Now()
	next := Point{Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: now.Add(-30 * time.Second)}
	flags := Classify(nil, next, now, testConfig())
	if hasFlag(flags, FlagStale) {
		t.Errorf("flags = %v, want no stale flag within the threshold", flags)
	}
}

func TestClassify_DuplicateSequence(t *testing.T) {
	now := time.Now()
	prev := Point{Sequence: 5, Lat: 6.5, Lng: 3.4, RecordedAt: now.Add(-5 * time.Second)}
	next := Point{Sequence: 5, Lat: 6.5001, Lng: 3.4001, RecordedAt: now}
	flags := Classify(&prev, next, now, testConfig())
	if !hasFlag(flags, FlagDuplicate) {
		t.Errorf("flags = %v, want duplicate", flags)
	}
	if hasFlag(flags, FlagOutOfOrder) {
		t.Errorf("flags = %v, an exact sequence repeat must be duplicate, not out_of_order", flags)
	}
}

func TestClassify_OutOfOrderSequence(t *testing.T) {
	now := time.Now()
	prev := Point{Sequence: 5, Lat: 6.5, Lng: 3.4, RecordedAt: now.Add(-5 * time.Second)}
	next := Point{Sequence: 3, Lat: 6.5001, Lng: 3.4001, RecordedAt: now}
	flags := Classify(&prev, next, now, testConfig())
	if !hasFlag(flags, FlagOutOfOrder) {
		t.Errorf("flags = %v, want out_of_order", flags)
	}
}

func TestClassify_ImpossibleJump(t *testing.T) {
	now := time.Now()
	// Lagos to London in 10 seconds.
	prev := Point{Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: now.Add(-10 * time.Second)}
	next := Point{Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: now}
	flags := Classify(&prev, next, now, testConfig())
	if !hasFlag(flags, FlagImpossibleJump) {
		t.Errorf("flags = %v, want impossible_jump", flags)
	}
}

func TestClassify_ImpossibleJumpWhenElapsedIsNonPositiveButDistanceIsReal(t *testing.T) {
	now := time.Now()
	// next.RecordedAt <= prev.RecordedAt despite an increasing sequence --
	// device clock skew, or a buffered batch the client stamped with one
	// coarse timestamp. Found live: this used to be treated as "no
	// meaningful speed to compute" and silently skipped, which let a
	// wildly-wrong fix (thousands of km off, same timestamp as its
	// predecessor) sail into the validated trail undetected and corrupt the
	// reconciled trip distance. Zero-or-negative elapsed time covering a
	// REAL distance is not unknowable, it is an infinite implied speed --
	// strictly worse than any finite speed over the threshold.
	prev := Point{Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: now}
	next := Point{Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: now.Add(-1 * time.Second)}
	flags := Classify(&prev, next, now, testConfig())
	if !hasFlag(flags, FlagImpossibleJump) {
		t.Errorf("flags = %v, want impossible_jump for a huge distance even with elapsed <= 0", flags)
	}
}

func TestClassify_NoImpossibleJumpForJitterWhenElapsedIsNonPositive(t *testing.T) {
	now := time.Now()
	// Two fixes ~5m apart sharing (almost) the same instant -- ordinary GPS/
	// clock jitter on a stationary or near-stationary driver, not a real
	// position change. Must stay unflagged.
	prev := Point{Sequence: 1, Lat: 6.52440, Lng: 3.37920, RecordedAt: now}
	next := Point{Sequence: 2, Lat: 6.52444, Lng: 3.37920, RecordedAt: now.Add(-1 * time.Second)}
	flags := Classify(&prev, next, now, testConfig())
	if hasFlag(flags, FlagImpossibleJump) {
		t.Errorf("flags = %v, want no impossible_jump for sub-material jitter", flags)
	}
}

func TestClassify_StaleAndOutOfOrderCanBothApply(t *testing.T) {
	now := time.Now()
	prev := Point{Sequence: 5, Lat: 6.5, Lng: 3.4, RecordedAt: now.Add(-5 * time.Second)}
	next := Point{Sequence: 2, Lat: 6.5001, Lng: 3.4001, RecordedAt: now.Add(-10 * time.Minute)}
	flags := Classify(&prev, next, now, testConfig())
	if !hasFlag(flags, FlagStale) || !hasFlag(flags, FlagOutOfOrder) {
		t.Errorf("flags = %v, want both stale and out_of_order", flags)
	}
}

func TestClassify_ZeroThresholdsDisableTheirCheck(t *testing.T) {
	now := time.Now()
	cfg := Config{MaxSpeedKph: 0, StaleThreshold: 0}
	prev := Point{Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: now.Add(-1 * time.Hour)}
	next := Point{Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: now.Add(-1 * time.Hour)}
	flags := Classify(&prev, next, now, cfg)
	if len(flags) != 0 {
		t.Errorf("flags = %v, want none when both thresholds are configured off", flags)
	}
}
