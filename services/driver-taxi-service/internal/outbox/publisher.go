package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	defaultLease       = 30 * time.Second
	defaultMaxAttempts = 8
	vendorEventPath    = "/internal/vendor/v1/commerce/logistics-events"
)

type Record struct {
	ID          string
	AggregateID string
	EventType   string
	Payload     json.RawMessage
	OccurredAt  time.Time
	Attempts    int
	// Target names the consumer that owns this row. Existing rows default to
	// the Vendor lifecycle ingress, so adding Fleet completed-work delivery
	// changes nothing about Taxi Goods.
	Target string
	// IdempotencyKey is the stable delivery identity. An ambiguous outcome is
	// always REDELIVERED under this same key, never re-minted (TAX-016A).
	IdempotencyKey string
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Claim(ctx context.Context, owner string, limit int, lease time.Duration) ([]Record, error) {
	if s == nil || s.db == nil || strings.TrimSpace(owner) == "" {
		return nil, errors.New("outbox publisher is not configured")
	}
	if limit < 1 {
		limit = 50
	}
	if lease <= 0 {
		lease = defaultLease
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT id FROM taxi_outbox
			 WHERE delivered_at IS NULL AND dead_lettered_at IS NULL
			   AND available_at <= now() AND (lease_until IS NULL OR lease_until <= now())
			 ORDER BY available_at,occurred_at,id
			 LIMIT $1 FOR UPDATE SKIP LOCKED
		)
		UPDATE taxi_outbox o
		   SET lease_owner=$2,lease_until=now()+$3::interval,attempts=attempts+1,last_error=''
		  FROM due WHERE o.id=due.id
		RETURNING o.id::text,o.aggregate_id::text,o.event_type,o.payload,o.occurred_at,o.attempts,
		          o.target,o.idempotency_key`,
		limit, owner, fmt.Sprintf("%f seconds", lease.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0, limit)
	for rows.Next() {
		var record Record
		if err = rows.Scan(&record.ID, &record.AggregateID, &record.EventType, &record.Payload,
			&record.OccurredAt, &record.Attempts, &record.Target, &record.IdempotencyKey); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) Ack(ctx context.Context, id, owner string) error {
	tag, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET delivered_at=now(),lease_owner=NULL,lease_until=NULL,
		last_error='',last_error_code='',reconciliation_state=$3,next_retry_at=NULL
		WHERE id=$1 AND lease_owner=$2 AND delivered_at IS NULL`, id, owner, ReconDelivered)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("outbox lease ownership was lost")
	}
	return nil
}

