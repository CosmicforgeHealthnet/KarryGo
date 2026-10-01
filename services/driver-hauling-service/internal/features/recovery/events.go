package recovery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"
)

// Event names.
//
// The authority-prefixed spelling is deliberate and is NOT the "network.*"
// form a first reading of the catalogue suggests. driver-taxi-service already
// publishes taxi.recovery.started, taxi.recovery.provider_replaced,
// taxi.recovery.resolved and taxi.guarantee.breached, and those are live with
// real consumers. Introducing a second, differently-prefixed vocabulary for
// the identical facts would leave every consumer having to know which
// authority speaks which dialect -- so Hauling joins the established one.
const (
	EventRecoveryStarted          = "hauling.recovery.started"
	EventRecoveryProviderReplaced = "hauling.recovery.provider_replaced"
	EventRecoveryResolved         = "hauling.recovery.resolved"
	EventRecoveryFailed           = "hauling.recovery.failed"
	EventGuaranteeBreached        = "hauling.guarantee.breached"
	EventCapabilityMismatch       = "hauling.capability_mismatch"
)

// SchemaVersion is the payload schema these events carry.
const SchemaVersion = 1

// Event is one recovery fact.
type Event struct {
	Name          string
	AggregateID   string
	Version       int64
	CorrelationID string
	Payload       map[string]any
}

// OutboxSink publishes recovery facts through hauling_domain_outbox.
//
// It reuses the outbox the completed-work path already rides rather than
// adding a table: the delivery worker, the lease, the backoff and the
// dead-letter handling all exist there and are proven. A second outbox would
// be a second thing to monitor and a second place for a delivery bug to live.
type OutboxSink struct {
	db      *pgxpool.Pool
	target  string
	enabled bool
	now     func() time.Time
}

// NewOutboxSink builds the sink. A disabled sink is a silent no-op, so a
// deployment that has not wired event delivery still recovers loads -- it just
// does not tell anyone about it.
func NewOutboxSink(db *pgxpool.Pool, target string, enabled bool, now func() time.Time) *OutboxSink {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if target == "" {
		target = "analytics-service"
	}
	return &OutboxSink{db: db, target: target, enabled: enabled, now: now}
}

// PublishTx enqueues an event inside a caller-owned transaction.
//
// A nil transaction opens its own. Both paths exist because the two callers
// genuinely differ: a state advance that already owns a transaction must
// enqueue inside it, so the fact and the state it describes commit together;
// a caller that has already committed needs the event enqueued anyway, and
// losing it would be worse than enqueueing it a moment late.
func (s *OutboxSink) PublishTx(ctx context.Context, tx pgx.Tx, event Event) error {
	if s == nil || !s.enabled || s.db == nil {
		return nil
	}
	payload := event.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	// The envelope fields every consumer relies on for idempotency and
	// ordering. event_id is generated here rather than derived from the
	// aggregate so a genuine re-publication of the same fact is a new event,
	// while the outbox's own (aggregate, version, type) uniqueness is what
	// makes an accidental double-enqueue a no-op.
	payload["event_id"] = uuid.NewString()
	payload["schema_version"] = SchemaVersion
	payload["source_service"] = "driver-hauling-service"
	payload["source_entity_type"] = "recovery_case"
	payload["source_entity_id"] = event.AggregateID
	payload["source_version"] = event.Version
	payload["occurred_at"] = s.now().UTC().Format(time.RFC3339Nano)
	if event.CorrelationID != "" {
		payload["correlation_id"] = event.CorrelationID
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	const statement = `
INSERT INTO hauling_domain_outbox
	(aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id, target)
VALUES ('recovery_case', $1, $2, $3, $4, $5, $6)
ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING`

	if tx != nil {
		_, err = tx.Exec(ctx, statement, event.AggregateID, event.Name, event.Version,
			encoded, event.CorrelationID, s.target)
		return err
	}
	_, err = s.db.Exec(ctx, statement, event.AggregateID, event.Name, event.Version,
		encoded, event.CorrelationID, s.target)
	return err
}

// publish is the service's convenience wrapper.
//
// Event delivery must never fail a recovery. A load whose replacement was
// bound but whose event did not enqueue is still a recovered load; refusing
// the recovery because analytics was unavailable would be strictly worse.
func (s *Service) publish(ctx context.Context, tx pgx.Tx, name string, current Case, extra map[string]any) {
	// Section 14: people are told only about committed facts. Inside a
	// caller's transaction the fact may still roll back, so notification
	// waits for the post-commit path -- which is every call today.
	if tx == nil {
		defer s.notify(ctx, name, current)
	}
	if s.events == nil {
		return
	}
	payload := map[string]any{
		"recovery_case_id": current.ID,
		"booking_id":       current.BookingID,
		"state":            string(current.State),
		"trigger_code":     current.TriggerCode,
	}
	if current.MissionRef != "" {
		payload["mission_ref"] = current.MissionRef
	}
	if current.ReasonCode != "" {
		payload["reason_code"] = current.ReasonCode
	}
	// Provider and truck ids are included because the consumer is the
	// platform's own analytics and operations plane, and a recovery fact with
	// no subject cannot be acted on. No customer data travels here: no name,
	// no phone, no address, no cargo description.
	if current.ReplacementProviderID != "" {
		payload["replacement_provider_id"] = current.ReplacementProviderID
	}
	if current.OriginalProviderID != "" {
		payload["original_provider_id"] = current.OriginalProviderID
	}
	for key, value := range extra {
		payload[key] = value
	}

	if err := s.events.PublishTx(ctx, tx, Event{
		Name: name, AggregateID: current.ID, Version: current.Version,
		CorrelationID: current.CorrelationID, Payload: payload,
	}); err != nil {
		logging.Error("hauling-recovery", "publish %s for case %s: %v", name, current.ID, err)
	}
}
