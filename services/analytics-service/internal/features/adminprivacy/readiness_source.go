package adminprivacy

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
)

// ReadinessSource reduces native Vendor order readiness components over a
// bounded independently authorized subset. It never asserts complete membership
// and has no runtime/HTTP registration. Every value is suppressed while partial.
type ReadinessSource struct {
	Authorities map[string]admincontext.Authority
	UnitField   string // explicit approved field; no default privacy-unit choice
}

func (s *ReadinessSource) SourceContract() string {
	return "analytics.scoped-readiness-components.v1:" + s.UnitField
}
func (s *ReadinessSource) ResolveScope(ctx context.Context, tx pgx.Tx, q Query) (adminpolicy.Facts, error) {
	snapshot, err := s.Snapshot(ctx, tx, q)
	return snapshot.Facts, err
}
func (s *ReadinessSource) Snapshot(ctx context.Context, tx pgx.Tx, q Query) (Snapshot, error) {
	var empty Snapshot
	if s == nil || (s.UnitField != "branch_id" && s.UnitField != "source_entity_id") || q.Metric != "readiness" || q.Capability != readinessComponentsPermission || len(q.Filters) != 2 || q.Filters["authority"] != networkanalytics.AuthorityVendor || !adminread.Opaque(q.Filters["product_code"]) || q.Start.IsZero() || !q.End.After(q.Start) {
		return empty, ErrUnmapped
	}
	kind, err := LineageKind(q.Metric, s.UnitField)
	if err != nil {
		return empty, err
	}
	owner := "vendor-service"
	authority, ok := s.Authorities[owner]
	if !ok || authority.Owner != owner {
		return empty, ErrUnmapped
	}
	reader := LineageReader{Owner: owner}
	store := admincontext.Store{Reader: reader, Authority: authority}
	// The index supplies candidates, never authorization. Every candidate must
	// resolve its authentic current owner context and native binding below.
	rows, err := tx.Query(ctx, `SELECT DISTINCT h.resource_ref,e.occurred_at FROM admin_owner_context_heads h JOIN analytics_network_events e ON e.event_id=h.resource_ref WHERE h.owner=$1 AND h.resource_kind=$2 AND h.product=$3 AND h.market=$4 AND h.zone=$5 AND h.organization_ref=$6 AND h.tenant_ref=$7 AND $8=ANY(h.capabilities) AND e.status='PROCESSED' AND e.occurred_at>=$9 AND e.occurred_at<$10 ORDER BY e.occurred_at,h.resource_ref LIMIT 101`, owner, kind, q.Scope.Product, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, "analytics.cohorts.readiness.export", q.Start, q.End)
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
	keys := readinessComponentKeys
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
		allocation, err := store.Resolve(ctx, tx, kind, ref, "analytics.cohorts.readiness.export", q.Scope, time.Now().UTC())
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

		// Values are read from the same locked native event whose exact minimized
		// payload hash was verified above. No default zero/false for absent data.
		var payload []byte
		if err := tx.QueryRow(ctx, "SELECT payload FROM analytics_network_events WHERE event_id=$1", ref).Scan(&payload); err != nil {
			return empty, err
		}
		values, err := readinessComponents(payload)
		if err != nil {
			return empty, err
		}
		sourceScope = allocation.Scope
		for _, key := range keys {
			value := values[key]
			if value > math.MaxInt64-counts[key] {
				return empty, ErrSource
			}
			counts[key] += value
			// The contributing population for each component is explicit. Zero-value
			// observations still belong to ready_count, not to a fabricated late/prep cohort.
			if key == "ready_count" || key == "on_time_count" && value > 0 || key == "late_count" && value > 0 || key == "lateness_seconds_total" && values["late_count"] > 0 || (key == "prep_count" || key == "prep_seconds_total") && values["prep_count"] > 0 {
				units[key][fact.SubjectHash] = true
			}
		}
		facts = append(facts, fact)
		if fact.RecordedAt.After(asOf) {
			asOf = fact.RecordedAt
		}
	}
	if len(facts) == 0 {
		return empty, ErrUnmapped
	}
	out := Snapshot{Facts: adminpolicy.Facts{Owner: "analytics-service", Reference: facts[0].EventID, SourceContract: s.SourceContract(), Scope: sourceScope}, SourceVersion: admincontext.Fingerprint(facts), AsOf: asOf, Partial: true, Cells: []Cell{}}
	witness := readinessAuthorization{Version: 1, Owner: owner, Metric: q.Metric, Field: s.UnitField, SourceVersion: out.SourceVersion, Scope: sourceScope, Members: []readinessMember{}}
	for _, fact := range facts {
		witness.Members = append(witness.Members, readinessMember{EventID: fact.EventID, FactHash: admincontext.Fingerprint(fact)})
	}
	out.Authorization, err = json.Marshal(witness)
	if err != nil {
		return empty, err
	}
	for _, key := range keys {
		count, cohort := counts[key], int64(len(units[key]))
		out.Cells = append(out.Cells, Cell{Key: key, Count: &count, CohortSize: &cohort})
	}
	return out, nil
}

