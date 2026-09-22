package networkanalytics

// Section 13: supply snapshots, produced by the operational authorities.
//
// Each authority observes its OWN providers -- its availability rows and its
// live store -- and reduces them to counts per zone and product. Only the
// counts leave the service: no provider, vehicle or customer identity, and no
// coordinate finer than a zone cell. Analytics never queries an operational
// database and never infers supply from mission history.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

// SupplyZonePrecision is the geohash precision of a supply zone, about
// 4.9 km x 4.9 km: coarse enough that a count never locates a person, fine
// enough to tell one part of a city from another.
const SupplyZonePrecision = 5

// UnlocatedZone holds online providers the authority has no fresh position
// for. They are real supply; they are just not placeable.
const UnlocatedZone = "UNLOCATED"

// SupplyBucket is the snapshot cadence. One snapshot per zone and product per
// bucket, never one per GPS update.
const SupplyBucket = 5 * time.Minute

// Supply states, per snapshot.
const (
	SupplyAvailable = "AVAILABLE" // at least one eligible provider free
	SupplyLimited   = "LIMITED"   // online, but none free and eligible
	SupplyUnknown   = "UNKNOWN"   // eligibility could not be determined
)

// ZoneFor maps a position to its analytical zone id. A position without a fix
// is unlocated.
func ZoneFor(lat, lng float64) string {
	point := geo.Point{Lat: lat, Lng: lng}
	if !point.HasFix() {
		return UnlocatedZone
	}
	return "gh5:" + string(geo.Encode(point, SupplyZonePrecision))
}

// SupplyObservation is one provider as its authority sees it right now. It
// never leaves the service: BuildSupplyFacts reduces it to counts.
type SupplyObservation struct {
	Zone        string
	ProductCode string
	Online      bool
	// Occupied is nil when the authority does not know whether the provider
	// is on a job; that makes the zone's occupied count UNKNOWN, not zero.
	Occupied *bool
	// Eligible is nil when eligibility could not be evaluated -- the same
	// rule: unknown is not zero.
	Eligible *bool
}

// SupplyFact is the fact an authority publishes for one zone and product.
// Counts that could not be determined are nil and marshal as JSON null.
type SupplyFact struct {
	Authority     string    `json:"authority"`
	MarketID      string    `json:"market_id,omitempty"`
	Zone          string    `json:"zone"`
	ProductCode   string    `json:"product_code"`
	OnlineCount   int64     `json:"online_count"`
	EligibleCount *int64    `json:"eligible_count"`
	OccupiedCount *int64    `json:"occupied_count"`
	State         string    `json:"state"`
	ObservedAt    time.Time `json:"observed_at"`
	BucketStart   time.Time `json:"bucket_start"`
	PolicyVersion string    `json:"policy_version,omitempty"`
}

// EntityID is the snapshot's identity: one per zone and product. Its version
// is the bucket, so a later bucket supersedes an earlier one and a replayed
// bucket is a duplicate.
func (s SupplyFact) EntityID() string {
	return strings.Join([]string{s.Authority, s.MarketID, s.Zone, s.ProductCode}, "|")
}

// Version is the bucket start in seconds: monotonic, and identical for every
// replica that observes the same bucket.
func (s SupplyFact) Version() int64 { return s.BucketStart.Unix() }

// EventID is deterministic, so two replicas publishing the same bucket
// publish the same event.
func (s SupplyFact) EventID() string {
	sum := sha256.Sum256([]byte(s.EntityID() + "|" + s.BucketStart.UTC().Format(time.RFC3339)))
	return "supply-" + hex.EncodeToString(sum[:16])
}

// BucketStart truncates an instant to its supply bucket.
func BucketStart(now time.Time) time.Time { return now.UTC().Truncate(SupplyBucket) }

// BuildSupplyFacts reduces observations to one snapshot per zone and
// product. Only online providers are supply.
func BuildSupplyFacts(authority, marketID string, observations []SupplyObservation,
	now time.Time, policyVersion string) []SupplyFact {
	type key struct{ zone, product string }
	type tally struct {
		online, eligible, occupied, free int64
		eligibleUnknown, occupiedUnknown bool
	}
	tallies := map[key]*tally{}
	for _, observation := range observations {
		if !observation.Online {
			continue
		}
		zone := strings.TrimSpace(observation.Zone)
		if zone == "" {
			zone = UnlocatedZone
		}
		k := key{zone, strings.ToUpper(strings.TrimSpace(observation.ProductCode))}
		t := tallies[k]
		if t == nil {
			t = &tally{}
			tallies[k] = t
		}
		t.online++
		if observation.Eligible == nil {
			t.eligibleUnknown = true
		} else if *observation.Eligible {
			t.eligible++
		}
		if observation.Occupied == nil {
			t.occupiedUnknown = true
		} else if *observation.Occupied {
			t.occupied++
		}
		if observation.Eligible != nil && *observation.Eligible &&
			observation.Occupied != nil && !*observation.Occupied {
			t.free++
		}
	}
	bucket := BucketStart(now)
	snapshots := make([]SupplyFact, 0, len(tallies))
	for k, t := range tallies {
		snapshot := SupplyFact{Authority: authority, MarketID: marketID, Zone: k.zone,
			ProductCode: k.product, OnlineCount: t.online, ObservedAt: now.UTC(), BucketStart: bucket,
			PolicyVersion: policyVersion}
		if !t.eligibleUnknown {
			eligible := t.eligible
			snapshot.EligibleCount = &eligible
		}
		if !t.occupiedUnknown {
			occupied := t.occupied
			snapshot.OccupiedCount = &occupied
		}
		switch {
		case t.eligibleUnknown || t.occupiedUnknown:
			snapshot.State = SupplyUnknown
		case t.free > 0:
			snapshot.State = SupplyAvailable
		default:
			snapshot.State = SupplyLimited
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].EntityID() < snapshots[j].EntityID() })
	return snapshots
}

// Bool returns a pointer, for observations.
func Bool(value bool) *bool { return &value }
