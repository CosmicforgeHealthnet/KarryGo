package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
// The outbox contains a Vendor envelope around the shared envelope; the
// grouping-only HTTP publisher unwraps it for Customer's contract.
type customerGroupingPublisher struct {
	db     *pgxpool.Pool
	outbox *events.OutboxRepository
}

func (p *customerGroupingPublisher) PublishGrouping(ctx context.Context, parentOrderID string, grouping sharedevents.MovementGrouping) error {
	return p.publish(ctx, parentOrderID, grouping, false)
}

// PublishCurrentGrouping reads the immutable active plan and its linked
// authority-owned references under the same locks used by historical repair.
// No execution state is inferred from commercial order status.
func (p *customerGroupingPublisher) PublishCurrentGrouping(ctx context.Context, parentOrderID string) error {
	if p == nil || p.db == nil || p.outbox == nil {
		return nil
	}
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentOrderID); err != nil {
		return err
	}
	grouping, _, valid, err := canonicalHistoricalGrouping(ctx, tx, parentOrderID)
	if err != nil {
		return err
	}
	if !valid {
		return nil
	}
	raw, err := json.Marshal(grouping)
	if err != nil {
		return err
	}
	if err = p.publishTx(ctx, tx, parentOrderID, grouping, raw, sha256.Sum256(raw), false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *customerGroupingPublisher) publish(ctx context.Context, parentOrderID string, grouping sharedevents.MovementGrouping, onlyMissing bool) error {
	if p == nil || p.db == nil || p.outbox == nil {
		return nil
	}
	if err := grouping.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(grouping)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	tx, err := p.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentOrderID); err != nil {
		return err
	}
	if err = p.publishTx(ctx, tx, parentOrderID, grouping, raw, digest, onlyMissing); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *customerGroupingPublisher) publishTx(ctx context.Context, tx pgx.Tx, parentOrderID string, grouping sharedevents.MovementGrouping, raw []byte, digest [32]byte, onlyMissing bool) error {
	var version int64
	conflict := `DO UPDATE SET source_version=commerce_customer_grouping_emissions.source_version+1,fingerprint=EXCLUDED.fingerprint,updated_at=now() WHERE commerce_customer_grouping_emissions.fingerprint<>EXCLUDED.fingerprint`
	if onlyMissing {
		conflict = `DO NOTHING`
	}
	err := tx.QueryRow(ctx, `INSERT INTO commerce_customer_grouping_emissions(parent_order_id,source_version,fingerprint)
	 VALUES($1,$2,$3) ON CONFLICT(parent_order_id) `+conflict+` RETURNING source_version`, parentOrderID, time.Now().UTC().UnixMicro(), hex.EncodeToString(digest[:])).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	envelope, err := sharedevents.NewGroupingEnvelope(uuid.NewString(), "vendor-service", parentOrderID,
		version, time.Now().UTC(), parentOrderID, grouping)
	if err != nil {
		return err
	}
	outboxEnvelope, err := events.NewEnvelope(sharedevents.CommerceGroupingEventName, 1, "commerce_order", parentOrderID,
		envelope.SourceVersion, "", parentOrderID, "", envelope, true)
	if err != nil {
		return err
	}
	if err = p.outbox.Append(ctx, tx, outboxEnvelope); err != nil {
		return err
	}
	return nil
}

// The generic Vendor HTTP publisher sends a Vendor envelope. Customer's
// grouping ingress expects the nested shared envelope, with its own event ID.
type customerGroupingHTTPPublisher struct{ inner events.Publisher }

func (p customerGroupingHTTPPublisher) Publish(ctx context.Context, record events.OutboxRecord) error {
	var outer struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(record.Payload, &outer); err != nil {
		return err
	}
	var envelope sharedevents.Envelope
	if err := json.Unmarshal(outer.Payload, &envelope); err != nil {
		return err
	}
	if envelope.EventName != sharedevents.CommerceGroupingEventName || envelope.Validate() != nil {
		return fmt.Errorf("invalid customer grouping envelope")
	}
	record.Payload = outer.Payload
	return p.inner.Publish(ctx, record)
}
