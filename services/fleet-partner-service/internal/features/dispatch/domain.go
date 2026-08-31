package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

const (
	PlanDraft                = "DRAFT"
	PlanReady                = "READY"
	PlanApproved             = "APPROVED"
	PlanPublished            = "PUBLISHED"
	PlanInProgress           = "IN_PROGRESS"
	PlanCompleted            = "COMPLETED"
	PlanCancelled            = "CANCELLED"
	RouteDraft               = "DRAFT"
	RouteOptimizationPending = "OPTIMIZATION_PENDING"
	RouteOptimized           = "OPTIMIZED"
	RouteReady               = "READY"
	RouteApproved            = "APPROVED"
	RoutePublished           = "PUBLISHED"
	RouteInProgress          = "IN_PROGRESS"
	RouteCompleted           = "COMPLETED"
	RoutePartiallyCompleted  = "PARTIALLY_COMPLETED"
	RouteFailed              = "FAILED"
	RouteCancelled           = "CANCELLED"
	StopPlanned              = "PLANNED"
	StopAssigned             = "ASSIGNED"
	StopEnRoute              = "EN_ROUTE"
	StopArrived              = "ARRIVED"
	StopServiceStarted       = "SERVICE_STARTED"
	StopCompleted            = "COMPLETED"
	StopFailed               = "FAILED"
	StopSkipped              = "SKIPPED"
	StopCancelled            = "CANCELLED"
	Fresh                    = "FRESH"
	Aging                    = "AGING"
	Stale                    = "STALE"
	Unknown                  = "UNKNOWN"
	Gapped                   = "GAPPED"
	AlertOpen                = "OPEN"
	AlertAcknowledged        = "ACKNOWLEDGED"
	AlertInProgress          = "IN_PROGRESS"
	AlertResolved            = "RESOLVED"
	AlertDismissed           = "DISMISSED"
)

var planTransitions = map[string]map[string]bool{
	PlanDraft: {PlanReady: true, PlanApproved: true, PlanCancelled: true}, PlanReady: {PlanDraft: true, PlanApproved: true, PlanCancelled: true},
	PlanApproved: {PlanPublished: true, PlanCancelled: true}, PlanPublished: {PlanInProgress: true, PlanCompleted: true, PlanCancelled: true},
	PlanInProgress: {PlanCompleted: true, PlanCancelled: true},
}
var routeTransitions = map[string]map[string]bool{
	RouteDraft:               {RouteOptimizationPending: true, RouteReady: true, RouteApproved: true, RouteCancelled: true},
	RouteOptimizationPending: {RouteOptimized: true, RouteFailed: true, RouteCancelled: true},
	RouteOptimized:           {RouteReady: true, RouteApproved: true, RouteOptimizationPending: true, RouteCancelled: true}, RouteReady: {RouteApproved: true, RouteOptimizationPending: true, RouteCancelled: true},
	RouteApproved: {RoutePublished: true, RouteCancelled: true}, RoutePublished: {RouteInProgress: true, RouteCompleted: true, RoutePartiallyCompleted: true, RouteFailed: true, RouteCancelled: true},
	RouteInProgress: {RouteCompleted: true, RoutePartiallyCompleted: true, RouteFailed: true, RouteCancelled: true},
}
var stopTransitions = map[string]map[string]bool{
	StopPlanned: {StopAssigned: true, StopCancelled: true}, StopAssigned: {StopEnRoute: true, StopCancelled: true}, StopEnRoute: {StopArrived: true, StopFailed: true, StopCancelled: true},
	StopArrived: {StopServiceStarted: true, StopFailed: true, StopSkipped: true}, StopServiceStarted: {StopCompleted: true, StopFailed: true},
}

func init() {
	// Authoritative execution projections may legitimately compress intermediate
	// provider UI states. These explicit convergence edges never infer success:
	// they are used only after a contiguous authoritative completion/failure.
	stopTransitions[StopAssigned][StopCompleted] = true
	stopTransitions[StopAssigned][StopFailed] = true
	stopTransitions[StopEnRoute][StopCompleted] = true
	stopTransitions[StopArrived][StopCompleted] = true
}

var alertTransitions = map[string]map[string]bool{
	AlertOpen: {AlertAcknowledged: true, AlertDismissed: true}, AlertAcknowledged: {AlertInProgress: true, AlertResolved: true, AlertDismissed: true}, AlertInProgress: {AlertResolved: true},
}

