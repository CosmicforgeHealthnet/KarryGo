package crm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

// OrderFactInput is one authoritative order observation.
type OrderFactInput struct {
	SourceService       string
	SourceOrderRef      string
	SourceEventID       string
	SourceVersion       int64
	OrderState          OrderFactState
	OrderType           string
	CorporateCustomerID *string
	CompletedAt         *time.Time
	Currency            string
	GrossMinor          int64
	NetMinor            int64
	RefundedMinor       int64
	BranchID            *string
	CapabilityType      *string
	ObservedAt          time.Time
	Lines               []OrderFactLineInput
}

// OrderFactLineInput is the bounded, reference-only line evidence used to
// rebuild category and item affinity. CRM never stores catalogue names or
// prices; the order authority supplies the historical net line amount.
type OrderFactLineInput struct {
	CategoryRef        string  `json:"category_ref"`
	ItemRef            string  `json:"item_ref"`
	Quantity           int     `json:"quantity"`
	NetLineAmountMinor int64   `json:"net_line_amount_minor"`
	BranchID           *string `json:"branch_ref"`
}

// ApplyOrderFact upserts one order fact and rebuilds the affected aggregates.
//
// The fact table is keyed by the authoritative order reference, so the same
// order observed any number of times — created, completed, partially refunded,
// redelivered — is always exactly one row. Aggregates are then recomputed from
// the fact set rather than incremented, which is what makes refunds and
// replays safe by construction instead of by careful arithmetic.
func (s *Service) ApplyOrderFact(ctx context.Context, tx pgx.Tx, actor Actor, profile Profile, in OrderFactInput) error {
	// Cross-currency data is quarantined rather than converted or summed:
	// CRM has no FX authority and must never silently corrupt a total.
	businessCurrency, err := s.businessCurrency(ctx, tx, actor.VendorID)
	if err != nil {
		return err
	}
	if in.Currency != businessCurrency {
		s.metrics.Inc("crm_order_fact_currency_conflicts_total")
		return errCurrencyConflict()
	}
	if in.BranchID != nil {
		owned, branchErr := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *in.BranchID)
		if branchErr != nil {
			return branchErr
		}
		if !owned {
			return errCrossTenant()
		}
	}

	var (
		existingID      string
		existingVersion int64
		existingState   OrderFactState
		existingGross   int64
	)
	err = tx.QueryRow(ctx, `SELECT id,source_version,order_state,gross_amount_minor FROM crm_customer_order_facts
		WHERE business_id=$1 AND source_service=$2 AND source_order_ref=$3 FOR UPDATE`,
		actor.VendorID, in.SourceService, in.SourceOrderRef).Scan(&existingID, &existingVersion, &existingState, &existingGross)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		existingID = newID("crmof_")
		if _, err = tx.Exec(ctx, `INSERT INTO crm_customer_order_facts
			(id,business_id,profile_id,source_service,source_order_ref,source_event_id,source_version,
			 order_state,order_type,corporate_customer_id,completed_at,currency,gross_amount_minor,net_amount_minor,refunded_amount_minor,
			 branch_id,capability_type,last_observed_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			existingID, actor.VendorID, profile.ID, in.SourceService, in.SourceOrderRef, in.SourceEventID,
			in.SourceVersion, in.OrderState, normalizedOrderType(in.OrderType), in.CorporateCustomerID, in.CompletedAt, in.Currency, in.GrossMinor, in.NetMinor,
			in.RefundedMinor, in.BranchID, in.CapabilityType, in.ObservedAt); err != nil {
			return fmt.Errorf("insert order fact: %w", err)
		}
	case err != nil:
		return err
	case in.SourceVersion < existingVersion:
		s.metrics.Inc("crm_order_fact_stale_total")
		return errSourceVersionStale()
	case in.SourceVersion == existingVersion && (existingState != in.OrderState || existingGross != in.GrossMinor):
		s.metrics.Inc("crm_order_fact_conflicts_total")
		return errSourceVersionConflict()
	case in.SourceVersion == existingVersion:
		return nil
	default:
		if _, err = tx.Exec(ctx, `UPDATE crm_customer_order_facts
			SET profile_id=$1,source_event_id=$2,source_version=$3,order_state=$4,order_type=$5,corporate_customer_id=$6,completed_at=$7,currency=$8,
				gross_amount_minor=$9,net_amount_minor=$10,refunded_amount_minor=$11,branch_id=$12,
				capability_type=$13,last_observed_at=$14,updated_at=now()
			WHERE id=$15`,
			profile.ID, in.SourceEventID, in.SourceVersion, in.OrderState, normalizedOrderType(in.OrderType), in.CorporateCustomerID, in.CompletedAt, in.Currency,
			in.GrossMinor, in.NetMinor, in.RefundedMinor, in.BranchID, in.CapabilityType, in.ObservedAt, existingID); err != nil {
			return fmt.Errorf("update order fact: %w", err)
		}
	}
	if err := s.replaceOrderFactLines(ctx, tx, actor, profile.ID, existingID, in); err != nil {
		return fmt.Errorf("replace order lines: %w", err)
	}

	s.metrics.Inc("crm_profile_projection_events_total", "state", string(in.OrderState))
	if err := s.RebuildCommercialProjection(ctx, tx, actor, profile.ID, in.SourceVersion, in.ObservedAt); err != nil {
		return fmt.Errorf("rebuild commercial: %w", err)
	}
	if in.CorporateCustomerID != nil {
		if err := s.RebuildCorporateCommercial(ctx, tx, actor, *in.CorporateCustomerID, in.ObservedAt); err != nil {
			return fmt.Errorf("rebuild corporate commercial: %w", err)
		}
	}
	return s.RebuildCompletionProjections(ctx, tx, actor, profile.ID, in.ObservedAt)
}

// RebuildCommercialProjection recomputes a profile's commercial aggregates and
// branch relations from its order facts.
//
// This is the single writer of those aggregates. Ingestion calls it, merges
// call it, and an operator repair path can call it — all with identical
// results, because the fact set is the only input.
func (s *Service) RebuildCommercialProjection(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, sourceVersion int64, observedAt time.Time) error {
	facts, err := s.orderFacts(ctx, tx, actor.VendorID, profileID)
	if err != nil {
		return fmt.Errorf("load order facts: %w", err)
	}
	totals := ComputeCommercialTotals(facts)

	if len(facts) == 0 {
		// No facts left (for example, everything moved away by a merge): the
		// honest state is "nothing observed", not "spent zero".
		_, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles SET
		observed_order_count=0, completed_order_count=0, gross_spend_minor=0, net_spend_minor=0, average_order_value_minor=0,
			first_completed_order_at=NULL, last_completed_order_at=NULL, last_order_ref=NULL,
			last_branch_ref=NULL, preferred_branch_ref=NULL, last_capability_type=NULL,
			commercial_currency=NULL, commercial_projection_state=$1, commercial_projection_version=NULL,
			last_commercial_source_observed_at=NULL, updated_at=now(), version=version+1
			WHERE business_id=$2 AND id=$3`,
			ProjectionNotAvailable, actor.VendorID, profileID)
		if err != nil {
			return err
		}
		return s.invalidateCommercialSegmentsTx(ctx, tx, actor.VendorID)
	}

	if err = s.rebuildBranchRelations(ctx, tx, actor, profileID, facts, sourceVersion); err != nil {
		return fmt.Errorf("rebuild branch relations: %w", err)
	}
	relations, err := s.branchRelations(ctx, tx, actor.VendorID, profileID)
	if err != nil {
		return fmt.Errorf("load branch relations: %w", err)
	}
	preferred := PreferredBranch(relations)

	currency := totals.Currency
	if currency == "" {
		// Facts exist but none is completed yet: the currency is still known
		// from the observed facts, so the projection is CURRENT with zeroes
		// rather than NOT_AVAILABLE.
		currency = facts[0].Currency
	}

	_, err = tx.Exec(ctx, `UPDATE vendor_customer_profiles SET
		observed_order_count=$1, completed_order_count=$2, gross_spend_minor=$3, net_spend_minor=$4, average_order_value_minor=$5,
		first_completed_order_at=$6, last_completed_order_at=$7, last_order_ref=$8, last_branch_ref=$9,
		preferred_branch_ref=$10, last_capability_type=$11, commercial_currency=$12,
		commercial_projection_state=$13, commercial_projection_version=$14,
		last_commercial_source_observed_at=$15, updated_at=now(), version=version+1
		WHERE business_id=$16 AND id=$17`,
		totals.ObservedOrderCount, totals.CompletedOrderCount, totals.GrossSpendMinor, totals.NetSpendMinor, totals.AverageOrderValueMinor,
		totals.FirstCompletedOrderAt, totals.LastCompletedOrderAt, totals.LastOrderRef, totals.LastBranchRef,
		preferred, totals.LastCapabilityType, currency,
		ProjectionCurrent, nullableVersion(sourceVersion), nullableTime(observedAt),
		actor.VendorID, profileID)
	if err != nil {
		return fmt.Errorf("update commercial profile: %w", err)
	}
	s.metrics.Inc("crm_commercial_rebuild_total")
	return s.invalidateCommercialSegmentsTx(ctx, tx, actor.VendorID)
}

