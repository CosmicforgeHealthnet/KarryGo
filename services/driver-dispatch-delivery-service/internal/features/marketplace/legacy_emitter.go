package marketplace

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LegacyEventEmitter lets the request and trip packages publish canonical
// marketplace events without importing this package.
//
// Those packages predate the marketplace outbox and own the transitions that
// eight of the eleven canonical events describe. Rather than move those
// transitions — a refactor beneath a large passing suite — they declare a
// narrow emitter interface of their own and this type satisfies it.
//
// The interface is expressed in terms of pgx.Tx rather than a package-local
// Querier for a reason that is easy to get wrong: Go interface satisfaction is
// by exact type identity, so a method taking marketplace.Querier would not
// satisfy an interface requiring trip.Querier even though the two are
// structurally identical. pgx.Tx is one named type shared by every package
// here, so this single emitter satisfies both consumers.
type LegacyEventEmitter struct {
	repo    *Repository
	enabled bool
}

// NewLegacyEventEmitter builds an emitter gated on the outbox flag.
//
// When the flag is off the emitter is inert rather than absent, so the wiring
// stays identical across environments and enabling the flag does not change
// which code path runs — only whether it writes.
func NewLegacyEventEmitter(repo *Repository, enabled bool) *LegacyEventEmitter {
	return &LegacyEventEmitter{repo: repo, enabled: enabled}
}

// Enabled reports whether this emitter writes events.
func (e *LegacyEventEmitter) Enabled() bool {
	return e != nil && e.enabled && e.repo != nil
}

// Emit publishes one validated event inside the caller's transaction.
//
// A nil transaction is an error rather than a fallback to the pool: an event
// that commits independently of the change it describes is precisely the split
// the outbox exists to prevent, and silently degrading to a pool write would
// reintroduce it at the one call site that forgot.
func (e *LegacyEventEmitter) Emit(ctx context.Context, tx pgx.Tx, eventType string,
	aggregateType string, aggregateID string, aggregateVersion int64,
	correlationID string, causationID string, payload map[string]any) error {
	if !e.Enabled() {
		return nil
	}
	if tx == nil {
		return fmt.Errorf("marketplace: event %s must be published inside the "+
			"transaction that produced it", eventType)
	}
	// A private-origin trip keeps Fleet references in a dedicated mapping table
	// rather than widening marketplace source constraints. Enrich lifecycle
	// events from that mapping so Fleet can project them without a second
	// cross-service lookup. Ordinary marketplace trips have no mapping and are
	// emitted unchanged.
	if aggregateType == "TRIP" {
		var operation, order, shipment, assignment, organization, settlement, provider string
		err := tx.QueryRow(ctx, `SELECT fleet_operation_id,fleet_private_order_id,
			fleet_shipment_id,fleet_assignment_id,organization_id,
			settlement_organization_id,executing_provider_id::text
			FROM dispatch_private_execution_mappings WHERE canonical_trip_id=$1`, aggregateID).
			Scan(&operation, &order, &shipment, &assignment, &organization, &settlement, &provider)
		if err == nil {
			payload["fleetExecutionOperationId"] = operation
			payload["privateOrderId"] = order
			payload["shipmentId"] = shipment
			payload["privateAssignmentId"] = assignment
			payload["organizationId"] = organization
			payload["settlementOrganizationId"] = settlement
			payload["executingProviderId"] = provider
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("marketplace: read private event references: %w", err)
		}
	}
	return e.repo.PublishMarketplaceEvent(ctx, tx, eventType, aggregateType,
		aggregateID, aggregateVersion, correlationID, causationID, payload)
}

// EmitTarget publishes a domain event to a named consumer through the same
// durable outbox used by the public marketplace lifecycle.
func (e *LegacyEventEmitter) EmitTarget(ctx context.Context, tx pgx.Tx,
	targetService, eventType, aggregateType, aggregateID string, aggregateVersion int64,
	correlationID, causationID string, payload map[string]any) error {
	if !e.Enabled() {
		return nil
	}
	if tx == nil {
		return fmt.Errorf("marketplace: event %s must be published inside the transaction that produced it", eventType)
	}
	return e.repo.PublishTargetedEvent(ctx, tx, targetService, eventType, aggregateType,
		aggregateID, aggregateVersion, correlationID, causationID, payload)
}
