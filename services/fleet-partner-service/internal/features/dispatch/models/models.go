package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	routemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	trackingmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/shared/go/geo"
)

const (
	PlanDraft         = "DRAFT"
	PlanReady         = "READY"
	PlanApproved      = "APPROVED"
	PlanPublished     = "PUBLISHED"
	PlanInProgress    = "IN_PROGRESS"
	PlanCompleted     = "COMPLETED"
	PlanCancelled     = "CANCELLED"
	AlertOpen         = "OPEN"
	AlertAcknowledged = "ACKNOWLEDGED"
	AlertInProgress   = "IN_PROGRESS"
	AlertResolved     = "RESOLVED"
	AlertDismissed    = "DISMISSED"
)

// Job types run by the dispatch workers.
const (
	JobGeofenceEvaluate = "section16.geofence.evaluate"
	JobAlertGenerate    = "section16.alert.generate"
	JobAlertEscalate    = "section16.alert.escalate"
)

var planTransitions = map[string]map[string]bool{
	PlanDraft: {PlanReady: true, PlanApproved: true, PlanCancelled: true}, PlanReady: {PlanDraft: true, PlanApproved: true, PlanCancelled: true},
	PlanApproved: {PlanPublished: true, PlanCancelled: true}, PlanPublished: {PlanInProgress: true, PlanCompleted: true, PlanCancelled: true},
	PlanInProgress: {PlanCompleted: true, PlanCancelled: true},
}

var alertTransitions = map[string]map[string]bool{
	AlertOpen: {AlertAcknowledged: true, AlertDismissed: true}, AlertAcknowledged: {AlertInProgress: true, AlertResolved: true, AlertDismissed: true}, AlertInProgress: {AlertResolved: true},
}

func validTransition(table map[string]map[string]bool, from, to string) bool {
	return from == to || table[from][to]
}
func ValidPlanTransition(from, to string) bool  { return validTransition(planTransitions, from, to) }
func ValidAlertTransition(from, to string) bool { return validTransition(alertTransitions, from, to) }

