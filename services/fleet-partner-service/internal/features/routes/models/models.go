// Package models holds the Section 16 route-planning domain: route plans,
// ordered stops, their state machines, stop validation and the optimization
// request/result contract. Canonical trips remain owned by Dispatch and
// Hauling; a published route only creates private assignments.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
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
)

// Job types run by the route workers.
const (
	JobOptimizationSubmit   = "section16.route_optimization.submit"
	JobOptimizationReadBack = "section16.route_optimization.readback"
	JobRouteProgress        = "section16.route.progress"
)

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

func validTransition(table map[string]map[string]bool, from, to string) bool {
	return from == to || table[from][to]
}
func ValidRouteTransition(from, to string) bool { return validTransition(routeTransitions, from, to) }
func ValidStopTransition(from, to string) bool  { return validTransition(stopTransitions, from, to) }

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

// ValidateStop applies the stop rules shared by creation, editing and
// publication. Passenger stops stay disabled.
func ValidateStop(v RouteStop) error {
	allowed := map[string]bool{"PICKUP": true, "DELIVERY": true, "WAREHOUSE_TRANSFER": true, "RETURN": true, "DEPOT_START": true, "DEPOT_END": true}
	if !allowed[strings.ToUpper(v.StopType)] {
		if strings.HasPrefix(strings.ToUpper(v.StopType), "PASSENGER_") {
			return errorcodes.Forbidden("taxi_execution_disabled", "Passenger stops are disabled.")
		}
		return apperrors.BadRequest("stopType is invalid.", nil)
	}
	if v.TimeWindowStart != nil && v.TimeWindowEnd != nil && !v.TimeWindowEnd.After(*v.TimeWindowStart) {
		return apperrors.BadRequest("time window end must be after start.", nil)
	}
	if (v.Latitude == nil) != (v.Longitude == nil) {
		return apperrors.BadRequest("latitude and longitude must be supplied together.", nil)
	}
	return nil
}

// IsDepot reports whether a stop is a depot boundary rather than work.
func IsDepot(stopType string) bool { return stopType == "DEPOT_START" || stopType == "DEPOT_END" }

type CreateRouteInput struct {
	BranchID, DispatchPlanID, RouteReference, RouteType, ProviderMembershipID, ProviderID, VehicleOrTruckReference, ExecutionService string
	ConstraintSnapshot                                                                                                               json.RawMessage
	IdempotencyKey                                                                                                                   string
}

type EditRouteInput struct {
	ProviderMembershipID, ProviderID, VehicleOrTruckReference string
	ConstraintSnapshot                                        json.RawMessage
	ExpectedVersion                                           int64
}

type PublishRouteInput struct {
	ExpectedVersion                int64
	OverrideReason, IdempotencyKey string
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

func OptimizationFingerprint(input OptimizationInput) string {
	raw, _ := json.Marshal(input)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type OptimizationRequest struct {
	ID                       string          `json:"id"`
	OrganizationID           string          `json:"organizationId"`
	RoutePlanID              string          `json:"routePlanId"`
	Status                   string          `json:"status"`
	ProviderReference        string          `json:"providerReference"`
	ProviderJobReference     string          `json:"providerJobReference"`
	InputFingerprint         string          `json:"inputFingerprint"`
	IdempotencyKey           string          `json:"idempotencyKey"`
	RequestFingerprint       string          `json:"requestFingerprint"`
	RouteVersion             int64           `json:"routeVersion"`
	OptimizationVersion      int64           `json:"optimizationVersion"`
	ResultVersion            int64           `json:"resultVersion"`
	InputSnapshot            json.RawMessage `json:"inputSnapshot"`
	ResultSnapshot           json.RawMessage `json:"resultSnapshot"`
	EstimatedDistance        float64         `json:"estimatedDistance"`
	EstimatedDurationSeconds int             `json:"estimatedDurationSeconds"`
}

// RouteProgress names an active route and the highest version of its stop
// assignments, so each assignment change schedules one progress job.
type RouteProgress struct {
	OrganizationID, RoutePlanID string
	AssignmentVersion           int64
}