func validTransition(table map[string]map[string]bool, from, to string) bool {
	return from == to || table[from][to]
}
func ValidPlanTransition(from, to string) bool  { return validTransition(planTransitions, from, to) }
func ValidRouteTransition(from, to string) bool { return validTransition(routeTransitions, from, to) }
func ValidStopTransition(from, to string) bool  { return validTransition(stopTransitions, from, to) }
func ValidAlertTransition(from, to string) bool { return validTransition(alertTransitions, from, to) }

type Flags struct {
	DispatchPlanning, RoutePlanning, RouteOptimization, RoutePublication          bool
	ControlTower, TrackingProjections, Geofences, OperationalAlerts               bool
	Workers, KillSwitch, OptimizationProviderConfigured, TrackingSourceConfigured bool
}

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
type RoutePlan struct {
	ID                      string          `json:"id"`
	OrganizationID          string          `json:"organizationId"`
	BranchID                string          `json:"branchId"`
	DispatchPlanID          string          `json:"dispatchPlanId,omitempty"`
	ReplacementForRouteID   string          `json:"replacementForRouteId,omitempty"`
	RouteReference          string          `json:"routeReference"`
	RouteType               string          `json:"routeType"`
	ProviderMembershipID    string          `json:"providerMembershipId,omitempty"`
	ProviderID              string          `json:"providerId,omitempty"`
	VehicleOrTruckReference string          `json:"vehicleOrTruckReference,omitempty"`
	ExecutionService        string          `json:"executionService"`
	Status                  string          `json:"status"`
	PlannedDistance         float64         `json:"plannedDistance"`
	DistanceUnit            string          `json:"distanceUnit"`
	PlannedDurationSeconds  int             `json:"plannedDurationSeconds"`
	OptimizationReference   string          `json:"optimizationReference,omitempty"`
	OptimizationVersion     int64           `json:"optimizationVersion"`
	ConstraintSnapshot      json.RawMessage `json:"constraintSnapshot"`
	ApprovedBy              string          `json:"approvedBy,omitempty"`
	ApprovedAt              *time.Time      `json:"approvedAt,omitempty"`
	PublishedAt             *time.Time      `json:"publishedAt,omitempty"`
	Version                 int64           `json:"version"`
	CreatedAt               time.Time       `json:"createdAt"`
	UpdatedAt               time.Time       `json:"updatedAt"`
}
type RouteStop struct {
	ID                     string     `json:"id"`
	OrganizationID         string     `json:"organizationId"`
	RoutePlanID            string     `json:"routePlanId"`
	Sequence               int        `json:"sequence"`
	StopType               string     `json:"stopType"`
	OrderID                string     `json:"orderId,omitempty"`
	ShipmentID             string     `json:"shipmentId,omitempty"`
	PackageID              string     `json:"packageId,omitempty"`
	ManifestID             string     `json:"manifestId,omitempty"`
	SourceOrderStopID      string     `json:"sourceOrderStopId,omitempty"`
	LocationReference      string     `json:"locationReference,omitempty"`
	Latitude               *float64   `json:"latitude,omitempty"`
	Longitude              *float64   `json:"longitude,omitempty"`
	TimeWindowStart        *time.Time `json:"timeWindowStart,omitempty"`
	TimeWindowEnd          *time.Time `json:"timeWindowEnd,omitempty"`
	ServiceDurationSeconds int        `json:"serviceDurationSeconds"`
	PlannedArrivalAt       *time.Time `json:"plannedArrivalAt,omitempty"`
	PlannedDepartureAt     *time.Time `json:"plannedDepartureAt,omitempty"`
	ActualArrivalAt        *time.Time `json:"actualArrivalAt,omitempty"`
	ActualDepartureAt      *time.Time `json:"actualDepartureAt,omitempty"`
	Status                 string     `json:"status"`
	Version                int64      `json:"version"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
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
		fresh = Unknown
	}
	if fresh == Stale || fresh == Aging || fresh == Unknown || fresh == Gapped {
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

type OptimizationInput struct {
	RouteID      string         `json:"routeId"`
	RouteVersion int64          `json:"routeVersion"`
	Stops        []RouteStop    `json:"stops"`
	Constraints  map[string]any `json:"constraints"`
}
type OptimizationResult struct {
	Sequence                 []string `json:"sequence"`
	EstimatedDistance        float64  `json:"estimatedDistance"`
	EstimatedDurationSeconds int      `json:"estimatedDurationSeconds"`
	ConstraintViolations     []string `json:"constraintViolations"`
	UnservedStops            []string `json:"unservedStops"`
	Warnings                 []string `json:"warnings"`
	ProviderReference        string   `json:"providerReference"`
	ProviderJobReference     string   `json:"providerJobReference"`
	InputFingerprint         string   `json:"inputFingerprint"`
	ResultVersion            int64    `json:"resultVersion"`
}
type OptimizationPort interface {
	Submit(ctx context.Context, input OptimizationInput) (providerJobReference string, unknown bool, err error)
	ReadBack(ctx context.Context, providerJobReference string) (OptimizationResult, bool, error)
}

func OptimizationFingerprint(input OptimizationInput) string {
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type DeterministicOptimizer struct{}

func (DeterministicOptimizer) Submit(input OptimizationInput) (string, bool, error) {
	return "local-" + OptimizationFingerprint(input)[:24], false, nil
}
func (DeterministicOptimizer) ReadBack(ref string) (OptimizationResult, bool, error) {
	return OptimizationResult{}, false, errors.New("deterministic optimizer requires test-supplied result")
}

type TrackingProjection struct {
	OrganizationID          string     `json:"organizationId"`
	SourceService           string     `json:"sourceService"`
	CanonicalTripReference  string     `json:"canonicalTripReference"`
	AssignmentID            string     `json:"assignmentId,omitempty"`
	RoutePlanID             string     `json:"routePlanId,omitempty"`
	ProviderID              string     `json:"providerId,omitempty"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference,omitempty"`
	Latitude                *float64   `json:"latitude,omitempty"`
	Longitude               *float64   `json:"longitude,omitempty"`
	Heading                 *float64   `json:"heading,omitempty"`
	Speed                   *float64   `json:"speed,omitempty"`
	Accuracy                *float64   `json:"accuracy,omitempty"`
	OperationalStatus       string     `json:"operationalStatus"`
	SourceTimestamp         time.Time  `json:"sourceTimestamp"`
	ReceivedAt              time.Time  `json:"receivedAt"`
	SourceVersion           int64      `json:"sourceVersion"`
	FreshnessStatus         string     `json:"freshnessStatus"`
	GapStatus               string     `json:"gapStatus"`
	LastReconciledAt        *time.Time `json:"lastReconciledAt,omitempty"`
}

