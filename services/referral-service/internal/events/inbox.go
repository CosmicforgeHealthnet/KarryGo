// Package events holds referral-service's event-intake plumbing: the inbox
// that deduplicates deliveries and the nonce store that rejects replayed
// signed requests.
package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Inbox statuses.
const (
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
	StatusRejected   = "REJECTED"
	StatusFailed     = "FAILED"
)

// ErrDuplicateEvent means the event was already fully processed; acknowledge
// the delivery without reprocessing.
var ErrDuplicateEvent = errors.New("inbox: event already processed")

// ErrPayloadMismatch means an event ID arrived again with a different body --
// a contract or security fault that must never be processed.
var ErrPayloadMismatch = errors.New("inbox: payload differs from the first delivery of this event id")

// HashPayload is the canonical payload hash stored per event.
func HashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Inbox deduplicates received events.
//
// Begin and MarkProcessed take the caller's transaction, so the dedupe
// record commits with the business effect or not at all. (An inbox that
// commits its record separately can mark an event processed whose effect
// then rolled back -- the event would be silently lost.)
type Inbox struct {
	db database.Querier
}

// NewInbox wires the inbox; db is used only for MarkFailed, which records a
// failure after the business transaction has rolled back.
func NewInbox(db database.Querier) *Inbox { return &Inbox{db: db} }

// Begin claims an event inside q's transaction. A first delivery inserts a
// PROCESSING row; a redelivery of a PROCESSED event returns
// ErrDuplicateEvent; a redelivery after a failure re-claims the row; the same
// ID with a different payload returns ErrPayloadMismatch. Concurrent
// deliveries of one event serialize on the row lock.
func (i *Inbox) Begin(ctx context.Context, q database.Querier, eventID, eventType, producer string, payload []byte) error {
	hash := HashPayload(payload)
	var status, storedHash string
	err := q.QueryRow(ctx, `SELECT status, payload_hash FROM referral_inbox WHERE event_id = $1 FOR UPDATE`, eventID).Scan(&status, &storedHash)
	switch {
	case err == nil:
		if storedHash != hash {
			return ErrPayloadMismatch
		}
		if status == StatusProcessed || status == StatusRejected {
			return ErrDuplicateEvent
		}
		if _, err := q.Exec(ctx, `UPDATE referral_inbox SET status = $1, error_code = NULL WHERE event_id = $2`, StatusProcessing, eventID); err != nil {
			return fmt.Errorf("inbox: reclaim: %w", err)
		}
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		// ON CONFLICT covers two first deliveries racing: the loser blocks on
		// the winner's uncommitted row and then sees it as a duplicate.
		tag, err := q.Exec(ctx, `
			INSERT INTO referral_inbox (event_id, event_type, producer, payload_hash, status)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (event_id) DO NOTHING`, eventID, eventType, producer, hash, StatusProcessing)
		if err != nil {
			return fmt.Errorf("inbox: insert: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return i.Begin(ctx, q, eventID, eventType, producer, payload)
		}
		return nil
	default:
		return fmt.Errorf("inbox: lookup: %w", err)
	}
}

// MarkProcessed completes the event inside the business transaction.
func (i *Inbox) MarkProcessed(ctx context.Context, q database.Querier, eventID string) error {
	return i.finish(ctx, q, eventID, StatusProcessed, "")
}

// MarkRejected acknowledges a malformed event inside the business
// transaction. Its code says why; it is never applied.
func (i *Inbox) MarkRejected(ctx context.Context, q database.Querier, eventID, code string) error {
	return i.finish(ctx, q, eventID, StatusRejected, code)
}

func (i *Inbox) finish(ctx context.Context, q database.Querier, eventID, status, code string) error {
	tag, err := q.Exec(ctx, `UPDATE referral_inbox SET status = $1, processed_at = now(), error_code = NULLIF($2, '') WHERE event_id = $3`, status, code, eventID)
	if err != nil {
		return fmt.Errorf("inbox: finish: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inbox: event %s not found", eventID)
	}
	return nil
}

// MarkFailed records a failure with a short code after the business
// transaction rolled back, so operators can see stuck events. The producer
// retries, and Begin re-claims the row.
func (i *Inbox) MarkFailed(ctx context.Context, eventID, eventType, producer string, payload []byte, code string) {
	if len(code) > 100 {
		code = code[:100]
	}
	_, _ = i.db.Exec(ctx, `
		INSERT INTO referral_inbox (event_id, event_type, producer, payload_hash, status, error_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO UPDATE SET status = EXCLUDED.status, error_code = EXCLUDED.error_code
		WHERE referral_inbox.status NOT IN ('PROCESSED', 'REJECTED')`,
		eventID, eventType, producer, HashPayload(payload), StatusFailed, code)
}

// deny renders the failure itself and stops the chain. These are security
// boundaries, so they must not depend on the application-wide error
// middleware being mounted.
func deny(c *gin.Context, err error) {
	httpx.RespondError(c, err)
	c.Abort()
}

// RequireCallers rejects signed requests from services outside allowed. It
// must run after serviceauth.Middleware: holding a secret for one route group
// is not permission to call every group.
func RequireCallers(allowed ...string) gin.HandlerFunc {
	set := map[string]bool{}
	for _, caller := range allowed {
		set[caller] = true
	}
	return func(c *gin.Context) {
		if !set[c.GetString("service_name")] {
			deny(c, apperrors.Forbidden("This service may not call this endpoint.", nil))
			return
		}
		c.Next()
	}
}

// RejectReplayedNonce makes every signed request single-use by recording its
// nonce until well past the signature's validity window.
func RejectReplayedNonce(db database.Querier) gin.HandlerFunc {
	return func(c *gin.Context) {
		nonce := c.GetHeader(serviceauth.HeaderNonce)
		caller := c.GetString("service_name")
		if nonce == "" || caller == "" {
			deny(c, apperrors.Unauthorized("Signed requests must carry a nonce.", nil))
			return
		}
		tag, err := db.Exec(c.Request.Context(), `
			INSERT INTO referral_service_nonces (caller, nonce, expires_at)
			VALUES ($1, $2, now() + interval '15 minutes')
			ON CONFLICT DO NOTHING`, caller, nonce)
		if err != nil {
			deny(c, apperrors.Unavailable("Request could not be verified.", err))
			return
		}
		if tag.RowsAffected() != 1 {
			deny(c, apperrors.Unauthorized("This request was already used.", nil))
			return
		}
		c.Next()
	}
}

// PruneNonces deletes expired nonces. Safe to run from any replica.
func PruneNonces(ctx context.Context, db database.Querier) error {
	_, err := db.Exec(ctx, `DELETE FROM referral_service_nonces WHERE expires_at < $1`, time.Now())
	return err
}
