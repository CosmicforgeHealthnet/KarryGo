package main

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	sharedevents "cosmicforge/logistics/shared/go/events"
)

// customerGroupingPublisher queues the commerce grouping of one purchase in
// the existing transactional outbox, so delivery to the customer context
// owner is durable and retried rather than a best-effort HTTP call made
// inside an order activation.
//
// The queued payload IS the shared movement envelope, because the outbox's
// HTTP publisher posts a record's payload verbatim: customer-service
// therefore receives exactly the contract it validates.
type customerGroupingPublisher struct {
	db     *pgxpool.Pool
	outbox *events.OutboxRepository
}

func (p *customerGroupingPublisher) PublishGrouping(ctx context.Context, parentOrderID string, grouping sharedevents.MovementGrouping) error {
	if p == nil || p.db == nil || p.outbox == nil {
		return nil
	}
	envelope, err := sharedevents.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parentOrderID,
		time.Now().UTC().Unix(), time.Now().UTC(), parentOrderID, grouping)
	if err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	outboxEnvelope, err := events.NewEnvelope(sharedevents.CommerceGroupingEventName, 1, "commerce_order", parentOrderID,
		envelope.SourceVersion, "", parentOrderID, "", envelope, true)
	if err != nil {
		return err
	}
	if err = p.outbox.Append(ctx, tx, outboxEnvelope); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
