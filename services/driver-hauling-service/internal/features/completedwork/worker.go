package completedwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/providercompat"
)

// Submitter is the slice of providercompat.Client this package calls.
type Submitter interface {
	SubmitCompletedWork(ctx context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error)
}

const (
	defaultBatchSize   = 25
	defaultLease       = 2 * time.Minute
	defaultMaxAttempts = 10
	defaultBaseBackoff = 5 * time.Second
	defaultMaxBackoff  = 15 * time.Minute
)

// Worker drains canonical completed-work rows from the existing hauling
// outbox to Fleet.
//
// It rides hauling_domain_outbox rather than a new table (HLG-018): the same
// leasing, attempt counting and dead-lettering that the Fleet domain-event
// worker already gets, with only the destination and the outcome
// classification differing.
type Worker struct {
	DB          *pgxpool.Pool
	Client      Submitter
	Enabled     bool
	BatchSize   int
	Lease       time.Duration
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Now         func() time.Time
}

func (w *Worker) withDefaults() {
	if w.BatchSize <= 0 {
		w.BatchSize = defaultBatchSize
	}
	if w.Lease <= 0 {
		w.Lease = defaultLease
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = defaultMaxAttempts
	}
	if w.BaseBackoff <= 0 {
		w.BaseBackoff = defaultBaseBackoff
	}
	if w.MaxBackoff <= 0 {
		w.MaxBackoff = defaultMaxBackoff
	}
	if w.Now == nil {
		w.Now = func() time.Time { return time.Now().UTC() }
	}
}

// backoffFor computes bounded exponential backoff (HLG-018).
func (w *Worker) backoffFor(attempt int) time.Duration {
	delay := w.BaseBackoff
	for i := 0; i < attempt && delay < w.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > w.MaxBackoff {
		delay = w.MaxBackoff
	}
	return delay
}

// Run drains the queue on an interval until the context ends.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	w.withDefaults()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	_ = w.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.RunOnce(ctx)
		}
	}
}

type row struct {
	ID       string
	Payload  json.RawMessage
	Attempts int
}