func Freshness(source, now time.Time, freshAfter, staleAfter time.Duration, gapped bool) string {
	if gapped {
		return Gapped
	}
	if source.IsZero() {
		return Unknown
	}
	age := now.Sub(source)
	if age <= freshAfter {
		return Fresh
	}
	if age <= staleAfter {
		return Aging
	}
	return Stale
}

type TrackingEvent struct {
	EventID, OrganizationID, SourceService, CanonicalTripReference, AssignmentID, RoutePlanID, ProviderID, VehicleOrTruckReference, OperationalStatus string
	Latitude, Longitude, Heading, Speed, Accuracy                                                                                                     *float64
	SourceTimestamp                                                                                                                                   time.Time
	SourceVersion, ReconcilesThroughVersion                                                                                                           int64
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

type PageFilter struct {
	BranchID, Status, Service, ProviderID, VehicleReference, AlertType string
	Date                                                               *time.Time
	Limit, Offset                                                      int
}
type Board struct {
	Unassigned             []map[string]any     `json:"unassigned"`
	PendingAssignments     []map[string]any     `json:"pendingAssignments"`
	AcceptedAssignments    []map[string]any     `json:"acceptedAssignments"`
	ActiveRoutes           []RoutePlan          `json:"activeRoutes"`
	ActiveTrips            []TrackingProjection `json:"activeTrips"`
	Alerts                 []OperationalAlert   `json:"alerts"`
	TrackingSummary        map[string]int       `json:"trackingSummary"`
	CapacitySummary        map[string]int       `json:"capacitySummary"`
	ProviderAvailability   []map[string]any     `json:"providerAvailability"`
	VehicleAvailability    []map[string]any     `json:"vehicleAvailability"`
	ProjectionGaps         []map[string]any     `json:"projectionGaps"`
	ProjectionIncomplete   bool                 `json:"projectionIncomplete"`
	ReconciliationRequired bool                 `json:"reconciliationRequired"`
}

func normalizeRules(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		out[i] = strings.ToUpper(strings.TrimSpace(out[i]))
	}
	sort.Strings(out)
	return out
}
