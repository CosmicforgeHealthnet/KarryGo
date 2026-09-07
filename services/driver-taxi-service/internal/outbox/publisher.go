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
		RETURNING o.id::text,o.aggregate_id::text,o.event_type,o.payload,o.occurred_at,o.attempts`,
		limit, owner, fmt.Sprintf("%f seconds", lease.Seconds()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0, limit)
	for rows.Next() {
		var record Record
		if err = rows.Scan(&record.ID, &record.AggregateID, &record.EventType, &record.Payload, &record.OccurredAt, &record.Attempts); err != nil {
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
	tag, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET delivered_at=now(),lease_owner=NULL,lease_until=NULL,last_error='' WHERE id=$1 AND lease_owner=$2 AND delivered_at IS NULL`, id, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("outbox lease ownership was lost")
	}
	return nil
}

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
	if record.Attempts >= maxAttempts {
		_, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET dead_lettered_at=now(),lease_owner=NULL,lease_until=NULL,last_error=$3 WHERE id=$1 AND lease_owner=$2`, record.ID, owner, message)
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
	_, err := s.db.Exec(ctx, `UPDATE taxi_outbox SET available_at=now()+$3::interval,lease_owner=NULL,lease_until=NULL,last_error=$4 WHERE id=$1 AND lease_owner=$2`, record.ID, owner, fmt.Sprintf("%f seconds", backoff.Seconds()), message)
	return err
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
