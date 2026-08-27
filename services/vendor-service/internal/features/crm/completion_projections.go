package crm

// This file is the final Section 8 projection seam. The producers remain
// authoritative elsewhere: CRM accepts only bounded facts and rebuilds every
// derived row from those facts, so replay, correction, refund and merge repair
// converge on the same answer.

import (
	"context"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

type AffinityView struct {
	Reference           string     `json:"reference"`
	BranchRef           *string    `json:"branch_ref,omitempty"`
	Scope               string     `json:"scope"`
	CompletedOrderCount int        `json:"completed_order_count"`
	QuantityCount       int        `json:"quantity_count"`
	NetSpendMinor       int64      `json:"net_spend_minor"`
	LastOrderedAt       *time.Time `json:"last_ordered_at,omitempty"`
	Rank                int        `json:"rank"`
	SourceAsOf          time.Time  `json:"source_as_of"`
}

type ServiceRecoveryView struct {
	State                    string     `json:"state"`
	ComplaintCount           *int       `json:"complaint_count,omitempty"`
	UnresolvedComplaintCount *int       `json:"unresolved_complaint_count,omitempty"`
	ResolvedComplaintCount   *int       `json:"resolved_complaint_count,omitempty"`
	RefundCount              *int       `json:"refund_count,omitempty"`
	PartialRefundCount       *int       `json:"partial_refund_count,omitempty"`
	LatestCaseRef            *string    `json:"latest_case_ref,omitempty"`
	LatestCaseOutcome        *string    `json:"latest_case_outcome,omitempty"`
	LatestCaseAt             *time.Time `json:"latest_case_at,omitempty"`
	LatestRefundRef          *string    `json:"latest_refund_ref,omitempty"`
	LatestRefundAt           *time.Time `json:"latest_refund_at,omitempty"`
	LastServiceRecoveryAt    *time.Time `json:"last_service_recovery_at,omitempty"`
	SourceAsOf               *time.Time `json:"source_as_of,omitempty"`
	Version                  int64      `json:"version"`
}

type SupportFactInput struct {
	SourceService   string
	SourceCaseRef   string
	SourceEventID   string
	SourceVersion   int64
	CaseState       string
	Outcome         string
	RelatedOrderRef *string
	BranchID        *string
	ResolvedAt      *time.Time
	SourceRefundRef *string
	OccurredAt      time.Time
}

type affinityAggregate struct {
	reference string
	orders    map[string]bool
	quantity  int
	spend     int64
	last      *time.Time
	branch    *string
}

// RebuildCompletionProjections rebuilds all final-completion projections that
// are currently derivable from CRM-owned facts.
func (s *Service) RebuildCompletionProjections(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, asOf time.Time) error {
	if err := s.rebuildAffinities(ctx, tx, actor.VendorID, profileID, asOf); err != nil {
		return err
	}
	if err := s.rebuildCustomerValue(ctx, tx, actor.VendorID, profileID, asOf); err != nil {
		return err
	}
	return s.rebuildRelationshipSignals(ctx, tx, actor.VendorID, profileID, asOf)
}

func (s *Service) rebuildAffinities(ctx context.Context, tx pgx.Tx, businessID, profileID string, asOf time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_category_affinities WHERE business_id=$1 AND profile_id=$2`, businessID, profileID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_item_affinities WHERE business_id=$1 AND profile_id=$2`, businessID, profileID); err != nil {
		return err
	}
	// The join intentionally filters to COMPLETED facts. A refund/correction
	// removes or replaces the fact and the next rebuild removes its influence.
	const source = `
		SELECT l.category_ref, l.item_ref, l.branch_id, f.source_order_ref,
		       f.completed_at, l.quantity, l.net_line_amount_minor
		FROM crm_customer_order_fact_lines l
		JOIN crm_customer_order_facts f ON f.id=l.order_fact_id AND f.business_id=l.business_id
		WHERE l.business_id=$1 AND l.profile_id=$2 AND f.order_state='COMPLETED'`
	rows, err := tx.Query(ctx, source, businessID, profileID)
	if err != nil {
		return err
	}
	categories := map[string]*affinityAggregate{}
	items := map[string]*affinityAggregate{}
	for rows.Next() {
		var category, item, order string
		var branch *string
		var completed *time.Time
		var qty int
		var spend int64
		if err := rows.Scan(&category, &item, &branch, &order, &completed, &qty, &spend); err != nil {
			rows.Close()
			return err
		}
		add := func(dst map[string]*affinityAggregate, key string, lineBranch *string) {
			if strings.TrimSpace(key) == "" {
				return
			}
			mapKey := key
			if lineBranch != nil {
				mapKey += "\x00" + *lineBranch
			}
			a := dst[mapKey]
			if a == nil {
				a = &affinityAggregate{reference: key, orders: map[string]bool{}, branch: lineBranch}
				dst[mapKey] = a
			}
			a.orders[order] = true
			a.quantity += qty
			a.spend += spend
			if completed != nil && (a.last == nil || completed.After(*a.last)) {
				v := *completed
				a.last = &v
			}
		}
		add(categories, category, nil)
		add(items, item, nil)
		if branch != nil {
			add(categories, category, branch)
			add(items, item, branch)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if err := insertAffinityRows(ctx, tx, businessID, profileID, asOf, categories, true); err != nil {
		return err
	}
	return insertAffinityRows(ctx, tx, businessID, profileID, asOf, items, false)
}

func insertAffinityRows(ctx context.Context, tx pgx.Tx, businessID, profileID string, asOf time.Time, values map[string]*affinityAggregate, category bool) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	// Stable ordering makes rank deterministic even when two affinities tie.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			a, b := values[keys[i]], values[keys[j]]
			if len(a.orders) < len(b.orders) || (len(a.orders) == len(b.orders) && (a.quantity < b.quantity || (a.quantity == b.quantity && keys[i] > keys[j]))) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for rank, key := range keys {
		a := values[key]
		orders := len(a.orders)
		scope := "BUSINESS"
		branch := a.branch
		if branch != nil {
			scope = "BRANCH"
		}
		refCol, table := "item_ref", "crm_customer_item_affinities"
		if category {
			refCol = "category_ref"
			table = "crm_customer_category_affinities"
		}
		_, err := tx.Exec(ctx, `INSERT INTO `+table+` (id,business_id,profile_id,scope,branch_id,`+refCol+`,completed_order_count,quantity_count,net_spend_minor,last_ordered_at,rank,source_as_of) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, newID("crmaff_"), businessID, profileID, scope, branch, a.reference, orders, a.quantity, a.spend, a.last, rank+1, asOf)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ApplySupportFact(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, in SupportFactInput) error {
	if strings.TrimSpace(in.SourceService) == "" || strings.TrimSpace(in.SourceCaseRef) == "" || in.SourceVersion < 1 {
		return apperrors.BadRequest("Support fact identity is required.", nil)
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
	applied := false
	err := tx.QueryRow(ctx, `SELECT id,source_version FROM crm_customer_support_facts WHERE business_id=$1 AND source_service=$2 AND source_case_ref=$3 FOR UPDATE`, actor.VendorID, in.SourceService, in.SourceCaseRef).Scan(&id, &version)
	if err == pgx.ErrNoRows {
		id = newID("crmsf_")
		_, err = tx.Exec(ctx, `INSERT INTO crm_customer_support_facts (id,business_id,profile_id,source_service,source_case_ref,source_event_id,source_version,case_state,outcome,related_order_ref,branch_id,resolved_at,source_refund_ref,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, actor.VendorID, profileID, in.SourceService, in.SourceCaseRef, in.SourceEventID, in.SourceVersion, in.CaseState, optional(in.Outcome), in.RelatedOrderRef, in.BranchID, in.ResolvedAt, in.SourceRefundRef, in.OccurredAt)
		applied = err == nil
	} else if err == nil && in.SourceVersion > version {
		_, err = tx.Exec(ctx, `UPDATE crm_customer_support_facts SET profile_id=$1,source_event_id=$2,source_version=$3,case_state=$4,outcome=$5,related_order_ref=$6,branch_id=$7,resolved_at=$8,source_refund_ref=$9,occurred_at=$10,last_observed_at=now(),updated_at=now() WHERE id=$11`, profileID, in.SourceEventID, in.SourceVersion, in.CaseState, optional(in.Outcome), in.RelatedOrderRef, in.BranchID, in.ResolvedAt, in.SourceRefundRef, in.OccurredAt, id)
		applied = err == nil
	}
	if err != nil {
		return err
	}
	if applied && (in.CaseState == "OPEN" || in.CaseState == "IN_REVIEW") {
		if err = s.publishTx(ctx, tx, actor, "commerce.vendor.support_action", "crm_support_fact", id, in.SourceVersion, map[string]any{
			"business_ref":     actor.VendorID,
			"parent_order_ref": optionalValue(in.RelatedOrderRef),
			"status":           in.CaseState,
			"source_version":   in.SourceVersion,
		}); err != nil {
			return err
		}
	}
	return s.rebuildServiceRecovery(ctx, tx, actor.VendorID, profileID, in.OccurredAt)
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) rebuildServiceRecovery(ctx context.Context, tx pgx.Tx, businessID, profileID string, asOf time.Time) error {
	var count, unresolved, resolved, refunds, partial int
	var latestRef, latestOutcome, refundRef *string
	var latestAt, refundAt *time.Time
	rows, err := tx.Query(ctx, `SELECT source_case_ref,outcome,case_state,occurred_at,source_refund_ref FROM crm_customer_support_facts WHERE business_id=$1 AND profile_id=$2 ORDER BY occurred_at`, businessID, profileID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var ref string
		var outcome *string
		var state string
		var at time.Time
		if err := rows.Scan(&ref, &outcome, &state, &at, &refundRef); err != nil {
			rows.Close()
			return err
		}
		count++
		if state == "OPEN" || state == "IN_REVIEW" {
			unresolved++
		}
		if state == "RESOLVED" || state == "REJECTED" || state == "WITHDRAWN" {
			resolved++
		}
		if outcome != nil && (*outcome == "REFUND_FULL" || *outcome == "REFUND_PARTIAL") {
			refunds++
			if *outcome == "REFUND_PARTIAL" {
				partial++
			}
			v := ref
			latestRef = &v
			v2 := *outcome
			latestOutcome = &v2
			t := at
			latestAt = &t
			if refundRef != nil {
				r := *refundRef
				refundRef = &r
				rt := at
				refundAt = &rt
			}
		}
	}
	rows.Close()
	state := "CURRENT"
	var cp, up, rp, pp *int
	cp = &count
	up = &unresolved
	rp = &refunds
	pp = &partial
	if count == 0 {
		state = "NOT_AVAILABLE"
		cp = nil
		up = nil
		rp = nil
		pp = nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO crm_customer_service_recovery_projections (business_id,profile_id,state,complaint_count,unresolved_complaint_count,resolved_complaint_count,refund_count,partial_refund_count,latest_case_ref,latest_case_outcome,latest_case_at,latest_refund_ref,latest_refund_at,last_service_recovery_at,source_as_of,computed_at,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now(),1) ON CONFLICT (business_id,profile_id) DO UPDATE SET state=EXCLUDED.state,complaint_count=EXCLUDED.complaint_count,unresolved_complaint_count=EXCLUDED.unresolved_complaint_count,resolved_complaint_count=EXCLUDED.resolved_complaint_count,refund_count=EXCLUDED.refund_count,partial_refund_count=EXCLUDED.partial_refund_count,latest_case_ref=EXCLUDED.latest_case_ref,latest_case_outcome=EXCLUDED.latest_case_outcome,latest_case_at=EXCLUDED.latest_case_at,latest_refund_ref=EXCLUDED.latest_refund_ref,latest_refund_at=EXCLUDED.latest_refund_at,last_service_recovery_at=EXCLUDED.last_service_recovery_at,source_as_of=EXCLUDED.source_as_of,computed_at=now(),version=crm_customer_service_recovery_projections.version+1`, businessID, profileID, state, cp, up, rp, pp, latestRef, latestOutcome, latestAt, refundRef, refundAt, latestAt, asOf)
	return err
}
