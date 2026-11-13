package organizations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// FleetEventsPath is Fleet's ingest route for verification decisions.
const FleetEventsPath = "/internal/fleet-partner/v1/organization-verification/events"

// errPermanent marks a delivery Fleet refused for good (4xx other than 408
// and 429): retrying the same event cannot succeed.
var errPermanent = errors.New("organizations: fleet rejected the event permanently")

// FleetDeliveryWorker durably delivers organization verification events to
// fleet-partner-service. Fleet also reads decisions back, so a lost delivery
// only delays activation; a delivered one makes it immediate.
type FleetDeliveryWorker struct {
	db     *pgxpool.Pool
	url    string
	secret []byte
	client *http.Client
}

// NewFleetDeliveryWorker builds the worker for Fleet's base origin.
func NewFleetDeliveryWorker(db *pgxpool.Pool, fleetBaseURL, secret string, timeout time.Duration) *FleetDeliveryWorker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &FleetDeliveryWorker{db: db, url: strings.TrimRight(fleetBaseURL, "/") + FleetEventsPath,
		secret: []byte(secret), client: &http.Client{Timeout: timeout}}
}

type fleetRecord struct {
	ID             string
	EventType      string
	EventVersion   int64
	OccurredAt     time.Time
	OrganizationID string
	SubmissionID   string
	Payload        []byte
	Attempts       int
}

// Process delivers up to limit due events.
func (w *FleetDeliveryWorker) Process(ctx context.Context, owner string, limit int) error {
	if limit <= 0 {
		limit = 25
	}
	rows, err := w.db.Query(ctx, `WITH due AS (
		SELECT id FROM verification_outbox
		WHERE event_type IN ($3, $4)
		  AND delivery_status IN ('PENDING','FAILED') AND next_attempt_at<=now()
		  AND (lease_expires_at IS NULL OR lease_expires_at<now())
		ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE verification_outbox o SET lease_owner=$2, lease_expires_at=now()+interval '30 seconds',
		 delivery_attempts=delivery_attempts+1, updated_at=now()
		FROM due WHERE o.id=due.id
		RETURNING o.id::text, o.event_type, o.event_version, o.created_at, o.aggregate_id,
		 COALESCE(o.payload->>'submissionId',
		   (SELECT s.submission_id FROM organization_verification_submissions s
		     WHERE s.id::text = o.payload->>'externalReference'), ''),
		 o.payload, o.delivery_attempts`,
		limit, owner, SubmittedEventType, StatusChangedEventType)
	if err != nil {
		return err
	}
	var records []fleetRecord
	for rows.Next() {
		var r fleetRecord
		if err = rows.Scan(&r.ID, &r.EventType, &r.EventVersion, &r.OccurredAt, &r.OrganizationID,
			&r.SubmissionID, &r.Payload, &r.Attempts); err != nil {
			rows.Close()
			return err
		}
		records = append(records, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, record := range records {
		err := w.deliver(ctx, record)
		if err == nil {
			_, _ = w.db.Exec(ctx, `UPDATE verification_outbox SET delivery_status='DELIVERED',delivered_at=now(),
				lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=now() WHERE id=$1`, record.ID)
			continue
		}
		status, code := "FAILED", "FLEET_DELIVERY_FAILED"
		if errors.Is(err, errPermanent) || record.Attempts >= 8 {
			status = "DEAD_LETTER"
		}
		if errors.Is(err, errPermanent) {
			code = "FLEET_REJECTED"
		}
		backoff := time.Duration(1<<min(record.Attempts, 10)) * time.Minute
		_, _ = w.db.Exec(ctx, `UPDATE verification_outbox SET delivery_status=$2,next_attempt_at=now()+$3::interval,
			lease_owner=NULL,lease_expires_at=NULL,last_error=$4,updated_at=now() WHERE id=$1`,
			record.ID, status, fmt.Sprintf("%d seconds", int64(backoff.Seconds())), code)
	}
	return nil
}

// envelope builds the Section 61 envelope Fleet ingests.
func (w *FleetDeliveryWorker) envelope(record fleetRecord) ([]byte, error) {
	var stored map[string]any
	if err := json.Unmarshal(record.Payload, &stored); err != nil {
		return nil, err
	}
	issues := []map[string]any{}
	if raw, ok := stored["issues"].([]any); ok {
		for _, item := range raw {
			if issue, ok := item.(map[string]any); ok {
				issues = append(issues, issue)
			}
		}
	}
	return json.Marshal(map[string]any{
		"eventId":        record.ID,
		"eventType":      record.EventType,
		"eventVersion":   record.EventVersion,
		"occurredAt":     record.OccurredAt.UTC().Format(time.RFC3339Nano),
		"producer":       "verification-compliance-service",
		"organizationId": record.OrganizationID,
		"correlationId":  "verification:" + record.ID,
		"payload": map[string]any{
			"submissionId":      record.SubmissionID,
			"externalReference": stored["externalReference"],
			"status":            stored["status"],
			"restrictionStatus": stored["restrictionStatus"],
			"sourceVersion":     stored["sourceVersion"],
			"issues":            issues,
		},
	})
}

func (w *FleetDeliveryWorker) deliver(ctx context.Context, record fleetRecord) error {
	body, err := w.envelope(record)
	if err != nil {
		return fmt.Errorf("%w: %v", errPermanent, err)
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
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("fleet delivery returned %d", response.StatusCode)
	case response.StatusCode >= 400 && response.StatusCode < 500:
		return fmt.Errorf("%w: status %d", errPermanent, response.StatusCode)
	}
	return fmt.Errorf("fleet delivery returned %d", response.StatusCode)
}

// Run delivers until ctx ends.
func (w *FleetDeliveryWorker) Run(ctx context.Context, owner string, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = w.Process(ctx, owner, 25)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