func (s *Service) invalidateCommercialSegmentsTx(ctx context.Context, tx pgx.Tx, businessID string) error {
	// Commercial facts are the only source both Section 8H models read, so a
	// projection change makes every score in this tenant stale. It is marked in
	// this same transaction: a crash must not leave a scope claiming CURRENT
	// over facts that already moved.
	if err := s.MarkIntelligenceStaleTx(ctx, tx, businessID); err != nil {
		return err
	}
	return s.invalidateSegmentsForDependencies(ctx, tx, businessID, []string{
		"commercial.projection_state",
		"commercial.completed_order_count",
		"commercial.gross_spend_minor",
		"commercial.net_spend_minor",
		"commercial.average_order_value_minor",
		"commercial.first_completed_order_at",
		"commercial.last_completed_order_at",
		"commercial.days_since_last_order",
		"relationship.branch_ref",
		"relationship.capability_type",
	})
}

func (s *Service) rebuildBranchRelations(ctx context.Context, tx pgx.Tx, actor Actor, profileID string, facts []OrderFact, sourceVersion int64) error {
	byBranch := map[string]*branchAggregate{}
	for _, fact := range facts {
		if fact.BranchID == nil {
			continue
		}
		entry, ok := byBranch[*fact.BranchID]
		if !ok {
			entry = &branchAggregate{}
			byBranch[*fact.BranchID] = entry
		}
		entry.observed++
		if fact.OrderState.CountsTowardCommercialTotals() {
			entry.count++
			entry.gross += fact.GrossMinor
			entry.net += fact.NetMinor
		}
		if entry.currency == "" {
			entry.currency = fact.Currency
		}
		if fact.CompletedAt == nil {
			continue
		}
		if entry.first == nil || fact.CompletedAt.Before(*entry.first) {
			at := *fact.CompletedAt
			entry.first = &at
		}
		if entry.lastAt == nil || fact.CompletedAt.After(*entry.lastAt) {
			at := *fact.CompletedAt
			entry.lastAt = &at
			entry.last = &at
			ref := fact.SourceOrderRef
			entry.lastOrderRef = &ref
			entry.lastCapability = fact.CapabilityType
		}
	}

	// Relations that no longer have any completed fact are removed, so a
	// rebuild after a refund or merge cannot leave a branch visible to a
	// branch-scoped actor who should no longer see the relationship.
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_branch_relations
		WHERE business_id=$1 AND profile_id=$2 AND NOT (branch_id = ANY($3))`,
		actor.VendorID, profileID, branchKeys(byBranch)); err != nil {
		return err
	}

	for branchID, entry := range byBranch {
		if _, err := tx.Exec(ctx, `INSERT INTO crm_customer_branch_relations
			(id,business_id,profile_id,branch_id,first_relationship_at,last_relationship_at,
			 observed_order_count,completed_order_count,gross_spend_minor,net_spend_minor,currency,last_order_ref,
			 last_capability_type,projection_state,source_version)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (business_id,profile_id,branch_id) DO UPDATE SET
				first_relationship_at=EXCLUDED.first_relationship_at,
				last_relationship_at=EXCLUDED.last_relationship_at,
				observed_order_count=EXCLUDED.observed_order_count,
				completed_order_count=EXCLUDED.completed_order_count,
				gross_spend_minor=EXCLUDED.gross_spend_minor,
				net_spend_minor=EXCLUDED.net_spend_minor,
				currency=EXCLUDED.currency,
				last_order_ref=EXCLUDED.last_order_ref,
				last_capability_type=EXCLUDED.last_capability_type,
				projection_state=EXCLUDED.projection_state,
				source_version=EXCLUDED.source_version,
				updated_at=now(), version=crm_customer_branch_relations.version+1`,
			newID("crmbr_"), actor.VendorID, profileID, branchID, entry.first, entry.last,
			entry.observed, entry.count, entry.gross, entry.net, nullableString(entry.currency), entry.lastOrderRef,
			entry.lastCapability, ProjectionCurrent, nullableVersion(sourceVersion)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) orderFacts(ctx context.Context, q querier, businessID, profileID string) ([]OrderFact, error) {
	rows, err := q.Query(ctx, `SELECT id,business_id,profile_id,source_service,source_order_ref,source_event_id,
		source_version,order_state,order_type,corporate_customer_id,completed_at,currency,gross_amount_minor,net_amount_minor,
		refunded_amount_minor,branch_id,capability_type,last_observed_at
		FROM crm_customer_order_facts WHERE business_id=$1 AND profile_id=$2`, businessID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var facts []OrderFact
	for rows.Next() {
		var f OrderFact
		if err = rows.Scan(&f.ID, &f.BusinessID, &f.ProfileID, &f.SourceService, &f.SourceOrderRef,
			&f.SourceEventID, &f.SourceVersion, &f.OrderState, &f.OrderType, &f.CorporateCustomerID, &f.CompletedAt, &f.Currency,
			&f.GrossMinor, &f.NetMinor, &f.RefundedMinor, &f.BranchID, &f.CapabilityType, &f.LastObservedAt); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

func (s *Service) branchRelations(ctx context.Context, q querier, businessID, profileID string) ([]BranchRelation, error) {
	rows, err := q.Query(ctx, `SELECT id,business_id,profile_id,branch_id,first_relationship_at,last_relationship_at,
		observed_order_count,completed_order_count,gross_spend_minor,net_spend_minor,currency,last_order_ref,last_capability_type,
		projection_state,source_version,version
		FROM crm_customer_branch_relations WHERE business_id=$1 AND profile_id=$2 ORDER BY branch_id`,
		businessID, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var relations []BranchRelation
	for rows.Next() {
		var r BranchRelation
		if err = rows.Scan(&r.ID, &r.BusinessID, &r.ProfileID, &r.BranchID, &r.FirstRelationshipAt,
			&r.LastRelationshipAt, &r.ObservedOrderCount, &r.CompletedOrderCount, &r.GrossSpendMinor, &r.NetSpendMinor,
			&r.Currency, &r.LastOrderRef, &r.LastCapabilityType, &r.ProjectionState, &r.SourceVersion, &r.Version); err != nil {
			return nil, err
		}
		relations = append(relations, r)
	}
	return relations, rows.Err()
}

func (s *Service) businessCurrency(ctx context.Context, q querier, businessID string) (string, error) {
	var currency string
	err := q.QueryRow(ctx, `SELECT currency FROM vendor_businesses WHERE id=$1`, businessID).Scan(&currency)
	return currency, err
}

func (s *Service) branchBelongsToBusiness(ctx context.Context, q querier, businessID, branchID string) (bool, error) {
	var owned bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE id=$1 AND business_id=$2)`,
		branchID, businessID).Scan(&owned)
	return owned, err
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// branchAggregate accumulates one branch's slice of a profile's order facts
// during a rebuild.
type branchAggregate struct {
	first, last    *time.Time
	observed       int
	count          int
	gross, net     int64
	currency       string
	lastOrderRef   *string
	lastCapability *string
	lastAt         *time.Time
}

