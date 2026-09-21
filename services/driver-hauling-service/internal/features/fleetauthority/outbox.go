package fleetauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxWorker struct {
	DB     *pgxpool.Pool
	URL    string
	Secret []byte
	Client *http.Client
}
type outboxRow struct {
	ID, AggregateType, AggregateID, EventType, CorrelationID string
	Version                                                  int64
	Attempts                                                 int
	Occurred                                                 time.Time
	Payload                                                  json.RawMessage
}

func (w *OutboxWorker) Run(ctx context.Context) {
	if w.Client == nil {
		w.Client = &http.Client{Timeout: 10 * time.Second}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.runBatch(ctx)
		}
	}
}

func (w *OutboxWorker) runBatch(ctx context.Context) error {
	rows, err := w.DB.Query(ctx, `UPDATE hauling_domain_outbox SET lease_owner='hauling-outbox',lease_until=now()+interval '30 seconds',attempts=attempts+1 WHERE id IN (SELECT id FROM hauling_domain_outbox WHERE target='fleet-events' AND delivered_at IS NULL AND dead_lettered_at IS NULL AND attempts<12 AND available_at<=now() AND (lease_until IS NULL OR lease_until<now()) ORDER BY occurred_at FOR UPDATE SKIP LOCKED LIMIT 25) RETURNING id::text,aggregate_type,aggregate_id::text,event_type,event_version,payload,correlation_id,occurred_at,attempts`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Version, &r.Payload, &r.CorrelationID, &r.Occurred, &r.Attempts); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	for _, r := range batch {
		if err := w.deliver(ctx, r); err != nil {
			_, _ = w.DB.Exec(ctx, `UPDATE hauling_domain_outbox SET lease_owner=NULL,lease_until=NULL,available_at=now()+LEAST(attempts,10)*interval '10 seconds',last_error=$2,dead_lettered_at=CASE WHEN attempts>=12 THEN now() ELSE NULL END WHERE id=$1`, r.ID, err.Error())
			continue
		}
		_, _ = w.DB.Exec(ctx, `UPDATE hauling_domain_outbox SET delivered_at=now(),lease_owner=NULL,lease_until=NULL,last_error=NULL WHERE id=$1`, r.ID)
	}
	return rows.Err()
}
func (w *OutboxWorker) deliver(ctx context.Context, r outboxRow) error {
	var payload map[string]any
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		return err
	}
	sum := sha256.Sum256(r.Payload)
	body, err := json.Marshal(map[string]any{"eventId": r.ID, "eventType": r.EventType, "schemaVersion": 1, "sourceService": "driver-hauling-service", "aggregateType": strings.ToLower(r.AggregateType), "aggregateId": r.AggregateID, "aggregateVersion": r.Version, "occurredAt": r.Occurred, "correlationId": r.CorrelationID, "payloadHash": hex.EncodeToString(sum[:]), "payload": payload})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Nonce", r.ID)
	if err = serviceauth.SignRequest(req, "driver-hauling-service", w.Secret, body, time.Time{}); err != nil {
		return err
	}
	resp, err := w.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fleet event status %d", resp.StatusCode)
	}
	return nil
}

// RunBatchForTest runs one delivery pass. Exposed for the cross-worker target
// isolation test; production code calls Run.
func (w *OutboxWorker) RunBatchForTest(ctx context.Context) { _ = w.runBatch(ctx) }
