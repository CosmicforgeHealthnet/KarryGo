package inventory

import (
	"context"
	"strings"
	"time"
)

// SourceReference identifies a typed source-domain entity.
//
// Section 14B will introduce persistent generic Inventory item identity.
type SourceReference struct {
	Type string
	Ref  string
}

// Validate rejects ambiguous source references.
func (s SourceReference) Validate() error {
	if strings.TrimSpace(s.Type) == "" ||
		strings.TrimSpace(s.Ref) == "" {
		return ErrInvalidSource
	}

	return nil
}

// ResolvedInventorySource is bounded ownership/status evidence returned by a
// source-domain adapter.
type ResolvedInventorySource struct {
	Source SourceReference

	BusinessID string
	BranchID   *string

	Active bool
}

// InventoryItemResolver validates source-domain entities without causing
// Inventory to import Shop/Food repository implementations.
type InventoryItemResolver interface {
	ResolveInventorySource(
		ctx context.Context,
		actor Actor,
		scope BranchScope,
		source SourceReference,
	) (ResolvedInventorySource, error)
}

// InventoryTransaction is an opaque Inventory application transaction.
//
// Infrastructure adapters may wrap the repository's PostgreSQL transaction.
type InventoryTransaction interface {
	Context() context.Context
}

// TransactionBoundary executes one Inventory unit of work atomically.
//
// Future stock mutation, movement evidence, audit and outbox publication must
// be able to participate in the same transaction.
type TransactionBoundary interface {
	WithinInventoryTransaction(
		ctx context.Context,
		work func(InventoryTransaction) error,
	) error
}

// Evidence is bounded Inventory operational/audit evidence.
type Evidence struct {
	Action string

	AggregateType string
	AggregateRef  string

	Actor Actor
	Scope BranchScope

	ReasonCode string
	Metadata   map[string]any
}

// InventoryEvidenceRecorder records Inventory audit/operational evidence.
//
// Section 14A creates no new audit table.
type InventoryEvidenceRecorder interface {
	RecordInventoryEvidence(
		ctx context.Context,
		tx InventoryTransaction,
		evidence Evidence,
	) error
}

// Event is a bounded Inventory domain event.
type Event struct {
	Type string

	BusinessID string
	BranchID   *string

	AggregateType string
	AggregateRef  string

	Revision int64

	CorrelationID string
	CausationID   string

	OccurredAt time.Time

	Payload any
}

// InventoryEventPublisher publishes using an infrastructure adapter to the
// existing Vendor event/outbox architecture.
type InventoryEventPublisher interface {
	PublishInventoryEvent(
		ctx context.Context,
		tx InventoryTransaction,
		event Event,
	) error
}