type DispatchPlan struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	BranchID        string     `json:"branchId"`
	PlanDate        time.Time  `json:"planDate"`
	WorkWindowStart time.Time  `json:"workWindowStart"`
	WorkWindowEnd   time.Time  `json:"workWindowEnd"`
	Timezone        string     `json:"timezone"`
	Status          string     `json:"status"`
	PlannerUserID   string     `json:"plannerUserId"`
	ApprovedBy      string     `json:"approvedBy,omitempty"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty"`
	PublishedAt     *time.Time `json:"publishedAt,omitempty"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CreatePlanInput struct {
	BranchID        string    `json:"branchId"`
	PlanDate        time.Time `json:"planDate"`
	WorkWindowStart time.Time `json:"workWindowStart"`
	WorkWindowEnd   time.Time `json:"workWindowEnd"`
	Timezone        string    `json:"timezone"`
	IdempotencyKey  string    `json:"-"`
}

type EditPlanInput struct {
	Timezone        string    `json:"timezone"`
	WorkWindowStart time.Time `json:"workWindowStart"`
	WorkWindowEnd   time.Time `json:"workWindowEnd"`
	ExpectedVersion int64     `json:"expectedVersion"`
}

type ScoreComponent struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
	Reason string `json:"reason"`
}
type Candidate struct {
	ID                      string           `json:"id"`
	OrganizationID          string           `json:"organizationId"`
	BranchID                string           `json:"branchId"`
	SourceType              string           `json:"sourceType"`
	SourceID                string           `json:"sourceId"`
	ProviderID              string           `json:"providerId"`
	ProviderMembershipID    string           `json:"providerMembershipId"`
	VehicleOrTruckReference string           `json:"vehicleOrTruckReference"`
	ExecutionService        string           `json:"executionService"`
	EligibilityStatus       string           `json:"eligibilityStatus"`
	TotalScore              int              `json:"totalScore"`
	ScoreComponents         []ScoreComponent `json:"scoreComponents"`
	EligibilityReasons      []string         `json:"eligibilityReasons"`
	Warnings                []string         `json:"warnings"`
	HardBlocks              []string         `json:"hardBlocks"`
	DataFreshness           string           `json:"dataFreshness"`
	GeneratedAt             time.Time        `json:"generatedAt"`
	ExpiresAt               time.Time        `json:"expiresAt"`
	SourceVersions          map[string]int64 `json:"sourceVersions"`
	InputFingerprint        string           `json:"inputFingerprint"`
}
type CandidateFact struct {
	ProviderID, ProviderMembershipID, VehicleReference, ExecutionService                                                                                                   string
	MembershipActive, ProviderActive, CapabilityMatch, Available, VehicleEligible, CapacityOK, BranchAllowed, CargoAllowed, SafetyAllowed, ReservationFree, ActiveWorkFree bool
	TrackingFreshness                                                                                                                                                      string
	SourceVersions                                                                                                                                                         map[string]int64
}

func ScoreCandidate(org, branch, sourceType, sourceID string, fact CandidateFact, now time.Time, ttl time.Duration) Candidate {
	blocks := []string{}
	warnings := []string{}
	reasons := []string{}
	components := []ScoreComponent{}
	score := 0
	checks := []struct {
		name          string
		ok            bool
		points        int
		reason, block string
	}{
		{"active_membership", fact.MembershipActive, 10, "active organization membership", "INACTIVE_MEMBERSHIP"}, {"provider_status", fact.ProviderActive, 10, "provider active", "PROVIDER_INACTIVE"}, {"capability", fact.CapabilityMatch, 15, "service capability matches", "CAPABILITY_MISMATCH"}, {"availability", fact.Available, 15, "provider available", "PROVIDER_UNAVAILABLE"}, {"vehicle", fact.VehicleEligible, 15, "vehicle or truck eligible", "VEHICLE_INELIGIBLE"}, {"capacity", fact.CapacityOK, 10, "capacity is sufficient", "CAPACITY_EXCEEDED"}, {"branch", fact.BranchAllowed, 10, "branch scope matches", "BRANCH_RESTRICTED"}, {"cargo", fact.CargoAllowed, 5, "cargo restrictions satisfied", "CARGO_RESTRICTED"}, {"safety", fact.SafetyAllowed, 5, "safety restrictions satisfied", "SAFETY_BLOCK"}, {"reservation", fact.ReservationFree, 3, "no reservation conflict", "RESERVATION_CONFLICT"}, {"active_work", fact.ActiveWorkFree, 2, "active work permits assignment", "ACTIVE_WORK_CONFLICT"},
	}
	for _, c := range checks {
		if c.ok {
			score += c.points
			reasons = append(reasons, c.reason)
			components = append(components, ScoreComponent{c.name, c.points, c.reason})
		} else {
			blocks = append(blocks, c.block)
			components = append(components, ScoreComponent{c.name, 0, c.block})
		}
	}
	fresh := strings.ToUpper(fact.TrackingFreshness)
	if fresh == "" {
		fresh = trackingmodels.Unknown
	}
	if fresh == trackingmodels.Stale || fresh == trackingmodels.Aging || fresh == trackingmodels.Unknown || fresh == trackingmodels.Gapped {
		warnings = append(warnings, "TRACKING_"+fresh)
	}
	status := "ELIGIBLE"
	if len(blocks) > 0 {
		status = "INELIGIBLE"
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	versions, _ := json.Marshal(fact.SourceVersions)
	sum := sha256.Sum256([]byte(strings.Join([]string{org, branch, sourceType, sourceID, fact.ProviderMembershipID, fact.VehicleReference, string(versions)}, "|")))
	return Candidate{OrganizationID: org, BranchID: branch, SourceType: sourceType, SourceID: sourceID, ProviderID: fact.ProviderID, ProviderMembershipID: fact.ProviderMembershipID, VehicleOrTruckReference: fact.VehicleReference, ExecutionService: fact.ExecutionService, EligibilityStatus: status, TotalScore: score, ScoreComponents: components, EligibilityReasons: reasons, Warnings: warnings, HardBlocks: blocks, DataFreshness: fresh, GeneratedAt: now.UTC(), ExpiresAt: now.UTC().Add(ttl), SourceVersions: fact.SourceVersions, InputFingerprint: hex.EncodeToString(sum[:])}
}

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type Geometry struct {
	Center       *Point  `json:"center,omitempty"`
	RadiusMeters float64 `json:"radiusMeters,omitempty"`
	Points       []Point `json:"points,omitempty"`
}
type Geofence struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organizationId"`
	BranchID          string    `json:"branchId,omitempty"`
	Name              string    `json:"name"`
	GeometryType      string    `json:"geometryType"`
	Geometry          Geometry  `json:"geometry"`
	GeometryReference string    `json:"geometryReference,omitempty"`
	EventRules        []string  `json:"eventRules"`
	ActiveStatus      string    `json:"activeStatus"`
	Version           int64     `json:"version"`
	CreatedBy         string    `json:"createdBy"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type CreateGeofenceInput struct {
	BranchID          string   `json:"branchId"`
	Name              string   `json:"name"`
	GeometryType      string   `json:"geometryType"`
	GeometryReference string   `json:"geometryReference"`
	Geometry          Geometry `json:"geometry"`
	EventRules        []string `json:"eventRules"`
	IdempotencyKey    string   `json:"idempotencyKey"`
}

func ValidateGeometry(kind string, g Geometry) error {
	kind = strings.ToUpper(kind)
	if kind == "CIRCLE" {
		if g.Center == nil || g.RadiusMeters <= 0 || !validPoint(*g.Center) {
			return errors.New("circle requires a valid center and positive radius")
		}
		return nil
	}
	if kind != "POLYGON" || len(g.Points) < 4 {
		return errors.New("polygon requires at least three vertices and a closing point")
	}
	for _, p := range g.Points {
		if !validPoint(p) {
			return errors.New("polygon contains invalid coordinate")
		}
	}
	if g.Points[0] != g.Points[len(g.Points)-1] {
		return errors.New("polygon must be closed")
	}
	if selfIntersects(g.Points) {
		return errors.New("polygon must not self-intersect")
	}
	return nil
}
func validPoint(p Point) bool {
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
}
func selfIntersects(p []Point) bool {
	for i := 0; i < len(p)-1; i++ {
		for j := i + 1; j < len(p)-1; j++ {
			if abs(i-j) <= 1 || (i == 0 && j == len(p)-2) {
				continue
			}
			if segmentsIntersect(p[i], p[i+1], p[j], p[j+1]) {
				return true
			}
		}
	}
	return false
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func orient(a, b, c Point) float64 {
	return (b.Longitude-a.Longitude)*(c.Latitude-a.Latitude) - (b.Latitude-a.Latitude)*(c.Longitude-a.Longitude)
}
func segmentsIntersect(a, b, c, d Point) bool {
	return orient(a, b, c)*orient(a, b, d) < 0 && orient(c, d, a)*orient(c, d, b) < 0
}
func Contains(g Geofence, p Point) bool {
	if strings.ToUpper(g.GeometryType) == "CIRCLE" && g.Geometry.Center != nil {
		return haversine(*g.Geometry.Center, p) <= g.Geometry.RadiusMeters
	}
	inside := false
	pts := g.Geometry.Points
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		if ((pts[i].Latitude > p.Latitude) != (pts[j].Latitude > p.Latitude)) && (p.Longitude < (pts[j].Longitude-pts[i].Longitude)*(p.Latitude-pts[i].Latitude)/(pts[j].Latitude-pts[i].Latitude)+pts[i].Longitude) {
			inside = !inside
		}
	}
	return inside
}

// haversine returns metres between two points, delegating to the
// platform-wide implementation in shared/go/geo.
func haversine(a, b Point) float64 {
	return geo.HaversineMeters(
		geo.Point{Lat: a.Latitude, Lng: a.Longitude},
		geo.Point{Lat: b.Latitude, Lng: b.Longitude},
	)
}

// NormalizeRules upper-cases, trims and sorts geofence event rules.
func NormalizeRules(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		out[i] = strings.ToUpper(strings.TrimSpace(out[i]))
	}
	sort.Strings(out)
	return out
}

// ValidEventRule reports whether rule is a supported geofence event rule.
func ValidEventRule(rule string) bool {
	return map[string]bool{"ENTER": true, "EXIT": true, "DWELL": true, "ROUTE_DEVIATION": true, "RESTRICTED_AREA": true}[rule]
}

// RuleAllowed reports whether the geofence is configured for rule.
func RuleAllowed(g Geofence, rule string) bool {
	for _, configured := range g.EventRules {
		if configured == rule {
			return true
		}
	}
	return false
}

// GeofenceEvent is one derived geofence transition (spec 16.12, 45.2). Events
// are written only by the geofence worker from FRESH authoritative tracking
// projections; this read surface never derives or edits them. DataFreshness
// reports how live the triggering location fix is at read time, so an old
// transition is never mistaken for a live position.
type GeofenceEvent struct {
	ID                     string    `json:"id"`
	OrganizationID         string    `json:"organizationId"`
	GeofenceID             string    `json:"geofenceId"`
	GeofenceName           string    `json:"geofenceName"`
	BranchID               string    `json:"branchId,omitempty"`
	CanonicalTripReference string    `json:"canonicalTripReference"`
	EventRule              string    `json:"eventRule"`
	SourceVersion          int64     `json:"sourceVersion"`
	Confidence             string    `json:"confidence"`
	OccurredAt             time.Time `json:"occurredAt"`
	RecordedAt             time.Time `json:"recordedAt"`
	DataFreshness          string    `json:"dataFreshness"`
	AgeSeconds             int64     `json:"ageSeconds"`
}

// GeofenceEventFilter selects geofence events. It pages by Offset or by the
// keyset position After/AfterID, never both.
type GeofenceEventFilter struct {
	GeofenceID, CanonicalTripReference, EventRule, BranchID string
	From, To                                                *time.Time
	Limit, Offset                                           int
	After                                                   *time.Time
	AfterID                                                 string
}

// GeofenceTransitionState is the latest recorded source version per rule
// for one geofence and trip, plus when it was last entered.
type GeofenceTransitionState struct {
	LastEnter, LastExit, LastDwell, LastDeviation, LastRestricted int64
	EnteredAt                                                     *time.Time
}

// GeofenceTransitions decides which rules a fresh position fires.
func GeofenceTransitions(g Geofence, inside bool, s GeofenceTransitionState, sourceTimestamp time.Time, onRoute bool) []string {
	rules := []string{}
	if inside {
		if s.LastEnter <= s.LastExit && RuleAllowed(g, "ENTER") {
			rules = append(rules, "ENTER")
		}
		if s.LastEnter > s.LastExit && s.LastDwell < s.LastEnter && s.EnteredAt != nil && sourceTimestamp.Sub(*s.EnteredAt) >= 5*time.Minute && RuleAllowed(g, "DWELL") {
			rules = append(rules, "DWELL")
		}
		if s.LastRestricted <= s.LastExit && RuleAllowed(g, "RESTRICTED_AREA") {
			rules = append(rules, "RESTRICTED_AREA")
		}
	} else if s.LastEnter > s.LastExit {
		if RuleAllowed(g, "EXIT") {
			rules = append(rules, "EXIT")
		}
		if onRoute && s.LastDeviation < s.LastEnter && RuleAllowed(g, "ROUTE_DEVIATION") {
			rules = append(rules, "ROUTE_DEVIATION")
		}
	}
	return rules
}

type OperationalAlert struct {
	ID                     string     `json:"id"`
	OrganizationID         string     `json:"organizationId"`
	BranchID               string     `json:"branchId,omitempty"`
	AlertType              string     `json:"alertType"`
	Severity               string     `json:"severity"`
	SourceType             string     `json:"sourceType"`
	SourceID               string     `json:"sourceId"`
	RoutePlanID            string     `json:"routePlanId,omitempty"`
	AssignmentID           string     `json:"assignmentId,omitempty"`
	ShipmentID             string     `json:"shipmentId,omitempty"`
	CanonicalTripReference string     `json:"canonicalTripReference,omitempty"`
	OwnerUserID            string     `json:"ownerUserId,omitempty"`
	Status                 string     `json:"status"`
	SLADueAt               time.Time  `json:"slaDueAt"`
	AcknowledgedAt         *time.Time `json:"acknowledgedAt,omitempty"`
	ResolvedAt             *time.Time `json:"resolvedAt,omitempty"`
	ResolutionCode         string     `json:"resolutionCode,omitempty"`
	ResolutionReason       string     `json:"resolutionReason,omitempty"`
	Version                int64      `json:"version"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

