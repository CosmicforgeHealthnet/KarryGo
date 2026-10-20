package analyticsadmin

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"encoding/json"
	"math"
	"strings"
	"time"
)

const MissionRatePermission = "analytics.scoped.mission-success-rate.read"
const MissionRateFormula = "completed/(completed+failed)"
const MissionRateBasis = "DISCLOSED_COMPONENT_COUNTS"

func MissionRateSourceContract(counts string) string {
	if !ValidMissionSourceContract(counts) {
		return ""
	}
	return strings.Replace(counts, "analytics.scoped-mission-success.v1:", "analytics.scoped-mission-success-rate.v1:", 1)
}

type RateRelease struct {
	PolicyID       string            `json:"policy_id"`
	PolicyVersion  int64             `json:"policy_version"`
	PolicyChecksum string            `json:"policy_checksum"`
	Scope          adminpolicy.Scope `json:"scope"`
	Metric         string            `json:"metric"`
	SourceVersion  string            `json:"source_version"`
	AsOf           time.Time         `json:"as_of"`
	Freshness      string            `json:"freshness"`
	Coverage       string            `json:"coverage"`
	Formula        string            `json:"formula"`
	Basis          string            `json:"basis"`
	Rate           *float64          `json:"rate"`
	Suppression    string            `json:"suppression"`
}

// MissionRate derives no information beyond already disclosed component counts.
// Approved source rounding is retained; it never substitutes raw source values.
func MissionRate(in Release) (RateRelease, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return RateRelease{}, ErrInvalid
	}
	in, err = Decode(raw)
	if err != nil {
		return RateRelease{}, err
	}
	out := RateRelease{PolicyID: in.PolicyID, PolicyVersion: in.PolicyVersion, PolicyChecksum: in.PolicyChecksum, Scope: in.Scope, Metric: "mission-success-rate", SourceVersion: in.SourceVersion, AsOf: in.AsOf, Freshness: in.Freshness, Coverage: in.Coverage, Formula: MissionRateFormula, Basis: MissionRateBasis}
	if in.Coverage != "COMPLETE" {
		out.Suppression = "PARTIAL_COVERAGE"
		return out, nil
	}
	if in.Freshness != "LIVE" {
		out.Suppression = "STALE"
		return out, nil
	}
	var completed, failed *int64
	for _, cell := range in.Cells {
		switch cell.Key {
		case "completed":
			completed = cell.Value
		case "failed":
			failed = cell.Value
		}
	}
	if completed == nil || failed == nil {
		out.Suppression = "SOURCE_SUPPRESSED"
		return out, nil
	}
	if *completed > math.MaxInt64-*failed {
		return RateRelease{}, ErrInvalid
	}
	denominator := *completed + *failed
	if denominator == 0 {
		out.Suppression = "NO_DENOMINATOR"
		return out, nil
	}
	value := float64(*completed) / float64(denominator)
	out.Rate = &value
	out.Suppression = "NONE"
	return out, nil
}
