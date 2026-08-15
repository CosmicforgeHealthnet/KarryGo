package crm

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
	"time"
)

type CorporateCommercialProjection struct {
	State                string     `json:"state"`
	OrderCount           *int       `json:"order_count,omitempty"`
	CompletedOrderCount  *int       `json:"completed_order_count,omitempty"`
	RecurringOrderCount  *int       `json:"recurring_order_count,omitempty"`
	BulkOrderCount       *int       `json:"bulk_order_count,omitempty"`
	NetSpendMinor        *int64     `json:"net_spend_minor,omitempty"`
	Currency             *string    `json:"currency,omitempty"`
	FirstOrderAt         *time.Time `json:"first_order_at,omitempty"`
	LastOrderAt          *time.Time `json:"last_order_at,omitempty"`
	LastBulkOrderAt      *time.Time `json:"last_bulk_order_at,omitempty"`
	LastRecurringOrderAt *time.Time `json:"last_recurring_order_at,omitempty"`
	LastBranchRef        *string    `json:"last_branch_ref,omitempty"`
}
type CorporateRFQFactInput struct {
	CorporateCustomerID, SourceService, SourceRFQRef, SourceEventID string
	SourceQuoteRef                                                  *string
	SourceVersion                                                   int64
	RFQState, QuoteOutcome                                          string
	ConvertedOrderRef, BranchID                                     *string
	ResolvedAt                                                      *time.Time
	OccurredAt                                                      time.Time
}
type CorporateRFQProjection struct {
	State              string  `json:"state"`
	RFQCount           *int    `json:"rfq_count,omitempty"`
	QuotedRFQCount     *int    `json:"quoted_rfq_count,omitempty"`
	AcceptedQuoteCount *int    `json:"accepted_quote_count,omitempty"`
	RejectedQuoteCount *int    `json:"rejected_quote_count,omitempty"`
	ConvertedRFQCount  *int    `json:"converted_rfq_count,omitempty"`
	QuoteConversionBPS *int    `json:"quote_conversion_bps,omitempty"`
	ResolvedRFQCount   *int    `json:"resolved_rfq_count,omitempty"`
	LatestRFQRef       *string `json:"latest_rfq_ref,omitempty"`
	LatestOutcome      *string `json:"latest_rfq_outcome,omitempty"`
}

func (s *Service) RebuildCorporateCommercial(ctx context.Context, tx pgx.Tx, actor Actor, corporateID string, asOf time.Time) error {
	var total, completed, recurring, bulk int
	var spend int64
	var currency string
	var first, last, lastBulk, lastRecurring *time.Time
	var lastBranch *string
	rows, err := tx.Query(ctx, `SELECT order_state,order_type,net_amount_minor,currency,completed_at,branch_id FROM crm_customer_order_facts WHERE business_id=$1 AND corporate_customer_id=$2 ORDER BY completed_at`, actor.VendorID, corporateID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var state, typ, cur string
		var net int64
		var at *time.Time
		var branch *string
		if err := rows.Scan(&state, &typ, &net, &cur, &at, &branch); err != nil {
			rows.Close()
			return err
		}
		total++
		if state != "COMPLETED" {
			continue
		}
		completed++
		spend += net
		if currency == "" {
			currency = cur
		}
		if at != nil {
			if first == nil {
				v := *at
				first = &v
			}
			v := *at
			last = &v
			lastBranch = branch
			if typ == "BULK" {
				bulk++
				lastBulk = &v
			}
			if typ == "RECURRING" {
				recurring++
				lastRecurring = &v
			}
		}
	}
	rows.Close()
	state := "CURRENT"
	var tp, cp, rp, bp *int
	var ns *int64
	var curp *string
	if total == 0 {
		state = "NOT_AVAILABLE"
	} else {
		tp = &total
		cp = &completed
		rp = &recurring
		bp = &bulk
		ns = &spend
		curp = &currency
	}
	_, err = tx.Exec(ctx, `INSERT INTO crm_corporate_commercial_projections (business_id,corporate_customer_id,state,order_count,completed_order_count,recurring_order_count,bulk_order_count,net_spend_minor,currency,first_order_at,last_order_at,last_bulk_order_at,last_recurring_order_at,last_branch_ref,source_as_of,computed_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now(),1) ON CONFLICT (business_id,corporate_customer_id) DO UPDATE SET state=EXCLUDED.state,order_count=EXCLUDED.order_count,completed_order_count=EXCLUDED.completed_order_count,recurring_order_count=EXCLUDED.recurring_order_count,bulk_order_count=EXCLUDED.bulk_order_count,net_spend_minor=EXCLUDED.net_spend_minor,currency=EXCLUDED.currency,first_order_at=EXCLUDED.first_order_at,last_order_at=EXCLUDED.last_order_at,last_bulk_order_at=EXCLUDED.last_bulk_order_at,last_recurring_order_at=EXCLUDED.last_recurring_order_at,last_branch_ref=EXCLUDED.last_branch_ref,source_as_of=EXCLUDED.source_as_of,computed_at=now(),version=crm_corporate_commercial_projections.version+1`, actor.VendorID, corporateID, state, tp, cp, rp, bp, ns, curp, first, last, lastBulk, lastRecurring, lastBranch, asOf)
	return err
}

