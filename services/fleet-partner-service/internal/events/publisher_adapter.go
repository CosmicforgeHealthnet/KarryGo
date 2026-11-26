package events

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// DomainPublisher appends corporate domain events to the transactional
// outbox inside the caller's transaction, so an event exists if and only if
// the domain mutation committed.
type DomainPublisher struct {
	outbox *OutboxRepository
}

// NewDomainPublisher wires the adapter.
func NewDomainPublisher(outbox *OutboxRepository) *DomainPublisher {
	return &DomainPublisher{outbox: outbox}
}

// Publish builds a corporate envelope and appends it to the outbox.
//
// Payloads must never carry OTP codes, invitation tokens, access or refresh
// tokens, identity documents or full financial account data — callers pass
// only safe references and masked values.
func (p *DomainPublisher) Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error {
	envelope, err := NewEnvelope(eventType, 1, aggregateType, aggregateID, aggregateVersion,
		organizationID, correlationID, payload, organizationID == "")
	if err != nil {
		return err
	}
	if err = p.outbox.Append(ctx, tx, envelope); err != nil {
		return err
	}
	// Section 26.2 core events are derived in the same transaction. Their
	// payload is the minimized contract: subject, version and status only.
	if organizationID == "" {
		return nil
	}
	for _, alias := range CanonicalAliases(eventType) {
		canonical, err := NewEnvelope(alias.EventType, 1, alias.AggregateType, aggregateID, aggregateVersion,
			organizationID, correlationID, ContractPayload{SubjectReference: aggregateID, SourceVersion: aggregateVersion, Status: alias.Status}, false)
		if err != nil {
			return err
		}
		if err = p.outbox.Append(ctx, tx, canonical); err != nil {
			return err
		}
	}
	return nil
}