// Fail records one failed delivery attempt and decides what happens next
// (Section 8 TAX-016B, TAX-017, TAX-017A).
//
// Three outcomes, deliberately distinct:
//
//   - TERMINAL: Fleet authoritatively rejected the row (wrong source service,
//     wrong provider, invalid membership, invalid payload). No retry can fix
//     it, so it is dead-lettered immediately as FAILED_TERMINAL instead of
//     burning the whole attempt budget first.
//   - AMBIGUOUS with the budget spent: we never learned whether Fleet
//     committed. It becomes RECONCILIATION_REQUIRED, NOT a terminal failure --
//     the record may well exist at Fleet, and claiming otherwise would be a
//     guess an operator could act on.
//   - everything else: bounded exponential backoff, RETRY_SCHEDULED, and the
//     row is redelivered under the SAME idempotency key.
func (s *Store) Fail(ctx context.Context, record Record, owner string, cause error, maxAttempts int) error {
	if maxAttempts < 1 {
		maxAttempts = defaultMaxAttempts
	}
	message := "delivery failed"
	if cause != nil {
		message = cause.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}

	var terminal *TerminalDeliveryError
	var ambiguous *AmbiguousDeliveryError
	code := ""
	switch {
	case errors.As(cause, &terminal):
		code = terminal.Code
	case errors.As(cause, &ambiguous):
		code = ambiguous.Code
	}

	exhausted := record.Attempts >= maxAttempts
	if errors.As(cause, &terminal) || (exhausted && !errors.As(cause, &ambiguous)) {
		state := ReconFailedTerminal
		_, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET dead_lettered_at=now(),lease_owner=NULL,lease_until=NULL,
			last_error=$3,last_error_code=$4,reconciliation_state=$5,next_retry_at=NULL
			WHERE id=$1 AND lease_owner=$2`, record.ID, owner, message, code, state)
		return err
	}
	if exhausted {
		// Ambiguous and out of attempts: Fleet may hold the record. Park it
		// for reconciliation rather than asserting a failure that may be false.
		_, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET dead_lettered_at=now(),lease_owner=NULL,lease_until=NULL,
			last_error=$3,last_error_code=$4,reconciliation_state=$5,next_retry_at=NULL
			WHERE id=$1 AND lease_owner=$2`, record.ID, owner, message, code, ReconReconciliationRequired)
		return err
	}

	shift := record.Attempts - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 8 {
		shift = 8
	}
	backoff := time.Second * time.Duration(1<<shift)
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	_, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET available_at=now()+$3::interval,lease_owner=NULL,lease_until=NULL,
		last_error=$4,last_error_code=$5,reconciliation_state=$6,next_retry_at=now()+$3::interval
		WHERE id=$1 AND lease_owner=$2`,
		record.ID, owner, fmt.Sprintf("%f seconds", backoff.Seconds()), message, code, ReconRetryScheduled)
	return err
}

// ListReconciliation returns completed-work rows in a given reconciliation
// state, newest first. This is the operator read surface TAX-017 requires:
// repairing a stuck completion must never mean editing the database by hand.
func (s *Store) ListReconciliation(ctx context.Context, target, state string, limit int) ([]ReconciliationRow, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, payload, reconciliation_state, attempts,
		       last_error_code, last_error, next_retry_at, occurred_at, delivered_at, idempotency_key
		  FROM taxi_outbox
		 WHERE target = $1 AND ($2 = '' OR reconciliation_state = $2)
		 ORDER BY occurred_at DESC
		 LIMIT $3`, target, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReconciliationRow, 0, limit)
	for rows.Next() {
		var row ReconciliationRow
		var payload []byte
		if err := rows.Scan(&row.ID, &payload, &row.ReconciliationState, &row.Attempts,
			&row.LastErrorCode, &row.LastError, &row.NextRetryAt, &row.OccurredAt,
			&row.DeliveredAt, &row.IdempotencyKey); err != nil {
			return nil, err
		}
		// Only the operational identifiers are surfaced. The payload's
		// monetary basis is organization-internal and has no business on an
		// operator delivery view.
		var decoded CompletedWorkPayload
		if json.Unmarshal(payload, &decoded) == nil {
			row.SourceJobID = decoded.SourceJobID
			row.SourceAssignmentID = decoded.SourceAssignmentID
			row.OrganizationID = decoded.OrganizationID
			row.ProviderID = decoded.ProviderID
			row.CorrelationID = decoded.CorrelationID
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Requeue returns a dead-lettered row to the delivery queue (TAX-017).
//
// The row's payload and idempotency key are untouched, so this is a
// REDELIVERY of the original completion, never a new completion event. That
// is what makes operator repair safe for a RECONCILIATION_REQUIRED row whose
// record may already exist at Fleet: Fleet returns the existing result.
func (s *Store) Requeue(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE taxi_outbox
		   SET dead_lettered_at=NULL, available_at=now(), attempts=0,
		       lease_owner=NULL, lease_until=NULL, reconciliation_state=$2,
		       next_retry_at=NULL
		 WHERE id=$1::uuid AND delivered_at IS NULL`, id, ReconPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("outbox row is not requeueable")
	}
	return nil
}

type Destination interface {
	Deliver(context.Context, Record) error
}

type Publisher struct {
	store       *Store
	destination Destination
	owner       string
	lease       time.Duration
	maxAttempts int
}

func NewPublisher(store *Store, destination Destination, owner string) *Publisher {
	return &Publisher{store: store, destination: destination, owner: owner, lease: defaultLease, maxAttempts: defaultMaxAttempts}
}

func (p *Publisher) ProcessBatch(ctx context.Context, limit int) error {
	if p == nil || p.store == nil || p.destination == nil {
		return errors.New("outbox publisher is not configured")
	}
	records, err := p.store.Claim(ctx, p.owner, limit, p.lease)
	if err != nil {
		return err
	}
	var failures []error
	for _, record := range records {
		if deliveryErr := p.destination.Deliver(ctx, record); deliveryErr != nil {
			failures = append(failures, deliveryErr)
			if failErr := p.store.Fail(ctx, record, p.owner, deliveryErr, p.maxAttempts); failErr != nil {
				failures = append(failures, failErr)
			}
			continue
		}
		if ackErr := p.store.Ack(ctx, record.ID, p.owner); ackErr != nil {
			failures = append(failures, ackErr)
		}
	}
	return errors.Join(failures...)
}

func (p *Publisher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		_ = p.ProcessBatch(ctx, 50)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type VendorClient struct {
	BaseURL    string
	Secret     []byte
	HTTPClient *http.Client
	Now        func() time.Time
}

type lifecyclePayload struct {
	MissionRef      string          `json:"mission_ref"`
	ParentOrderID   string          `json:"parent_order_id"`
	VendorOrderIDs  []string        `json:"vendor_order_ids"`
	SourceVersion   int64           `json:"source_version"`
	VendorEventType string          `json:"vendor_event_type"`
	Payload         json.RawMessage `json:"-"`
}

func (c VendorClient) Deliver(ctx context.Context, record Record) error {
	if strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return errors.New("Vendor lifecycle destination is not configured")
	}
	var lifecycle lifecyclePayload
	if err := json.Unmarshal(record.Payload, &lifecycle); err != nil {
		return err
	}
	if lifecycle.MissionRef == "" || lifecycle.ParentOrderID == "" || lifecycle.SourceVersion < 1 || len(lifecycle.VendorOrderIDs) == 0 {
		return errors.New("Taxi Goods lifecycle envelope is incomplete")
	}
	if lifecycle.VendorEventType == "" {
		lifecycle.VendorEventType = record.EventType
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	for _, vendorOrderID := range lifecycle.VendorOrderIDs {
		body, err := json.Marshal(map[string]any{
			"event_id": record.ID + ":" + vendorOrderID, "mission_ref": lifecycle.MissionRef,
			"vendor_order_id": vendorOrderID, "parent_order_id": lifecycle.ParentOrderID,
			"event_type": lifecycle.VendorEventType, "source_version": lifecycle.SourceVersion,
			"occurred_at": record.OccurredAt, "payload": json.RawMessage(record.Payload),
		})
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+vendorEventPath, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		if err = serviceauth.SignRequest(request, "driver-taxi-service", c.Secret, body, now().UTC()); err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("Vendor lifecycle ingress returned %d", response.StatusCode)
		}
	}
	return nil
}