// RunOnce processes one batch.
//
// The claim commits before any HTTP call, so no database transaction is open
// while waiting on Fleet. A crash mid-delivery releases the lease by timeout
// rather than stranding the completion.
func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil || w.DB == nil {
		return nil
	}
	w.withDefaults()
	if !w.Enabled || w.Client == nil {
		return nil
	}
	rows, err := w.DB.Query(ctx, `
		UPDATE hauling_domain_outbox
		   SET lease_owner='hauling-completed-work',
		       lease_until=now()+$2::interval,
		       attempts=attempts+1
		 WHERE id IN (
			SELECT id FROM hauling_domain_outbox
			 WHERE target=$1 AND delivered_at IS NULL AND dead_lettered_at IS NULL
			   AND available_at<=now()
			   AND (lease_until IS NULL OR lease_until<now())
			 ORDER BY occurred_at
			 FOR UPDATE SKIP LOCKED
			 LIMIT $3)
		RETURNING id::text, payload, attempts`,
		Target, fmt.Sprintf("%f seconds", w.Lease.Seconds()), w.BatchSize)
	if err != nil {
		return err
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Payload, &r.Attempts); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var failures []error
	for _, r := range batch {
		if ctx.Err() != nil {
			return nil
		}
		if err := w.deliverOne(ctx, r); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (w *Worker) deliverOne(ctx context.Context, r row) error {
	// The request is rebuilt ENTIRELY from the stored payload, never from the
	// live booking: a delivery replayed days later must reproduce exactly what
	// was completed, under exactly the same idempotency key (HLG-019).
	var request providercompat.CompletedWorkRequest
	if err := json.Unmarshal(r.Payload, &request); err != nil {
		return w.markTerminal(ctx, r.ID, "payload_decode_failed", err.Error(), ReconFailedTerminal)
	}
	request.CompletedAt = request.CompletedAt.UTC()

	result, submitErr := w.Client.SubmitCompletedWork(ctx, request)
	switch result.Outcome {
	case providercompat.SubmitAccepted, providercompat.SubmitDuplicate:
		// DUPLICATE is a success: it is precisely what a redelivery after a
		// lost response returns, and it proves Fleet holds the record once.
		return w.markDelivered(ctx, r.ID)

	case providercompat.SubmitTerminal:
		// Fleet authoritatively rejected it. No retry can fix a wrong source
		// service, a wrong provider or an invalid membership, so dead-letter
		// immediately instead of burning the whole attempt budget first.
		return w.markTerminal(ctx, r.ID, codeOr(result.Code, "rejected"), errText(submitErr), ReconFailedTerminal)

	case providercompat.SubmitUnknown:
		// We never learned whether Fleet committed. Retry as a REDELIVERY
		// under the same key; once the budget is spent this becomes
		// RECONCILIATION_REQUIRED, not a claimed failure, because Fleet may
		// in fact hold the record (HLG-020).
		if r.Attempts >= w.MaxAttempts {
			return w.markTerminal(ctx, r.ID, codeOr(result.Code, "unknown"), errText(submitErr), ReconReconciliationRequired)
		}
		return w.markRetry(ctx, r.ID, r.Attempts, codeOr(result.Code, "unknown"), errText(submitErr))

	default:
		if r.Attempts >= w.MaxAttempts {
			return w.markTerminal(ctx, r.ID, codeOr(result.Code, "retryable"), errText(submitErr), ReconFailedTerminal)
		}
		return w.markRetry(ctx, r.ID, r.Attempts, codeOr(result.Code, "retryable"), errText(submitErr))
	}
}

func (w *Worker) markDelivered(ctx context.Context, id string) error {
	_, err := w.DB.Exec(ctx, `
		UPDATE hauling_domain_outbox
		   SET delivered_at=now(), lease_owner=NULL, lease_until=NULL,
		       last_error=NULL, last_error_code='', reconciliation_state=$2,
		       next_retry_at=NULL
		 WHERE id=$1::uuid`, id, ReconDelivered)
	return err
}

func (w *Worker) markRetry(ctx context.Context, id string, attempts int, code, message string) error {
	backoff := w.backoffFor(attempts)
	_, err := w.DB.Exec(ctx, `
		UPDATE hauling_domain_outbox
		   SET available_at=now()+$2::interval, next_retry_at=now()+$2::interval,
		       lease_owner=NULL, lease_until=NULL,
		       last_error=$3, last_error_code=$4, reconciliation_state=$5
		 WHERE id=$1::uuid`,
		id, fmt.Sprintf("%f seconds", backoff.Seconds()), truncate(message), code, ReconRetryScheduled)
	return err
}

func (w *Worker) markTerminal(ctx context.Context, id, code, message, state string) error {
	_, err := w.DB.Exec(ctx, `
		UPDATE hauling_domain_outbox
		   SET dead_lettered_at=now(), lease_owner=NULL, lease_until=NULL,
		       last_error=$2, last_error_code=$3, reconciliation_state=$4,
		       next_retry_at=NULL
		 WHERE id=$1::uuid`, id, truncate(message), code, state)
	return err
}

// ReconciliationRow is the operator-facing view of one completed-work
// delivery (HLG-020).
type ReconciliationRow struct {
	ID                  string     `json:"id"`
	SourceJobID         string     `json:"sourceJobId"`
	SourceAssignmentID  string     `json:"sourceAssignmentId"`
	OrganizationID      string     `json:"organizationId"`
	ProviderID          string     `json:"providerId"`
	IdempotencyKey      string     `json:"idempotencyKey"`
	CorrelationID       string     `json:"correlationId"`
	ReconciliationState string     `json:"reconciliationState"`
	Attempts            int        `json:"attempts"`
	LastErrorCode       string     `json:"lastErrorCode"`
	LastError           string     `json:"lastError"`
	NextRetryAt         *time.Time `json:"nextRetryAt,omitempty"`
	OccurredAt          time.Time  `json:"occurredAt"`
	DeliveredAt         *time.Time `json:"deliveredAt,omitempty"`
}

// Store is the operator read/repair surface over the outbox.
type Store struct{ DB *pgxpool.Pool }

// List returns completed-work rows in a given reconciliation state, newest
// first. Repairing a stuck completion must never mean editing the database by
// hand, which is what this read plus Requeue below exist to avoid.
func (s *Store) List(ctx context.Context, state string, limit int) ([]ReconciliationRow, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id::text, payload, reconciliation_state, attempts,
		       last_error_code, COALESCE(last_error,''), next_retry_at,
		       occurred_at, delivered_at, idempotency_key
		  FROM hauling_domain_outbox
		 WHERE target=$1 AND ($2='' OR reconciliation_state=$2)
		 ORDER BY occurred_at DESC
		 LIMIT $3`, Target, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReconciliationRow, 0, limit)
	for rows.Next() {
		var item ReconciliationRow
		var payload []byte
		if err := rows.Scan(&item.ID, &payload, &item.ReconciliationState, &item.Attempts,
			&item.LastErrorCode, &item.LastError, &item.NextRetryAt,
			&item.OccurredAt, &item.DeliveredAt, &item.IdempotencyKey); err != nil {
			return nil, err
		}
		// Only operational identifiers are surfaced. The payload's monetary
		// basis is organization-internal and has no business on an operator
		// delivery view.
		var decoded providercompat.CompletedWorkRequest
		if json.Unmarshal(payload, &decoded) == nil {
			item.SourceJobID = decoded.SourceJobID
			item.SourceAssignmentID = decoded.SourceAssignmentID
			item.OrganizationID = decoded.OrganizationID
			item.ProviderID = decoded.ProviderID
			item.CorrelationID = decoded.CorrelationID
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Requeue returns a dead-lettered row to the delivery queue (HLG-020).
//
// The row's payload and idempotency key are untouched, so this is a
// REDELIVERY of the original completion, never a new completion event. That
// is what makes operator repair safe for a RECONCILIATION_REQUIRED row whose
// record may already exist at Fleet: Fleet returns the existing result.
func (s *Store) Requeue(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `
		UPDATE hauling_domain_outbox
		   SET dead_lettered_at=NULL, available_at=now(), attempts=0,
		       lease_owner=NULL, lease_until=NULL,
		       reconciliation_state=$2, next_retry_at=NULL
		 WHERE id=$1::uuid AND target=$3 AND delivered_at IS NULL`,
		id, ReconPending, Target)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("completed-work row is not requeueable")
	}
	return nil
}

func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

func errText(err error) string {
	if err == nil {
		return "delivery failed"
	}
	return err.Error()
}

func truncate(message string) string {
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}