// AlertRequest describes an operational alert to raise for one condition.
type AlertRequest struct {
	Kind, Severity, SourceType, SourceID, BranchID, RoutePlanID, AssignmentID, CanonicalTripReference, Condition string
	SLADueAt                                                                                                     time.Time
}

// EscalatedSeverity is the next severity of an alert past its SLA.
func EscalatedSeverity(severity string) string {
	return map[string]string{"INFO": "LOW", "LOW": "MEDIUM", "MEDIUM": "HIGH", "HIGH": "CRITICAL", "CRITICAL": "CRITICAL"}[severity]
}

// AssignmentCondition is a route-stop assignment the maintenance sweep
// reports as needing a dispatcher.
type AssignmentCondition struct {
	OrganizationID, BranchID, RoutePlanID, AssignmentID, Status string
	Version                                                     int64
}

type Board struct {
	Unassigned             []map[string]any                    `json:"unassigned"`
	PendingAssignments     []map[string]any                    `json:"pendingAssignments"`
	AcceptedAssignments    []map[string]any                    `json:"acceptedAssignments"`
	ActiveRoutes           []routemodels.RoutePlan             `json:"activeRoutes"`
	ActiveTrips            []trackingmodels.TrackingProjection `json:"activeTrips"`
	Alerts                 []OperationalAlert                  `json:"alerts"`
	TrackingSummary        map[string]int                      `json:"trackingSummary"`
	CapacitySummary        map[string]int                      `json:"capacitySummary"`
	ProviderAvailability   []map[string]any                    `json:"providerAvailability"`
	VehicleAvailability    []map[string]any                    `json:"vehicleAvailability"`
	ProjectionGaps         []map[string]any                    `json:"projectionGaps"`
	ProjectionIncomplete   bool                                `json:"projectionIncomplete"`
	ReconciliationRequired bool                                `json:"reconciliationRequired"`
}

// CandidateAvailability is one unexpired candidate projection row.
type CandidateAvailability struct {
	ProviderID, VehicleReference, Eligibility, BranchID string
	GeneratedAt, ExpiresAt                              time.Time
}

// AssignmentSummary is one private assignment row for the board.
type AssignmentSummary struct {
	ID, BranchID, WorkOrigin, Status, ProviderID, VehicleReference, ExecutionService string
	ResponseDeadline                                                                 time.Time
}