func (s *Service) ApplyCorporateRFQFact(ctx context.Context, tx pgx.Tx, actor Actor, in CorporateRFQFactInput) error {
	if in.CorporateCustomerID == "" || in.SourceRFQRef == "" || in.SourceVersion < 1 {
		return apperrors.BadRequest("RFQ identity is required.", nil)
	}
	if in.BranchID != nil {
		ok, err := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *in.BranchID)
		if err != nil {
			return err
		}
		if !ok {
			return errCrossTenant()
		}
	}
	var id string
	var version int64
	err := tx.QueryRow(ctx, `SELECT id,source_version FROM crm_corporate_rfq_facts WHERE business_id=$1 AND source_service=$2 AND source_rfq_ref=$3 FOR UPDATE`, actor.VendorID, in.SourceService, in.SourceRFQRef).Scan(&id, &version)
	if err == pgx.ErrNoRows {
		id = newID("crmr_")
		_, err = tx.Exec(ctx, `INSERT INTO crm_corporate_rfq_facts (id,business_id,corporate_customer_id,source_service,source_rfq_ref,source_quote_ref,source_event_id,source_version,rfq_state,quote_outcome,converted_order_ref,branch_id,resolved_at,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, actor.VendorID, in.CorporateCustomerID, in.SourceService, in.SourceRFQRef, in.SourceQuoteRef, in.SourceEventID, in.SourceVersion, in.RFQState, optional(in.QuoteOutcome), in.ConvertedOrderRef, in.BranchID, in.ResolvedAt, in.OccurredAt)
	} else if err == nil && in.SourceVersion > version {
		_, err = tx.Exec(ctx, `UPDATE crm_corporate_rfq_facts SET source_version=$1,source_event_id=$2,rfq_state=$3,quote_outcome=$4,converted_order_ref=$5,branch_id=$6,resolved_at=$7,occurred_at=$8,updated_at=now() WHERE id=$9`, in.SourceVersion, in.SourceEventID, in.RFQState, optional(in.QuoteOutcome), in.ConvertedOrderRef, in.BranchID, in.ResolvedAt, in.OccurredAt, id)
	}
	if err != nil {
		return err
	}
	return s.rebuildCorporateRFQ(ctx, tx, actor.VendorID, in.CorporateCustomerID, in.OccurredAt)
}

func (s *Service) rebuildCorporateRFQ(ctx context.Context, tx pgx.Tx, businessID, corporateID string, asOf time.Time) error {
	var total, quoted, accepted, rejected, converted, resolved int
	var latest, latestOutcome *string
	rows, err := tx.Query(ctx, `SELECT source_rfq_ref,rfq_state,quote_outcome,occurred_at FROM crm_corporate_rfq_facts WHERE business_id=$1 AND corporate_customer_id=$2 ORDER BY occurred_at`, businessID, corporateID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ref, state string
		var outcome *string
		var at time.Time
		if err := rows.Scan(&ref, &state, &outcome, &at); err != nil {
			rows.Close()
			return err
		}
		total++
		if state == "QUOTED" || outcome != nil {
			quoted++
		}
		if outcome != nil {
			resolved++
			if *outcome == "ACCEPTED" {
				accepted++
			}
			if *outcome == "REJECTED" {
				rejected++
			}
			v := ref
			latest = &v
			o := *outcome
			latestOutcome = &o
		}
		if state == "CONVERTED" {
			converted++
		}
	}
	rows.Close()
	state := "CURRENT"
	var tp, qp, ap, rp, cp, rr *int
	var bps *int
	if total == 0 {
		state = "NOT_AVAILABLE"
	} else {
		tp = &total
		qp = &quoted
		ap = &accepted
		rp = &rejected
		cp = &converted
		rr = &resolved
		if quoted > 0 {
			v := accepted * 10000 / quoted
			bps = &v
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO crm_corporate_rfq_projections (business_id,corporate_customer_id,state,rfq_count,quoted_rfq_count,accepted_quote_count,rejected_quote_count,converted_rfq_count,quote_conversion_bps,resolved_rfq_count,latest_rfq_ref,latest_rfq_at,source_as_of,computed_at,version,latest_rfq_outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULL,$12,now(),1,$13) ON CONFLICT (business_id,corporate_customer_id) DO UPDATE SET state=EXCLUDED.state,rfq_count=EXCLUDED.rfq_count,quoted_rfq_count=EXCLUDED.quoted_rfq_count,accepted_quote_count=EXCLUDED.accepted_quote_count,rejected_quote_count=EXCLUDED.rejected_quote_count,converted_rfq_count=EXCLUDED.converted_rfq_count,quote_conversion_bps=EXCLUDED.quote_conversion_bps,resolved_rfq_count=EXCLUDED.resolved_rfq_count,latest_rfq_ref=EXCLUDED.latest_rfq_ref,latest_rfq_outcome=EXCLUDED.latest_rfq_outcome,source_as_of=EXCLUDED.source_as_of,computed_at=now(),version=crm_corporate_rfq_projections.version+1`, businessID, corporateID, state, tp, qp, ap, rp, cp, bps, rr, latest, asOf, latestOutcome)
	return err
}
