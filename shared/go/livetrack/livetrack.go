// Package livetrack stores and fans out live vehicle positions. It is the
// Redis half of the tracking stack: shared/go/geo owns the maths, this package
// owns the keys, TTLs, spatial index and pub/sub fan-out, and each service
// owns its own Redis instance and its own HTTP surface.
//
// It generalises the location store that driver-dispatch-delivery-service grew
// first, with two deliberate changes:
//
//   - proximity queries read a geohash cell index rather than a single Redis
//     GEO set. Cell membership is scored by fix time and pruned on every read,
//     so the index cannot drift out of sync with the fix TTL the way an
//     untimed GEO set does; and
//   - a nearby query costs two pipelined round trips regardless of how many
//     candidates it returns, rather than two sequential reads per candidate.
//
// Every method is nil-safe: a zero store degrades to a no-op so a service can
// boot without Redis.
package livetrack

import (
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

const (
	// DefaultFixTTL is how long a position stays readable and discoverable
	// after the device reported it. It matches the availability TTLs the
	// services already use.
	DefaultFixTTL = 90 * time.Second

	// DefaultMaxCandidates caps how many entity ids one nearby query will
	// hydrate, bounding the work a dense city cell can cause.
	DefaultMaxCandidates = 500

	// DefaultNearbyLimit is applied when a query does not set one.
	DefaultNearbyLimit = 50
)

// Update types. These strings are on the wire to already-shipped clients (the
// dispatch rider app and the partner API's tracking websocket), so they are
// held fixed rather than renamed to match this package's own vocabulary.
const (
	UpdateTypeLocation = "location_update"
	UpdateTypeOffline  = "provider_offline"
)

// Fix is one position report. The field set is deliberately the same one
// fleet_partner_tracking_projections stores, so forwarding fixes to Fleet's
// existing ingest endpoint is a field-for-field mapping rather than a new
// contract.
type Fix struct {
	EntityID string  `json:"entity_id"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	// Heading is the compass bearing in degrees, [0, 360). Zero means the
	// device did not report one.
	Heading float64 `json:"heading"`
	// Speed is ground speed in metres per second.
	Speed float64 `json:"speed"`
	// Accuracy is the reported horizontal accuracy in metres.
	Accuracy float64 `json:"accuracy"`
	// RecordedAt is when the device took the fix; ReceivedAt is when the
	// service accepted it. Keeping both makes a delayed batch of fixes
	// distinguishable from a stalled device.
	RecordedAt time.Time `json:"recorded_at"`
	ReceivedAt time.Time `json:"received_at"`
	// Source is the owning service name, so a downstream consumer can tell
	// a hauling fix from a dispatch one.
	Source string `json:"source"`
}

// Point returns the fix position.
func (f Fix) Point() geo.Point { return geo.Point{Lat: f.Lat, Lng: f.Lng} }

// HasFix reports whether the coordinate is usable.
func (f Fix) HasFix() bool { return f.Point().HasFix() }

// Age returns how long ago the device recorded the fix.
func (f Fix) Age(now time.Time) time.Duration { return now.Sub(f.RecordedAt) }

// IsStale reports whether the fix is older than ttl.
func (f Fix) IsStale(now time.Time, ttl time.Duration) bool { return f.Age(now) > ttl }

// Update is the JSON pushed to websocket subscribers on each fix.
//
// ProviderID duplicates EntityID. The dispatch rider app and the partner API
// tracking socket already read `provider_id`, so it stays on the wire; new
// consumers should read `entity_id`.
type Update struct {
	Type       string    `json:"type"`
	EntityID   string    `json:"entity_id"`
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Heading    float64   `json:"heading"`
	Speed      float64   `json:"speed"`
	Accuracy   float64   `json:"accuracy"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// LocationUpdate builds the wire message for a fix.
func LocationUpdate(f Fix) Update {
	return Update{
		Type:       UpdateTypeLocation,
		EntityID:   f.EntityID,
		ProviderID: f.EntityID,
		Lat:        f.Lat,
		Lng:        f.Lng,
		Heading:    f.Heading,
		Speed:      f.Speed,
		Accuracy:   f.Accuracy,
		UpdatedAt:  f.RecordedAt,
	}
}

// OfflineUpdate builds the wire message sent when an entity stops reporting.
func OfflineUpdate(entityID string) Update {
	return Update{Type: UpdateTypeOffline, EntityID: entityID, ProviderID: entityID}
}

// NearbyQuery asks for the entities closest to a point.
type NearbyQuery struct {
	Center   geo.Point
	RadiusKm float64
	Limit    int
}

// NearbyEntity is one proximity result, nearest first.
type NearbyEntity struct {
	Fix        Fix     `json:"fix"`
	DistanceKm float64 `json:"distance_km"`
}

// Config tunes one service's tracking namespace.
type Config struct {
	// Namespace prefixes every key, so several services can share one Redis
	// without colliding. Required.
	Namespace string
	// FixTTL is how long a fix stays readable and indexed.
	FixTTL time.Duration
	// CellPrecisions are the geohash precisions the index is written at. A
	// query picks whichever of them covers its radius in the fewest cells
	// while staying under MaxCoverCells. Coarse-first ordering is not
	// required.
	CellPrecisions []int
	// MaxCoverCells caps how many cells one query may read.
	MaxCoverCells int
	// MaxCandidates caps how many entity ids one query hydrates.
	MaxCandidates int
	// ChannelKeyFunc overrides where updates are published. It exists for
	// services that already shipped a pub/sub channel name to clients: they
	// keep publishing on the key their apps subscribe to instead of moving
	// to this package's namespaced default. Leave nil for new services.
	ChannelKeyFunc func(entityID string) string
}

// WithDefaults fills unset fields. It is exported so services can log the
// effective configuration.
func (c Config) WithDefaults() Config {
	if c.FixTTL <= 0 {
		c.FixTTL = DefaultFixTTL
	}
	if len(c.CellPrecisions) == 0 {
		c.CellPrecisions = []int{geo.CoarsePrecision, geo.FinePrecision}
	}
	if c.MaxCoverCells <= 0 {
		c.MaxCoverCells = geo.DefaultMaxCoverCells
	}
	if c.MaxCandidates <= 0 {
		c.MaxCandidates = DefaultMaxCandidates
	}
	return c
}

// IndexPrecision returns the finest configured precision, which is the one
// entity cells are recorded at. Every coarser indexed precision is a prefix of
// it, which is why only one cell per entity has to be remembered.
func (c Config) IndexPrecision() int {
	finest := 0
	for _, p := range c.CellPrecisions {
		if p > finest {
			finest = p
		}
	}
	if finest == 0 {
		return geo.FinePrecision
	}
	return finest
}
