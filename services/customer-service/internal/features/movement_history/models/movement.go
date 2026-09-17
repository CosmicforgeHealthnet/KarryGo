// Package movementmodels holds the customer movement read model and the pure
// projection rules. Customer Service never decides execution: it only
// reflects summaries published by Taxi, Dispatch and Hauling.
package movementmodels

import (
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/events"
)

const (
	OutcomeApplied           = "APPLIED"
	OutcomeStale             = "STALE"
	OutcomeTerminalProtected = "TERMINAL_PROTECTED"
	OutcomeDuplicate         = "DUPLICATE"
)

var ErrCustomerMismatch = errors.New("movement customer does not match the projected movement")

type Movement struct {
	CustomerID               string          `json:"-"`
	Authority                string          `json:"authority"`
	SourceRef                string          `json:"source_ref"`
	SourceVersion            int64           `json:"source_version"`
	ProductCode              *string         `json:"product_code"`
	MovementType             string          `json:"movement_type"`
	State                    string          `json:"state"`
	OriginSummary            json.RawMessage `json:"origin_summary"`
	DestinationSummary       json.RawMessage `json:"destination_summary"`
	ScheduledAt              *time.Time      `json:"scheduled_at"`
	StartedAt                *time.Time      `json:"started_at"`
	CompletedAt              *time.Time      `json:"completed_at"`
	ProviderReplacementCount int             `json:"provider_replacement_count"`
	RecoveryState            *string         `json:"recovery_state"`
	ParentContextType        *string         `json:"parent_context_type"`
	ParentContextRef         *string         `json:"parent_context_ref"`
	FulfilmentPlanRef        *string         `json:"fulfilment_plan_ref"`
	FulfilmentGroupRef       *string         `json:"fulfilment_group_ref"`
	MissionSequence          *int            `json:"mission_sequence"`
	MissionCount             *int            `json:"mission_count"`
	LastEventID              string          `json:"-"`
	LastEventAt              time.Time       `json:"last_event_at"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

// FromEnvelope converts a validated movement envelope into a projection row.
func FromEnvelope(envelope events.Envelope) (Movement, error) {
	if err := envelope.Validate(); err != nil {
		return Movement{}, err
	}
	var summary events.MovementSummary
	if err := json.Unmarshal(envelope.Payload, &summary); err != nil {
		return Movement{}, events.ErrInvalidEnvelope
	}
	if err := summary.Validate(); err != nil {
		return Movement{}, err
	}
	if summary.SourceRef != envelope.SourceEntityID {
		return Movement{}, events.ErrInvalidEnvelope
	}
	m := Movement{
		CustomerID: summary.CustomerID, Authority: summary.Authority, SourceRef: summary.SourceRef,
		SourceVersion: envelope.SourceVersion, ProductCode: text(summary.ProductCode), MovementType: summary.MovementType,
		State: summary.State, ScheduledAt: summary.ScheduledAt, StartedAt: summary.StartedAt, CompletedAt: summary.CompletedAt,
		ProviderReplacementCount: summary.ProviderReplacementCount, RecoveryState: text(summary.RecoveryState),
		ParentContextType: text(summary.ParentContextType), ParentContextRef: text(summary.ParentContextRef),
		FulfilmentPlanRef: text(summary.FulfilmentPlanRef), FulfilmentGroupRef: text(summary.FulfilmentGroupRef),
		MissionSequence: summary.MissionSequence, MissionCount: summary.MissionCount,
		LastEventID: envelope.EventID, LastEventAt: envelope.OccurredAt.UTC(),
	}
	if summary.Origin != nil {
		m.OriginSummary, _ = json.Marshal(summary.Origin)
	}
	if summary.Destination != nil {
		m.DestinationSummary, _ = json.Marshal(summary.Destination)
	}
	return m, nil
}

// Apply decides how an incoming summary changes the projection:
//   - no existing row: apply;
//   - a different customer for the same authority/source_ref: reject;
//   - an equal or older source_version: stale, ignored;
//   - an existing terminal state: protected, never regressed or rewritten;
//   - otherwise: apply, keeping the provider replacement count monotonic.
func Apply(existing *Movement, incoming Movement) (Movement, string, error) {
	if existing == nil {
		return incoming, OutcomeApplied, nil
	}
	if existing.CustomerID != incoming.CustomerID {
		return *existing, "", ErrCustomerMismatch
	}
	if incoming.SourceVersion <= existing.SourceVersion {
		return *existing, OutcomeStale, nil
	}
	if events.IsTerminalMovementState(existing.State) {
		return *existing, OutcomeTerminalProtected, nil
	}
	if incoming.ProviderReplacementCount < existing.ProviderReplacementCount {
		incoming.ProviderReplacementCount = existing.ProviderReplacementCount
	}
	// Descriptive context is sticky: an authority may omit unchanged
	// details on a later lifecycle event without erasing them.
	if len(incoming.OriginSummary) == 0 {
		incoming.OriginSummary = existing.OriginSummary
	}
	if len(incoming.DestinationSummary) == 0 {
		incoming.DestinationSummary = existing.DestinationSummary
	}
	incoming.ProductCode = keep(incoming.ProductCode, existing.ProductCode)
	incoming.ParentContextType = keep(incoming.ParentContextType, existing.ParentContextType)
	incoming.ParentContextRef = keep(incoming.ParentContextRef, existing.ParentContextRef)
	incoming.FulfilmentPlanRef = keep(incoming.FulfilmentPlanRef, existing.FulfilmentPlanRef)
	incoming.FulfilmentGroupRef = keep(incoming.FulfilmentGroupRef, existing.FulfilmentGroupRef)
	if incoming.ScheduledAt == nil {
		incoming.ScheduledAt = existing.ScheduledAt
	}
	if incoming.StartedAt == nil {
		incoming.StartedAt = existing.StartedAt
	}
	if incoming.MissionSequence == nil {
		incoming.MissionSequence, incoming.MissionCount = existing.MissionSequence, existing.MissionCount
	}
	incoming.CreatedAt = existing.CreatedAt
	return incoming, OutcomeApplied, nil
}

func keep(incoming, existing *string) *string {
	if incoming == nil {
		return existing
	}
	return incoming
}

func text(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
