package http

import "encoding/json"

// Every administrative request names the person responsible and states why.
// They are required rather than optional because a rate change with no author and
// no reason cannot be reviewed months later, which is when it usually matters.
type actor struct {
	PerformedBy string `json:"performed_by"`
	Reason      string `json:"reason"`
}

type createDraftRequest struct {
	actor
	Kind           string          `json:"kind"`
	GeographyLevel string          `json:"geography_level"`
	GeographyRef   string          `json:"geography_ref"`
	SchemaVersion  int             `json:"schema_version"`
	Body           json.RawMessage `json:"body"`
	CloneFrom      string          `json:"clone_from,omitempty"`
}

type updateDraftRequest struct {
	actor
	Body             json.RawMessage `json:"body"`
	ExpectedBodyHash string          `json:"expected_body_hash"`
}

type decideRequest struct {
	actor
	ExpectedBodyHash string `json:"expected_body_hash"`
}

type activateRequest struct {
	actor
	// EffectiveFrom is RFC 3339. Empty means now. A past time is refused.
	EffectiveFrom    string `json:"effective_from,omitempty"`
	Rollback         bool   `json:"rollback,omitempty"`
	ExpectedBodyHash string `json:"expected_body_hash"`
}

type simulateRequest struct {
	// VersionID prices against a specific version, which is how a draft is tried
	// on real inputs before anyone approves it.
	VersionID string `json:"version_id"`
	// CommissionVersionID is optional; without it the active commission applies.
	CommissionVersionID string `json:"commission_version_id,omitempty"`

	Scope                  string `json:"scope"`
	Channel                string `json:"channel"`
	Class                  string `json:"class"`
	DistanceMeters         int64  `json:"distance_meters"`
	DurationSeconds        int64  `json:"duration_seconds"`
	TrafficDurationSeconds int64  `json:"traffic_duration_seconds,omitempty"`
	RouteConfidence        string `json:"route_confidence,omitempty"`
	RouteDegraded          bool   `json:"route_degraded,omitempty"`
	PackageSize            string `json:"package_size,omitempty"`
	ServiceTier            string `json:"service_tier,omitempty"`
	WeightGrams            int64  `json:"weight_grams,omitempty"`
	Helpers                int64  `json:"helpers,omitempty"`
}

type resolveRequest struct {
	Kind        string `form:"kind"`
	ZoneID      string `form:"zone_id"`
	MarketID    string `form:"market_id"`
	StateID     string `form:"state_id"`
	CountryCode string `form:"country_code"`
	AsOf        string `form:"as_of"`
}

type rolloutRequest struct {
	actor
	Scope      string `json:"scope"`
	Capability string `json:"capability"`
	MarketCode string `json:"market_code"`
	Mode       string `json:"mode"`
}

type freezeRequest struct {
	actor
}

type versionResponse struct {
	ID            string          `json:"id"`
	PolicyID      string          `json:"policy_id"`
	VersionNo     int             `json:"version_no"`
	Kind          string          `json:"kind"`
	SchemaVersion int             `json:"schema_version"`
	Status        string          `json:"status"`
	Body          json.RawMessage `json:"body"`
	BodyHash      string          `json:"body_hash"`
	CreatedBy     string          `json:"created_by"`
	ChangeReason  string          `json:"change_reason"`
	CreatedAt     string          `json:"created_at"`
}