type readinessMember struct {
	EventID  string `json:"event_id"`
	FactHash string `json:"fact_hash"`
}
type readinessAuthorization struct {
	Version       int               `json:"version"`
	Owner         string            `json:"owner"`
	Metric        string            `json:"metric"`
	Field         string            `json:"field"`
	SourceVersion string            `json:"source_version"`
	Scope         adminpolicy.Scope `json:"scope"`
	Members       []readinessMember `json:"members"`
}

// AuthorizeRelease checks the frozen original member set rather than today's
// candidate index. Dropping one revoked member from a new snapshot cannot make
// an earlier release authorized. Counts and release values are never resampled.
func (s *ReadinessSource) AuthorizeRelease(ctx context.Context, tx pgx.Tx, q Query, version string, raw json.RawMessage) error {
	if s == nil || (s.UnitField != "branch_id" && s.UnitField != "source_entity_id") || q.Filters["authority"] != networkanalytics.AuthorityVendor || len(raw) == 0 || len(raw) > 65536 || q.Metric != "readiness" || q.Capability != readinessComponentsPermission || len(q.Filters) != 2 {
		return ErrUnmapped
	}
	var witness readinessAuthorization
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&witness) != nil {
		return ErrUnmapped
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrUnmapped
	}
	if witness.Version != 1 || witness.Metric != q.Metric || witness.Field != s.UnitField || witness.SourceVersion != version || witness.Scope != q.Scope || len(witness.Members) == 0 || len(witness.Members) > 100 {
		return ErrUnmapped
	}
	owner, authorityCode := canonicalLineageOwner(witness.Owner)
	authority, ok := s.Authorities[owner]
	if !ok || owner != witness.Owner || authority.Owner != owner || authorityCode != q.Filters["authority"] {
		return ErrUnmapped
	}
	kind, err := LineageKind(q.Metric, s.UnitField)
	if err != nil {
		return err
	}
	reader := LineageReader{Owner: owner}
	store := admincontext.Store{Reader: reader, Authority: authority}
	seen := map[string]bool{}
	for _, member := range witness.Members {
		digest, err := hex.DecodeString(member.FactHash)
		if !adminread.Opaque(member.EventID) || seen[member.EventID] || err != nil || len(digest) != 32 {
			return ErrUnmapped
		}
		seen[member.EventID] = true
		var locked int
		if err := tx.QueryRow(ctx, "SELECT 1 FROM analytics_network_events WHERE event_id=$1 FOR SHARE", member.EventID).Scan(&locked); err != nil {
			return err
		}
		allocation, err := store.Resolve(ctx, tx, kind, member.EventID, "analytics.cohorts.readiness.export", q.Scope, time.Now().UTC())
		if err != nil {
			return err
		}
		fact, err := reader.ReadUnit(ctx, tx, q.Metric, s.UnitField, member.EventID)
		if err != nil {
			return err
		}
		if allocation.Scope != q.Scope || allocation.SourceContract != fact.SourceContract || admincontext.Fingerprint(fact) != member.FactHash || fact.Authority != q.Filters["authority"] || fact.ProductCode != q.Filters["product_code"] || fact.OccurredAt.Before(q.Start) || !fact.OccurredAt.Before(q.End) {
			return ErrUnmapped
		}
	}
	return nil
}

const readinessComponentsPermission = "analytics.scoped.readiness-components.read"

var readinessComponentKeys = []string{"ready_count", "on_time_count", "late_count", "lateness_seconds_total", "prep_seconds_total", "prep_count"}

func readinessComponents(raw []byte) (map[string]int64, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var p map[string]any
	if decoder.Decode(&p) != nil || p == nil {
		return nil, ErrSource
	}
	onTime, ok := p["on_time"].(bool)
	if !ok {
		return nil, ErrSource
	}
	seconds := func(key string) (int64, error) {
		n, ok := p[key].(json.Number)
		if !ok {
			return 0, ErrSource
		}
		v, err := n.Int64()
		if err != nil || v < 0 {
			return 0, ErrSource
		}
		return v, nil
	}
	late, err := seconds("lateness_seconds")
	if err != nil {
		return nil, err
	}
	prep, err := seconds("prep_seconds")
	if err != nil {
		return nil, err
	}
	// A producer's contradictory observation is not repaired into a result.
	if onTime && late != 0 {
		return nil, ErrSource
	}
	out := map[string]int64{"ready_count": 1, "on_time_count": 0, "late_count": 0, "lateness_seconds_total": late, "prep_seconds_total": prep, "prep_count": 0}
	if onTime {
		out["on_time_count"] = 1
	} else {
		out["late_count"] = 1
	}
	if prep > 0 {
		out["prep_count"] = 1
	}
	return out, nil
}
