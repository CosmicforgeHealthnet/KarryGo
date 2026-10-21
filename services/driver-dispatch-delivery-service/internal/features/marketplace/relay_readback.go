package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/providerreceipt"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrReceiptUnsupported = errors.New("authoritative receipt not supported for this operation")

type FleetReceiptReader interface {
	Readback(context.Context, CallMeta, RelayOperation) (*providerreceipt.Receipt, error)
}

// Readback has no mutation fallback, including on 404, outage or mismatch.
func (r *HTTPFleetResponder) Readback(ctx context.Context, meta CallMeta, op RelayOperation) (*providerreceipt.Receipt, error) {
	isOffer := op.OperationType == RelayProviderOfferAccept || op.OperationType == RelayProviderOfferDecline
	if isOffer && (op.OfferID == nil || op.AssignmentID != nil) || !isOffer && (op.AssignmentID == nil || op.OfferID != nil || op.OperationType != RelayProviderAssignmentAccept && op.OperationType != RelayProviderAssignmentDecline) {
		return nil, ErrReceiptUnsupported
	}
	var payload decisionPayload
	if json.Unmarshal(op.Payload, &payload) != nil || payload.ProviderID != op.ProviderID || payload.ClientResponseID != op.IdempotencyKey || payload.ProviderService != "DISPATCH" {
		return nil, ErrFleetUnavailable
	}
	decision := "ACCEPTED"
	intent := providerreceipt.Intent{ProviderID: op.ProviderID, ClientResponseID: op.IdempotencyKey, Decision: decision}
	version := providerreceipt.Version
	entityPath := "assignments/"
	entityID := ""
	if isOffer {
		intent.OfferID = *op.OfferID
		if op.OperationType == RelayProviderOfferAccept {
			intent.ReservationID = payload.DispatchReservationID
			intent.RequestVersion = payload.DispatchRequestVersion
		}
		entityID = *op.OfferID
		entityPath = "marketplace-offers/"
		version = providerreceipt.OfferVersion
	} else {
		intent.AssignmentID = *op.AssignmentID
		entityID = *op.AssignmentID
	}
	if isOffer || op.OperationType == RelayProviderAssignmentDecline {
		if op.OperationType == RelayProviderOfferDecline || op.OperationType == RelayProviderAssignmentDecline {
			intent.Decision = "DECLINED"
		}
		intent.ReasonCode = payload.ReasonCode
		intent.Note = payload.Note
		intent.Evidence = payload.EvidenceFileIDs
	}
	if !isOffer && privateAssignmentID(*op.AssignmentID) {
		// The native private decision contract trims its stored safe note and
		// does not record evidence. Unrecorded evidence cannot be confirmed.
		if len(payload.EvidenceFileIDs) > 0 {
			return nil, ErrReceiptUnsupported
		}
		intent.Note = strings.TrimSpace(intent.Note)
	}
	query := url.Values{"providerId": {op.ProviderID}, "clientResponseId": {op.IdempotencyKey}}
	path := "/internal/fleet-partner/v1/" + entityPath + url.PathEscape(entityID) + "/provider-decision-receipt?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+path, nil)
	if err != nil {
		return nil, ErrFleetUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderRequestID, meta.RequestID)
	req.Header.Set(HeaderCorrelationID, meta.CorrelationID)
	if err = serviceauth.SignRequest(req, r.serviceName, r.secret, nil, time.Now()); err != nil {
		return nil, ErrFleetUnavailable
	}
	res, err := r.client.Do(req)
	if err != nil {
		return nil, ErrFleetUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrFleetUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxFleetResponseBytes+1))
	if err != nil || len(raw) > maxFleetResponseBytes {
		return nil, ErrFleetUnavailable
	}
	var envelope struct {
		Data providerreceipt.Receipt `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, ErrFleetUnavailable
	}
	receipt := envelope.Data
	if receipt.ContractVersion != version || receipt.OfferID != intent.OfferID || receipt.ReceiptID == "" || len(receipt.ReceiptID) > 128 || receipt.AssignmentID != intent.AssignmentID || receipt.ProviderID != intent.ProviderID || receipt.ClientResponseID != intent.ClientResponseID || receipt.Decision != intent.Decision || receipt.IntentDigest != intent.Digest() || receipt.RecordedAt.IsZero() || receipt.RecordedAt.After(time.Now().Add(time.Minute)) {
		return nil, ErrFleetUnavailable
	}
	return &receipt, nil
}

// ClaimRelayReadbacks leases uncertain rows without making them deliverable.
func (r *Repository) ClaimRelayReadbacks(ctx context.Context, owner string, lease time.Duration, limit int) ([]RelayOperation, error) {
	rows, err := r.db.Query(ctx, `WITH claimed AS (
	 SELECT id FROM dispatch_fleet_relay_operations
	 WHERE readback_required AND status IN ('UNKNOWN_OUTCOME','DEAD_LETTER')
	 AND next_attempt_at<=now() AND (lease_expires_at IS NULL OR lease_expires_at<now())
	 ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT $3
	) UPDATE dispatch_fleet_relay_operations o SET lease_owner=$1,
	 lease_expires_at=now()+make_interval(secs=>$2),updated_at=now()
	 FROM claimed WHERE o.id=claimed.id RETURNING `+qualifiedRelayColumns(), owner, lease.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RelayOperation
	for rows.Next() {
		op, err := scanRelay(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *op)
	}
	return result, rows.Err()
}

func qualifiedRelayColumns() string {
	// Fixed list shared with the delivery claim; never a caller-supplied selector.
	return "o.id,o.operation_type,o.provider_id,o.request_id,o.offer_id,o.assignment_id,o.idempotency_key,o.request_fingerprint,o.payload,o.payload_hash,o.status,o.attempt_count,o.next_attempt_at,o.lease_owner,o.lease_expires_at,o.response_snapshot,o.last_error,o.created_at,o.updated_at,o.completed_at,o.readback_required"
}

func (r *Repository) finishRelayReadback(ctx context.Context, op RelayOperation, owner string, receipt *providerreceipt.Receipt, backoff time.Duration, code string) error {
	var raw []byte
	var err error
	if receipt != nil {
		raw, err = json.Marshal(receipt)
		if err != nil {
			return err
		}
	}
	return r.WithinTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET
	 status=CASE WHEN $3::jsonb IS NOT NULL THEN 'CONFIRMED' ELSE status END,
	 readback_required=($3::jsonb IS NULL),response_snapshot=COALESCE($3::jsonb,response_snapshot),
	 completed_at=CASE WHEN $3::jsonb IS NOT NULL THEN now() ELSE completed_at END,
	 attempt_count=attempt_count+1,next_attempt_at=now()+make_interval(secs=>$4),
	 last_error=CASE WHEN $3::jsonb IS NOT NULL THEN NULL ELSE $5 END,
	 lease_owner=NULL,lease_expires_at=NULL,updated_at=now()
	 WHERE id=$1 AND lease_owner=$2 AND lease_expires_at>now() AND readback_required
	 AND status IN ('UNKNOWN_OUTCOME','DEAD_LETTER')`, op.ID, owner, raw, backoff.Seconds(), code)
		if err == nil && tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		if err != nil {
			return err
		}
		outcome := "UNRESOLVED"
		var receiptRef, digest *string
		if receipt != nil {
			outcome = "CONFIRMED"
			receiptRef = &receipt.ReceiptID
			digest = &receipt.IntentDigest
		} else if code == "receipt_not_supported" {
			outcome = "UNSUPPORTED"
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_fleet_relay_readback_audit(operation_id,outcome,receipt_ref,intent_digest) VALUES($1,$2,$3,$4)`, op.ID, outcome, receiptRef, digest)
		return err
	})
}

func (w *RelayWorker) reconcile(ctx context.Context, result *RelayResult) error {
	reader, ok := w.responder.(FleetReceiptReader)
	if !ok || !w.responder.Configured() {
		return nil
	}
	// Each claim has an unguessable lease token, even when replicas use the
	// same configured worker label. A timed-out reader cannot finish a new lease.
	owner := w.config.Owner + "/" + uuid.NewString()
	operations, err := w.repo.ClaimRelayReadbacks(ctx, owner, w.config.Lease, w.config.BatchSize)
	if err != nil {
		return err
	}
	for _, op := range operations {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		receipt, readErr := reader.Readback(ctx, CallMeta{RequestID: op.ID, CorrelationID: op.IdempotencyKey}, op)
		code := "receipt_unresolved"
		if errors.Is(readErr, ErrReceiptUnsupported) {
			code = "receipt_not_supported"
		}
		if readErr != nil {
			receipt = nil
		}
		if err = w.repo.finishRelayReadback(ctx, op, owner, receipt, w.config.backoffFor(op.AttemptCount), code); err != nil {
			return err
		}
		if receipt != nil {
			result.Reconciled++
		}
	}
	return nil
}
