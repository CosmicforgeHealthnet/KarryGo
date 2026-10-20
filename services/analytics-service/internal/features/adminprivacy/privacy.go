// Package adminprivacy is the owner-side disclosure framework. Its Source port
// must resolve scoped cohorts from Analytics' canonical projections; no public
// route or raw/unscoped legacy report is registered by this package.
package adminprivacy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
)

var ErrRestricted = errors.New("restricted analytics query")
var ErrReconstruction = errors.New("reconstruction guard refused disclosure")
var ErrRate = errors.New("analytics query budget exhausted")
var ErrUnmapped = errors.New("authoritative scoped cohort source unavailable")
var ErrSource = errors.New("invalid analytics cohort source")

type Query struct {
	Capability string            `json:"capability"`
	Metric     string            `json:"metric"`
	Scope      adminpolicy.Scope `json:"scope"`
	Purpose    string            `json:"purpose"`
	Start      time.Time         `json:"start"`
	End        time.Time         `json:"end"`
	Filters    map[string]string `json:"filters"`
}
type Cell struct {
	Key        string
	CohortSize *int64
	Count      *int64
}
type Snapshot struct {
	Facts adminpolicy.Facts
	// Authorization records the original private membership witness. It is
	// never serialized into a Release or treated as complete cohort coverage.
	Authorization json.RawMessage
	SourceVersion string
	AsOf          time.Time
	Cells         []Cell
	Partial       bool
}
type ReleasedCell = analyticsadmin.Cell
type Release = analyticsadmin.Release

func hash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func contains(values []string, v string) bool {
	for _, value := range values {
		if value == v {
			return true
		}
	}
	return false
}

func validateQuery(q Query, p adminpolicy.Disclosure, now time.Time) error {
	if !adminread.Opaque(q.Metric) || q.Start.IsZero() || !q.End.After(q.Start) || q.End.After(now) || q.Start.Before(now.Add(-time.Duration(p.MaxLookbackSeconds)*time.Second)) || q.End.Sub(q.Start) != time.Duration(p.ReleaseBucketSeconds)*time.Second || q.Start.Unix()%int64(p.ReleaseBucketSeconds) != 0 || len(q.Filters) == 0 || len(q.Filters) > 8 {
		return ErrRestricted
	}
	allowed := false
	for _, set := range p.FilterSets {
		if len(set) != len(q.Filters) {
			continue
		}
		match := true
		for _, field := range set {
			value, exists := q.Filters[field]
			if !exists || !contains(p.AllowedFilterValues[field], value) {
				match = false
				break
			}
		}
		allowed = allowed || match
	}
	if !allowed {
		return ErrRestricted
	}
	return nil
}

// Suppression is conservative: if any cell is small or unknown, all cells in
// the release are withheld, including otherwise publishable complements. Raw
// cohort sizes and totals never appear in the released shape. This prevents
// within-release subtraction; the durable journal separately fences repeated
// filter/window/snapshot variations across users and processes.
func suppress(q Query, s Snapshot, p adminpolicy.Disclosure, policy *adminpolicy.Engine, now time.Time) (Release, error) {
	out := Release{PolicyID: policy.ID(), PolicyVersion: policy.Version(), PolicyChecksum: policy.Checksum(), Scope: q.Scope, Metric: q.Metric, SourceVersion: s.SourceVersion, AsOf: s.AsOf, Freshness: "LIVE", Coverage: "COMPLETE", Cells: []ReleasedCell{}}
	if !adminread.Opaque(s.SourceVersion) || s.AsOf.IsZero() || s.AsOf.After(now.Add(time.Minute)) || len(s.Cells) != len(p.AllowedCellKeys) || len(s.Cells) > 128 {
		return out, ErrSource
	}
	if s.Partial {
		out.Coverage = "PARTIAL_COVERAGE"
	}
	stale := now.Sub(s.AsOf) > time.Duration(p.FreshnessSeconds)*time.Second
	primary := map[string]string{}
	seen := map[string]bool{}
	for _, cell := range s.Cells {
		if !contains(p.AllowedCellKeys, cell.Key) || seen[cell.Key] || cell.Count != nil && *cell.Count < 0 || cell.CohortSize != nil && *cell.CohortSize < 0 {
			return out, ErrSource
		}
		seen[cell.Key] = true
		if cell.Count == nil || cell.CohortSize == nil {
			primary[cell.Key] = "UNKNOWN"
		} else if *cell.CohortSize < p.MinimumCohort || *cell.Count > 0 && *cell.Count < p.MinimumCohort {
			primary[cell.Key] = "SMALL_CELL"
		}
	}
	for _, cell := range s.Cells {
		released := ReleasedCell{Key: cell.Key, Suppression: "NONE"}
		switch {
		case stale:
			released.Suppression = "STALE"
			out.Freshness = "STALE"
		case primary[cell.Key] != "":
			released.Suppression = primary[cell.Key]
		case len(primary) > 0:
			released.Suppression = "COMPLEMENTARY"
		case s.Partial:
			released.Suppression = "PARTIAL_COVERAGE"
		default:
			value := *cell.Count - (*cell.Count % p.RoundingMultiple)
			released.Value = &value
		}
		out.Cells = append(out.Cells, released)
	}
	sort.Slice(out.Cells, func(i, j int) bool { return out.Cells[i].Key < out.Cells[j].Key })
	return out, nil
}
func suppressStale(out Release, p adminpolicy.Disclosure, now time.Time) Release {
	if now.Sub(out.AsOf) > time.Duration(p.FreshnessSeconds)*time.Second {
		out.Freshness = "STALE"
		for i := range out.Cells {
			out.Cells[i].Value = nil
			out.Cells[i].Suppression = "STALE"
		}
	}
	return out
}
