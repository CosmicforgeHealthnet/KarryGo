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
	return p.outbox.Append(ctx, tx, envelope)
}
