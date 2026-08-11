package vendors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// CallbackWorker durably delivers authoritative Vendor decisions. Vendor also
// performs read-back reconciliation, so callback loss and polling loss repair
// one another without weakening verification authority.
type CallbackWorker struct {
	db     *pgxpool.Pool
	url    string
	secret []byte
	client *http.Client
}

func NewCallbackWorker(db *pgxpool.Pool, vendorBaseURL, secret string, timeout time.Duration) *CallbackWorker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CallbackWorker{db: db, url: strings.TrimRight(vendorBaseURL, "/") + "/internal/vendor/v1/verification-events", secret: []byte(secret), client: &http.Client{Timeout: timeout}}
}

type callbackRecord struct {
	ID       string
	Payload  []byte
	Attempts int
}

func (w *CallbackWorker) Process(ctx context.Context, owner string, limit int) error {
	if limit <= 0 {
		limit = 25
	}
	rows, err := w.db.Query(ctx, `WITH due AS (
		SELECT id FROM verification_outbox
		WHERE event_type='vendor.verification.status_changed'
		  AND delivery_status IN ('PENDING','FAILED') AND next_attempt_at<=now()
		  AND (lease_expires_at IS NULL OR lease_expires_at<now())
		ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE verification_outbox o SET lease_owner=$2,lease_expires_at=now()+interval '30 seconds',
		 delivery_attempts=delivery_attempts+1,updated_at=now()
		FROM due WHERE o.id=due.id RETURNING o.id::text,o.payload,o.delivery_attempts`, limit, owner)
	if err != nil {
		return err
	}
	var records []callbackRecord
	for rows.Next() {
		var r callbackRecord
		if err = rows.Scan(&r.ID, &r.Payload, &r.Attempts); err != nil {
			rows.Close()
			return err
		}
		records = append(records, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, record := range records {
		if err = w.deliver(ctx, record); err == nil {
			_, _ = w.db.Exec(ctx, `UPDATE verification_outbox SET delivery_status='DELIVERED',delivered_at=now(),lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=now() WHERE id=$1`, record.ID)
			continue
		}
		status := "FAILED"
		if record.Attempts >= 8 {
			status = "DEAD_LETTER"
		}
		backoff := time.Duration(1<<min(record.Attempts, 10)) * time.Minute
		_, _ = w.db.Exec(ctx, `UPDATE verification_outbox SET delivery_status=$2,next_attempt_at=now()+$3::interval,lease_owner=NULL,lease_expires_at=NULL,last_error=$4,updated_at=now() WHERE id=$1`, record.ID, status, fmt.Sprintf("%d seconds", int64(backoff.Seconds())), "VENDOR_CALLBACK_FAILED")
	}
	return nil
}

func (w *CallbackWorker) deliver(ctx context.Context, record callbackRecord) error {
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return err
	}
	payload["event_id"] = record.ID
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = serviceauth.SignRequest(req, "verification-compliance-service", w.secret, body, time.Now()); err != nil {
		return err
	}
	response, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("vendor callback returned status %d", response.StatusCode)
	}
	return nil
}

// Replay makes an operator-selected dead letter eligible immediately.
func (w *CallbackWorker) Replay(ctx context.Context, eventID string) error {
	tag, err := w.db.Exec(ctx, `UPDATE verification_outbox SET delivery_status='PENDING',delivery_attempts=0,next_attempt_at=now(),lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=now() WHERE id=$1 AND event_type='vendor.verification.status_changed' AND delivery_status='DEAD_LETTER'`, eventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("vendor callback dead letter not found")
	}
	return nil
}