func normalizedOrderType(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "" {
		return "STANDARD"
	}
	switch v {
	case "STANDARD", "BULK", "RECURRING", "CORPORATE":
		return v
	}
	return "STANDARD"
}

func (s *Service) replaceOrderFactLines(ctx context.Context, tx pgx.Tx, actor Actor, profileID, orderFactID string, in OrderFactInput) error {
	if orderFactID == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM crm_customer_order_fact_lines WHERE business_id=$1 AND order_fact_id=$2`, actor.VendorID, orderFactID); err != nil {
		return err
	}
	for _, line := range in.Lines {
		if strings.TrimSpace(line.CategoryRef) == "" && strings.TrimSpace(line.ItemRef) == "" {
			return apperrors.BadRequest("An order line must identify a category or item.", nil)
		}
		if line.Quantity < 0 || line.NetLineAmountMinor < 0 {
			return apperrors.BadRequest("Order line quantity and amount cannot be negative.", nil)
		}
		branch := line.BranchID
		if branch == nil {
			branch = in.BranchID
		}
		if branch != nil {
			owned, err := s.branchBelongsToBusiness(ctx, tx, actor.VendorID, *branch)
			if err != nil {
				return err
			}
			if !owned {
				return errCrossTenant()
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO crm_customer_order_fact_lines
			(id,business_id,order_fact_id,profile_id,branch_id,category_ref,item_ref,quantity,net_line_amount_minor)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, newID("crmline_"), actor.VendorID, orderFactID, profileID,
			branch, optional(line.CategoryRef), optional(line.ItemRef), line.Quantity, line.NetLineAmountMinor); err != nil {
			return err
		}
	}
	return nil
}

func branchKeys(m map[string]*branchAggregate) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func nullableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nullableVersion(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func nullableTime(v time.Time) *time.Time {
	if v.IsZero() {
		return nil
	}
	return &v
}
