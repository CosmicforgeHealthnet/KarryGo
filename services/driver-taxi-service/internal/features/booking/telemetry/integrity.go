package bookingtelemetry

import (
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// Config tunes classification thresholds. Sourced from cmd/main.go's
// env-driven service config, never a hard-coded constant here, matching
// every other tunable in this service (see MatchingConfig's own doc
// comment for the same convention).
type Config struct {
	// MaxSpeedKph is the implied speed above which two consecutive points
	// are flagged FlagImpossibleJump.
	MaxSpeedKph float64
	// StaleThreshold is how far RecordedAt may lag the ingest time before a
	// point is flagged FlagStale.
	StaleThreshold time.Duration
}

// Classify computes the integrity flags for one incoming point against the
// last point ALREADY STORED for this booking+provider ('prev'; nil for the
// first point of a trip). 'now' is the ingest time (received_at), passed in
// rather than read here so classification stays deterministic and testable.
//
// prev is used as-is regardless of whether IT was itself flagged: a flagged
// point is still real, still stored, and still the most recent thing this
// driver's device actually reported. Walking back further to find the last
// UNFLAGGED point would let one bad fix mask an otherwise-valid subsequent
// one, or silently paper over a genuinely broken device -- this build keeps
// the rule simple and honest instead: compare against whatever was received
// immediately before.
func Classify(prev *Point, next Point, now time.Time, cfg Config) []string {
	var flags []string

	if cfg.StaleThreshold > 0 && now.Sub(next.RecordedAt) > cfg.StaleThreshold {
		flags = append(flags, string(FlagStale))
	}

	if prev == nil {
		return flags
	}

	switch {
	case next.Sequence == prev.Sequence:
		flags = append(flags, string(FlagDuplicate))
	case next.Sequence < prev.Sequence:
		flags = append(flags, string(FlagOutOfOrder))
	default:
		if cfg.MaxSpeedKph > 0 {
			elapsed := next.RecordedAt.Sub(prev.RecordedAt)
			distanceKm := geo.HaversineKm(geo.Point{Lat: prev.Lat, Lng: prev.Lng}, geo.Point{Lat: next.Lat, Lng: next.Lng})
			switch {
			case elapsed > 0:
				impliedSpeedKph := distanceKm / elapsed.Hours()
				if impliedSpeedKph > cfg.MaxSpeedKph {
					flags = append(flags, string(FlagImpossibleJump))
				}
			case distanceKm > materialDistanceKm:
				// elapsed <= 0 with an increasing sequence means the two
				// RecordedAt timestamps disagree with sequence order (clock
				// skew, or a batch of buffered points the device stamped
				// with one coarse timestamp) -- but zero-or-negative time
				// covering a REAL distance is not "unknowable speed", it is
				// an infinite one: strictly worse than any finite speed
				// over MaxSpeedKph. Found live: a batch where every point
				// shared one timestamp let a wildly wrong fix (thousands of
				// km off) sail through unflagged and corrupt the reconciled
				// trip distance, because this branch used to skip the
				// distance check entirely whenever elapsed was non-positive.
				// materialDistanceKm tolerates GPS jitter on two fixes that
				// are genuinely the same instant and the same place.
				flags = append(flags, string(FlagImpossibleJump))
			}
		}
	}

	return flags
}

// materialDistanceKm is the smallest movement, between two points sharing
// (or disagreeing on) a timestamp, treated as a real position change rather
// than GPS/rounding jitter -- 50 meters.
const materialDistanceKm = 0.05
