package adminprivacy

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
)

// MissionSuccessSource reads a bounded, independently authorized subset of
// canonical native events. It intentionally never claims complete membership:
// a source-owned coverage/export contract is still required before any values
// can be disclosed. No legacy aggregate, bucket count or query tuple is used
// as a privacy cohort. This adapter is not registered in a production runtime.
type MissionSuccessSource struct {
	Authorities map[string]admincontext.Authority
	UnitField   string // explicit approved field; no default privacy-unit choice
}

func (s *MissionSuccessSource) SourceContract() string {
	return "analytics.scoped-mission-success.v1:" + s.UnitField
}
func (s *MissionSuccessSource) ResolveScope(ctx context.Context, tx pgx.Tx, q Query) (adminpolicy.Facts, error) {
	snapshot, err := s.Snapshot(ctx, tx, q)
	return snapshot.Facts, err
}
func (s *MissionSuccessSource) Snapshot(ctx context.Context, tx pgx.Tx, q Query) (Snapshot, error) {
	var empty Snapshot
	if s == nil || q.Metric != "mission-success" || q.Capability != "analytics.network.read" || len(q.Filters) != 2 || !networkanalytics.ValidAuthority(q.Filters["authority"]) || !adminread.Opaque(q.Filters["product_code"]) || q.Start.IsZero() || !q.End.After(q.Start) {
		return empty, ErrUnmapped
	}
	kind, err := LineageKind(q.Metric, s.UnitField)
	if err != nil {
		return empty, err
	}
	owner := ""
	switch q.Filters["authority"] {
	case "TAXI":
		owner = "driver-taxi-service"
	case "DISPATCH":
		owner = "driver-dispatch-delivery-service"
	case "HAULING":
		owner = "driver-hauling-service"
	}
	authority, ok := s.Authorities[owner]
	if !ok || authority.Owner != owner {
		return empty, ErrUnmapped
	}
	reader := LineageReader{Owner: owner}
	store := admincontext.Store{Reader: reader, Authority: authority}
	// The index supplies candidates, never authorization. Every candidate must
	// resolve its authentic current owner context and native binding below.
	rows, err := tx.Query(ctx, `SELECT DISTINCT h.resource_ref,e.occurred_at FROM admin_owner_context_heads h JOIN analytics_network_events e ON e.event_id=h.resource_ref WHERE h.owner=$1 AND h.resource_kind=$2 AND h.product=$3 AND h.market=$4 AND h.zone=$5 AND h.organization_ref=$6 AND h.tenant_ref=$7 AND $8=ANY(h.capabilities) AND e.status='PROCESSED' AND e.occurred_at>=$9 AND e.occurred_at<$10 ORDER BY e.occurred_at,h.resource_ref LIMIT 101`, owner, kind, q.Scope.Product, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, "analytics.cohorts.mission-success.export", q.Start, q.End)
	if err != nil {
		return empty, err
	}
	refs := []string{}
	for rows.Next() {
		var ref string
		var at time.Time
		if err := rows.Scan(&ref, &at); err != nil {
			rows.Close()
			return empty, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, err
	}
	if len(refs) > 100 {
		refs = refs[:100]
	}
	keys := []string{"completed", "failed", "cancelled", "guarantee_breaches"}
	counts := map[string]int64{}
	units := map[string]map[string]bool{}
	for _, key := range keys {
		units[key] = map[string]bool{}
	}
	facts := []LineageFact{}
	var sourceScope adminpolicy.Scope
	var asOf time.Time
	for _, ref := range refs {
		// Retained facts are immutable through normal ingest. A share lock also
		// keeps an exceptional projection repair from changing native identity
		// between owner-context validation and the unit/count read.
		var locked int
		if err := tx.QueryRow(ctx, "SELECT 1 FROM analytics_network_events WHERE event_id=$1 FOR SHARE", ref).Scan(&locked); err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return empty, err
		}
		allocation, err := store.Resolve(ctx, tx, kind, ref, "analytics.cohorts.mission-success.export", q.Scope, time.Now().UTC())
		if err != nil {
			if err == admincontext.ErrUnmapped {
				continue
			}
			return empty, err
		}
		fact, err := reader.ReadUnit(ctx, tx, q.Metric, s.UnitField, ref)
		if err != nil {
			return empty, err
		}
		if allocation.Scope != q.Scope || allocation.SourceContract != fact.SourceContract {
			return empty, ErrSource
		}
		if fact.Authority != q.Filters["authority"] || fact.ProductCode != q.Filters["product_code"] || fact.OccurredAt.Before(q.Start) || !fact.OccurredAt.Before(q.End) {
			continue
		}
		_, nativeKind, _ := networkanalytics.Classify(fact.EventName, fact.Authority)
		key := ""
		switch nativeKind {
		case networkanalytics.KindMissionCompleted:
			key = "completed"
		case networkanalytics.KindMissionFailed:
			key = "failed"
		case networkanalytics.KindMissionCancelled:
			key = "cancelled"
		case networkanalytics.KindGuaranteeBreached:
			key = "guarantee_breaches"
		default:
			return empty, ErrSource
		}
		sourceScope = allocation.Scope
		counts[key]++
		units[key][fact.SubjectHash] = true
		facts = append(facts, fact)
		if fact.RecordedAt.After(asOf) {
			asOf = fact.RecordedAt
		}
	}
	if len(facts) == 0 {
		return empty, ErrUnmapped
	}
	out := Snapshot{Facts: adminpolicy.Facts{Owner: "analytics-service", Reference: facts[0].EventID, SourceContract: s.SourceContract(), Scope: sourceScope}, SourceVersion: admincontext.Fingerprint(facts), AsOf: asOf, Partial: true, Cells: []Cell{}}
	for _, key := range keys {
		count, cohort := counts[key], int64(len(units[key]))
		out.Cells = append(out.Cells, Cell{Key: key, Count: &count, CohortSize: &cohort})
	}
	return out, nil
}
