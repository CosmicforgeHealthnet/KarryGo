// Package bookingtelemetry owns the active-trip location trail (phase B6,
// addendum §8): batched GPS ingestion, server-side integrity classification,
// and the durable, append-only store those points land in.
//
// A flagged point is still stored -- the trail is a durable audit record,
// never a filtered "what we chose to believe" view. Classification only
// decides two downstream questions: whether a point is trusted enough to
// become the driver's LIVE tracked position (see PositionUpdater in
// internal/features/booking/usecases/telemetry.go), and whether it counts
// toward the server-computed actual distance (distance.go) used to reconcile
// the final fare. Nothing here ever rejects a point outright.
package bookingtelemetry

import "time"

// IntegrityFlag is one of the addendum §8 signals computed server-side for
// an incoming point. There is deliberately no "spoof_suspected" flag here,
// despite the column allowing arbitrary text values and the migration's own
// comment listing it as a possibility: no spoof-detection signal exists
// anywhere in this platform to compute it from, and inventing one would be
// exactly the kind of fake implementation the build plan's exclusion list
// (RideGuard/fraud scoring, phase B12+) explicitly defers.
type IntegrityFlag string

const (
	// FlagImpossibleJump means the implied speed between this point and the
	// immediately preceding STORED point (regardless of that point's own
	// flags -- see Classify's doc comment) exceeds Config.MaxSpeedKph.
	FlagImpossibleJump IntegrityFlag = "impossible_jump"
	// FlagStale means RecordedAt is further behind the ingest time than
	// Config.StaleThreshold allows -- the fix is real but arrived too late
	// to be trusted as "where the driver is right now".
	FlagStale IntegrityFlag = "stale"
	// FlagDuplicate means this exact (booking_id, provider_id, sequence)
	// triple was already stored -- a client resend, not a new fix.
	FlagDuplicate IntegrityFlag = "duplicate"
	// FlagOutOfOrder means this point's sequence is lower than one already
	// stored for the same booking+provider -- a late-arriving older fix.
	FlagOutOfOrder IntegrityFlag = "out_of_order"
)

// Point is one GPS fix in a trip's location trail, mirroring
// taxi_trip_location_points exactly.
type Point struct {
	ID             string
	BookingID      string
	ProviderID     string
	Sequence       int64
	Lat            float64
	Lng            float64
	AccuracyM      *float64
	SpeedMps       *float64
	HeadingDeg     *float64
	RecordedAt     time.Time
	ReceivedAt     time.Time
	Source         string
	IntegrityFlags []string
	ClientEventID  string
}

// Clean reports whether this point carries no integrity flags at all -- the
// only points trusted for live tracking or distance reconciliation.
func (p Point) Clean() bool { return len(p.IntegrityFlags) == 0 }
