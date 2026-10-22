package networkanalytics

// Section 13: demand buckets, produced by the operational authorities.
//
// Demand is counted from each authority's own committed work -- bookings and
// missions it has accepted -- in fixed time buckets per zone and product. The
// authority resolves the zone (a Taxi operating zone, or a coarse cell for the
// others); no pickup or dropoff coordinate reaches analytics.
//
// A bucket is re-published as it fills. Its version is the moment it was
// observed, so a later count supersedes an earlier one, a replay is a
// duplicate, and a delayed stale observation cannot move a count backwards.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EventDemandBucket is the demand fact.
const EventDemandBucket = "network.demand.bucket.v1"

// DemandBucketSize is the demand bucket width.
const DemandBucketSize = 15 * time.Minute

// DemandObservation is one committed unit of work, reduced before publishing.
type DemandObservation struct {
	MarketID    string
	Zone        string
	ProductCode string
	CommittedAt time.Time
	// Unmatched is true when the work could not be served.
	Unmatched bool
}

// DemandBucket is the published fact for one zone, product and bucket.
type DemandBucket struct {
	Authority      string    `json:"authority"`
	MarketID       string    `json:"market_id,omitempty"`
	Zone           string    `json:"zone"`
	ProductCode    string    `json:"product_code"`
	BucketStart    time.Time `json:"bucket_start"`
	BucketMinutes  int       `json:"bucket_minutes"`
	DemandCount    int64     `json:"demand_count"`
	UnmatchedCount int64     `json:"unmatched_count"`
	ObservedAt     time.Time `json:"observed_at"`
}

// EntityID identifies the bucket.
func (b DemandBucket) EntityID() string {
	return strings.Join([]string{b.Authority, b.MarketID, b.Zone, b.ProductCode,
		b.BucketStart.UTC().Format(time.RFC3339)}, "|")
}

// Version is the observation instant in microseconds.
func (b DemandBucket) Version() int64 { return b.ObservedAt.UTC().UnixMicro() }

// EventID is deterministic per bucket and observation.
func (b DemandBucket) EventID() string {
	sum := sha256.Sum256([]byte(b.EntityID() + "|" + strconv.FormatInt(b.Version(), 10)))
	return "demand-" + hex.EncodeToString(sum[:16])
}

// DemandBucketStart truncates an instant to its demand bucket.
func DemandBucketStart(at time.Time) time.Time { return at.UTC().Truncate(DemandBucketSize) }

// BuildDemandBuckets counts observations per market, zone, product and
// bucket. Every observation in the input is counted; the caller chooses the
// window (normally the open bucket and the one just closed).
func BuildDemandBuckets(authority string, observations []DemandObservation, now time.Time) []DemandBucket {
	type key struct {
		market, zone, product string
		bucket                time.Time
	}
	counts := map[key]*DemandBucket{}
	for _, observation := range observations {
		zone := strings.TrimSpace(observation.Zone)
		if zone == "" {
			zone = UnlocatedZone
		}
		k := key{strings.TrimSpace(observation.MarketID), zone,
			strings.ToUpper(strings.TrimSpace(observation.ProductCode)), DemandBucketStart(observation.CommittedAt)}
		bucket := counts[k]
		if bucket == nil {
			bucket = &DemandBucket{Authority: authority, MarketID: k.market, Zone: k.zone, ProductCode: k.product,
				BucketStart: k.bucket, BucketMinutes: int(DemandBucketSize / time.Minute), ObservedAt: now.UTC()}
			counts[k] = bucket
		}
		bucket.DemandCount++
		if observation.Unmatched {
			bucket.UnmatchedCount++
		}
	}
	buckets := make([]DemandBucket, 0, len(counts))
	for _, bucket := range counts {
		buckets = append(buckets, *bucket)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].EntityID() < buckets[j].EntityID() })
	return buckets
}

// DemandWindowStart is where an observer's demand window begins: the start of
// the previous bucket, so the bucket that just closed gets its final count.
func DemandWindowStart(now time.Time) time.Time {
	return DemandBucketStart(now).Add(-DemandBucketSize)
}

// SameSlotMean is the DemandForecastMethod model itself: the mean of the
// observed history slots. It is trusted only with at least
// DemandForecastMinimumSlots slots. A caller must pass only slots whose input
// is genuinely known; an absent slot is never a zero.
func SameSlotMean(history []int64) (expected float64, trusted bool) {
	if len(history) == 0 {
		return 0, false
	}
	var total int64
	for _, count := range history {
		total += count
	}
	return float64(total) / float64(len(history)), len(history) >= DemandForecastMinimumSlots
}
