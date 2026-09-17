package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Customer movement summary contract. Operational authorities (Taxi,
// Dispatch, Hauling) publish a full state snapshot with every lifecycle or
// recovery event, so a consumer can project from any single event and
// out-of-order delivery resolves by SourceVersion alone.
const (
	MovementAuthorityTaxi     = "TAXI"
	MovementAuthorityDispatch = "DISPATCH"
	MovementAuthorityHauling  = "HAULING"

	MovementTypePassenger = "PASSENGER"
	MovementTypeGoods     = "GOODS"
	MovementTypeDispatch  = "DISPATCH"
	MovementTypeHaulage   = "HAULAGE"

	MovementSchemaVersion = 1

	// CustomerMovementIngressPath is customer-service's HMAC ingress.
	CustomerMovementIngressPath = "/internal/customer/v1/movement-events"

	// CustomerMovementGroupingPath is customer-service's HMAC ingress for
	// commerce grouping. Grouping says which movements belong to one
	// purchase; it never carries execution state or provider identity.
	CustomerMovementGroupingPath = "/internal/customer/v1/movement-groupings"

	// CommerceGroupingEventName is the only event that path accepts.
	CommerceGroupingEventName = "commerce.fulfilment.customer_grouping"

	// CustomerMovementStatePath lets a publishing authority ask what the
	// customer projection already holds for its own movements, so a backfill
	// republishes only what is missing or behind.
	CustomerMovementStatePath = "/internal/customer/v1/movement-state"
)

// Taxi canonical movement event names.
const (
	TaxiBookingCreated          = "taxi.booking.created"
	TaxiBookingAssigned         = "taxi.booking.assigned"
	TaxiBookingProviderArrived  = "taxi.booking.provider_arrived"
	TaxiBookingStarted          = "taxi.booking.started"
	TaxiBookingCompleted        = "taxi.booking.completed"
	TaxiBookingCancelled        = "taxi.booking.cancelled"
	TaxiBookingUnmatched        = "taxi.booking.unmatched"
	TaxiRecoveryStarted         = "taxi.recovery.started"
	TaxiRecoveryProviderReplace = "taxi.recovery.provider_replaced"
	TaxiRecoveryResolved        = "taxi.recovery.resolved"
)

// Normalized movement states shared by every authority.
const (
	MovementStateRequested  = "REQUESTED"
	MovementStateScheduled  = "SCHEDULED"
	MovementStateSearching  = "SEARCHING"
	MovementStateAssigned   = "ASSIGNED"
	MovementStateArrived    = "PROVIDER_ARRIVED"
	MovementStateInProgress = "IN_PROGRESS"
	MovementStateDelivered  = "DELIVERED"
	MovementStateCompleted  = "COMPLETED"
	MovementStateCancelled  = "CANCELLED"
	MovementStateUnmatched  = "UNMATCHED"
	MovementStateFailed     = "FAILED"
)

var movementStates = map[string]bool{
	MovementStateRequested: true, MovementStateScheduled: true, MovementStateSearching: true,
	MovementStateAssigned: true, MovementStateArrived: true, MovementStateInProgress: true,
	MovementStateDelivered: true, MovementStateCompleted: true, MovementStateCancelled: true,
	MovementStateUnmatched: true, MovementStateFailed: true,
}

// IsTerminalMovementState reports states a projection must never leave.
func IsTerminalMovementState(state string) bool {
	switch state {
	case MovementStateCompleted, MovementStateCancelled, MovementStateUnmatched, MovementStateFailed:
		return true
	}
	return false
}

// PlaceSummary is a customer-safe location summary. It carries no contact data.
type PlaceSummary struct {
	Label     string  `json:"label,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// MovementSummary is the payload of every customer movement event.
type MovementSummary struct {
	CustomerID               string        `json:"customer_id"`
	Authority                string        `json:"authority"`
	SourceRef                string        `json:"source_ref"`
	ProductCode              string        `json:"product_code,omitempty"`
	MovementType             string        `json:"movement_type"`
	State                    string        `json:"state"`
	Origin                   *PlaceSummary `json:"origin,omitempty"`
	Destination              *PlaceSummary `json:"destination,omitempty"`
	ScheduledAt              *time.Time    `json:"scheduled_at,omitempty"`
	StartedAt                *time.Time    `json:"started_at,omitempty"`
	CompletedAt              *time.Time    `json:"completed_at,omitempty"`
	ProviderReplacementCount int           `json:"provider_replacement_count"`
	RecoveryState            string        `json:"recovery_state,omitempty"`
	ParentContextType        string        `json:"parent_context_type,omitempty"`
	ParentContextRef         string        `json:"parent_context_ref,omitempty"`
	FulfilmentPlanRef        string        `json:"fulfilment_plan_ref,omitempty"`
	FulfilmentGroupRef       string        `json:"fulfilment_group_ref,omitempty"`
	MissionSequence          *int          `json:"mission_sequence,omitempty"`
	MissionCount             *int          `json:"mission_count,omitempty"`
}

func (m MovementSummary) Validate() error {
	switch {
	case strings.TrimSpace(m.CustomerID) == "":
		return fmt.Errorf("%w: customer_id", ErrInvalidEnvelope)
	case m.Authority != MovementAuthorityTaxi && m.Authority != MovementAuthorityDispatch && m.Authority != MovementAuthorityHauling:
		return fmt.Errorf("%w: authority", ErrInvalidEnvelope)
	case strings.TrimSpace(m.SourceRef) == "", len(m.SourceRef) > 128:
		return fmt.Errorf("%w: source_ref", ErrInvalidEnvelope)
	case m.MovementType != MovementTypePassenger && m.MovementType != MovementTypeGoods && m.MovementType != MovementTypeDispatch && m.MovementType != MovementTypeHaulage:
		return fmt.Errorf("%w: movement_type", ErrInvalidEnvelope)
	case !movementStates[m.State]:
		return fmt.Errorf("%w: state", ErrInvalidEnvelope)
	case m.ProviderReplacementCount < 0:
		return fmt.Errorf("%w: provider_replacement_count", ErrInvalidEnvelope)
	case m.MissionSequence != nil && *m.MissionSequence < 1, m.MissionCount != nil && *m.MissionCount < 1,
		m.MissionSequence != nil && m.MissionCount != nil && *m.MissionSequence > *m.MissionCount:
		return fmt.Errorf("%w: mission sequence", ErrInvalidEnvelope)
	}
	return nil
}

// NewMovementEnvelope builds a validated movement envelope.
func NewMovementEnvelope(eventID, eventName, sourceService, entityType string, sourceVersion int64, occurredAt time.Time, correlationID, causationID string, summary MovementSummary) (Envelope, error) {
	if err := summary.Validate(); err != nil {
		return Envelope{}, err
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		return Envelope{}, err
	}
	envelope := Envelope{EventID: eventID, EventName: eventName, SchemaVersion: MovementSchemaVersion, SourceService: sourceService,
		SourceEntityType: entityType, SourceEntityID: summary.SourceRef, SourceVersion: sourceVersion, OccurredAt: occurredAt.UTC(),
		CorrelationID: correlationID, CausationID: causationID, Payload: payload}
	return envelope, envelope.Validate()
}

// MovementGrouping is the commercial grouping of one customer purchase.
//
// It is deliberately thin: the parent order, the plan it was cut from, and
// which planned mission each fulfilment group is. It carries no state, no
// provider, no vehicle and no price, because Vendor owns none of those.
type MovementGrouping struct {
	CustomerID        string            `json:"customer_id"`
	ParentContextType string            `json:"parent_context_type"`
	ParentContextRef  string            `json:"parent_context_ref"`
	FulfilmentPlanRef string            `json:"fulfilment_plan_ref,omitempty"`
	Missions          []GroupingMission `json:"missions"`
}

// GroupingMission is one planned mission's place in the purchase.
type GroupingMission struct {
	FulfilmentGroupRef string `json:"fulfilment_group_ref"`
	MissionSequence    int    `json:"mission_sequence"`
	MissionCount       int    `json:"mission_count"`
}

func (g MovementGrouping) Validate() error {
	switch {
	case strings.TrimSpace(g.CustomerID) == "":
		return fmt.Errorf("%w: customer_id", ErrInvalidEnvelope)
	case strings.TrimSpace(g.ParentContextRef) == "":
		return fmt.Errorf("%w: parent_context_ref", ErrInvalidEnvelope)
	case len(g.Missions) == 0:
		return fmt.Errorf("%w: missions", ErrInvalidEnvelope)
	}
	for _, mission := range g.Missions {
		if strings.TrimSpace(mission.FulfilmentGroupRef) == "" || mission.MissionSequence < 1 || mission.MissionCount < 1 {
			return fmt.Errorf("%w: mission grouping is incomplete", ErrInvalidEnvelope)
		}
		if mission.MissionSequence > mission.MissionCount {
			return fmt.Errorf("%w: mission_sequence exceeds mission_count", ErrInvalidEnvelope)
		}
	}
	return nil
}

// NewGroupingEnvelope builds the grouping event Vendor publishes.
func NewGroupingEnvelope(eventID, sourceService, sourceRef string, sourceVersion int64, occurredAt time.Time, correlationID string, grouping MovementGrouping) (Envelope, error) {
	if err := grouping.Validate(); err != nil {
		return Envelope{}, err
	}
	payload, err := json.Marshal(grouping)
	if err != nil {
		return Envelope{}, err
	}
	envelope := Envelope{
		EventID: eventID, EventName: CommerceGroupingEventName, SchemaVersion: MovementSchemaVersion,
		SourceService: sourceService, SourceEntityType: "fulfilment_plan", SourceEntityID: sourceRef,
		SourceVersion: sourceVersion, OccurredAt: occurredAt.UTC(), CorrelationID: correlationID, Payload: payload,
	}
	return envelope, envelope.Validate()
}
