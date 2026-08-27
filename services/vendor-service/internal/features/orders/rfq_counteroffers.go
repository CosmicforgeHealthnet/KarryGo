package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type CounterOfferInput struct {
	PriceMinor     int64
	Currency       string
	Snapshot       map[string]any
	Capacity       CapacityRequirement
	ExpiresAt      time.Time
	IdempotencyKey string
}

func (s *Section21Service) SubmitRFQCounterOffer(ctx context.Context, requesterID, rfqID, quoteID string, in CounterOfferInput) (RFQQuoteRevision, error) {
	if err := s.available(); err != nil {
		return RFQQuoteRevision{}, err
	}
	if strings.TrimSpace(requesterID) == "" || strings.TrimSpace(quoteID) == "" || in.PriceMinor < 0 || strings.TrimSpace(in.Currency) == "" || !in.ExpiresAt.After(s.now().UTC()) {
		return RFQQuoteRevision{}, ErrSection21Conflict
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RFQQuoteRevision{}, err
	}
	defer tx.Rollback(ctx)
	var vendorID, branchID, rfqStatus string
	var current int64
	if err = tx.QueryRow(ctx, `SELECT q.vendor_id,q.branch_id,q.current_revision,r.status FROM commerce_rfq_quotes q JOIN commerce_rfqs r ON r.id=q.rfq_id WHERE q.id=$1 AND q.rfq_id=$2 AND r.requester_id=$3 FOR UPDATE`, quoteID, rfqID, requesterID).Scan(&vendorID, &branchID, &current, &rfqStatus); err != nil {
		return RFQQuoteRevision{}, ErrSection21NotFound
	}
	if rfqStatus != "OPEN" && rfqStatus != "QUOTED" {
		return RFQQuoteRevision{}, ErrSection21InvalidState
	}
	if in.IdempotencyKey != "" {
		var prior RFQQuoteRevision
		var raw, capraw, vendorRaw, paymentRaw, depositRaw, cancellationRaw []byte
		err = tx.QueryRow(ctx, `SELECT r.id,r.quote_id,r.rfq_id,q.vendor_id,q.branch_id,r.revision,r.price_minor,r.currency,r.snapshot,r.capacity_requirement,r.vendor_terms,r.payment_policy,r.deposit_policy,r.cancellation_policy,r.expires_at,q.status FROM commerce_rfq_quote_revisions r JOIN commerce_rfq_quotes q ON q.id=r.quote_id WHERE r.quote_id=$1 AND r.idempotency_key=$2 FOR UPDATE`, quoteID, in.IdempotencyKey).Scan(&prior.ID, &prior.QuoteID, &prior.RFQID, &prior.VendorID, &prior.BranchID, &prior.Revision, &prior.PriceMinor, &prior.Currency, &raw, &capraw, &vendorRaw, &paymentRaw, &depositRaw, &cancellationRaw, &prior.ExpiresAt, &prior.Status)
		if err == nil {
			_ = json.Unmarshal(raw, &prior.Snapshot)
			_ = json.Unmarshal(capraw, &prior.Capacity)
			_ = json.Unmarshal(vendorRaw, &prior.VendorTerms)
			_ = json.Unmarshal(paymentRaw, &prior.PaymentPolicy)
			_ = json.Unmarshal(depositRaw, &prior.DepositPolicy)
			_ = json.Unmarshal(cancellationRaw, &prior.CancellationPolicy)
			if err = tx.Commit(ctx); err != nil {
				return RFQQuoteRevision{}, err
			}
			return prior, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return RFQQuoteRevision{}, err
		}
	}
	raw, _ := json.Marshal(in.Snapshot)
	capraw, _ := json.Marshal(in.Capacity)
	vendorRaw := snapshotMap(in.Snapshot, "vendor_terms")
	paymentRaw := snapshotMapWithDefault(in.Snapshot, "payment_policy", map[string]any{"type": "FULL_PAYMENT"})
	depositRaw := snapshotMap(in.Snapshot, "deposit_policy")
	cancellationRaw := snapshotMap(in.Snapshot, "cancellation_policy")
	revision := current + 1
	id := sid("rfq_r_")
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_rfq_quote_revisions(id,quote_id,rfq_id,revision,price_minor,currency,snapshot,capacity_requirement,vendor_terms,payment_policy,deposit_policy,cancellation_policy,created_by,expires_at,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,''))`, id, quoteID, rfqID, revision, in.PriceMinor, in.Currency, raw, capraw, vendorRaw, paymentRaw, depositRaw, cancellationRaw, requesterID, in.ExpiresAt, in.IdempotencyKey); err != nil {
		return RFQQuoteRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_rfq_quotes SET current_revision=$2,updated_at=now() WHERE id=$1`, quoteID, revision); err != nil {
		return RFQQuoteRevision{}, err
	}
	if err = s.appendEvent(ctx, tx, "commerce.rfq.counter_offer.submitted", "commerce_rfq", rfqID, vendorID, revision, map[string]any{"quote_id": quoteID, "revision": revision, "created_by": requesterID}); err != nil {
		return RFQQuoteRevision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RFQQuoteRevision{}, err
	}
	return RFQQuoteRevision{ID: id, QuoteID: quoteID, RFQID: rfqID, VendorID: vendorID, BranchID: branchID, Revision: revision, PriceMinor: in.PriceMinor, Currency: in.Currency, Snapshot: in.Snapshot, Capacity: in.Capacity, ExpiresAt: in.ExpiresAt, Status: "ACTIVE"}, nil
}
