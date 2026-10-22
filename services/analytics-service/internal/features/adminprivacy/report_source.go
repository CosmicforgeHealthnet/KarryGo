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
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
)

// ReportSource reduces only native events with independently signed memberships.
// Until an owner coverage export exists, the selected subset is obligatorily
// partial and every value is withheld by the existing disclosure service.
type ReportSource struct {
	Report      string
	Authorities map[string]admincontext.Authority
	UnitField   string // explicit approved field; no default privacy-unit choice
}

func (s *ReportSource) SourceContract() string {
	r, _ := analyticsadmin.ReportNamed(s.Report)
	return r.Contract + ":" + s.UnitField
}
func (s *ReportSource) ResolveScope(ctx context.Context, tx pgx.Tx, q Query) (adminpolicy.Facts, error) {
	snapshot, err := s.Snapshot(ctx, tx, q)
	return snapshot.Facts, err
}
func (s *ReportSource) Snapshot(ctx context.Context, tx pgx.Tx, q Query) (Snapshot, error) {
	var empty Snapshot
	if s == nil || q.Metric != s.Report || !validReportQuery(q) || len(q.Filters) != 2 || !networkanalytics.ValidAuthority(q.Filters["authority"]) || !adminread.Opaque(q.Filters["product_code"]) || q.Start.IsZero() || !q.End.After(q.Start) {
		return empty, ErrUnmapped
	}
	kind, err := LineageKind(q.Metric, s.UnitField)
	if err != nil {
		return empty, err
	}
	if complete, ok, err := tryComplete(ctx, tx, s.Authorities, s.Report, s.UnitField, q); err != nil {
		return empty, err
	} else if ok {
		return complete, nil
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
	rows, err := tx.Query(ctx, `SELECT DISTINCT h.resource_ref,e.occurred_at FROM admin_owner_context_heads h JOIN analytics_network_events e ON e.event_id=h.resource_ref WHERE h.owner=$1 AND h.resource_kind=$2 AND h.product=$3 AND h.market=$4 AND h.zone=$5 AND h.organization_ref=$6 AND h.tenant_ref=$7 AND $8=ANY(h.capabilities) AND e.status='PROCESSED' AND e.occurred_at>=$9 AND e.occurred_at<$10 ORDER BY e.occurred_at,h.resource_ref LIMIT 101`, owner, kind, q.Scope.Product, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, "analytics.cohorts."+s.Report+".export", q.Start, q.End)
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
	report, _ := analyticsadmin.ReportNamed(s.Report)
	keys := report.Keys
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
		allocation, err := store.Resolve(ctx, tx, kind, ref, "analytics.cohorts."+s.Report+".export", q.Scope, time.Now().UTC())
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
		var payload []byte
		if err := tx.QueryRow(ctx, "SELECT payload FROM analytics_network_events WHERE event_id=$1", ref).Scan(&payload); err != nil {
			return empty, err
		}
		values, err := reportComponents(s.Report, fact, payload)
		if err != nil {
			return empty, err
		}
		sourceScope = allocation.Scope
		for key, value := range values {
			if _, ok := units[key]; !ok || value < 0 || value > math.MaxInt64-counts[key] {
				return empty, ErrSource
			}
			counts[key] += value
			if value > 0 {
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
	witness := reportAuthorization{Version: 1, Owner: owner, Metric: q.Metric, Field: s.UnitField, SourceVersion: out.SourceVersion, Scope: sourceScope, Members: []reportMember{}}
	for _, fact := range facts {
		witness.Members = append(witness.Members, reportMember{EventID: fact.EventID, FactHash: admincontext.Fingerprint(fact)})
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

type reportMember struct {
	EventID  string `json:"event_id"`
	FactHash string `json:"fact_hash"`
}
type reportAuthorization struct {
	Version       int               `json:"version"`
	Owner         string            `json:"owner"`
	Metric        string            `json:"metric"`
	Field         string            `json:"field"`
	SourceVersion string            `json:"source_version"`
	Scope         adminpolicy.Scope `json:"scope"`
	Members       []reportMember    `json:"members"`
}

// AuthorizeRelease checks the frozen original member set rather than today's
// candidate index. Dropping one revoked member from a new snapshot cannot make
// an earlier release authorized. Counts and release values are never resampled.
func (s *ReportSource) AuthorizeRelease(ctx context.Context, tx pgx.Tx, q Query, version string, raw json.RawMessage) error {
	if s == nil || len(raw) == 0 || len(raw) > 65536 || q.Metric != s.Report || !validReportQuery(q) || len(q.Filters) != 2 {
		return ErrUnmapped
	}
	if isCompleteWitness(raw) {
		return authorizeComplete(ctx, tx, s.Authorities, s.UnitField, q, version, raw)
	}
	var witness reportAuthorization
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
		allocation, err := store.Resolve(ctx, tx, kind, member.EventID, "analytics.cohorts."+s.Report+".export", q.Scope, time.Now().UTC())
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

func validReportQuery(q Query) bool {
	r, ok := analyticsadmin.ReportNamed(q.Metric)
	return ok && r.Permission == q.Capability && q.Metric != "mission-success" && q.Metric != "readiness" && q.Metric != "supply" && q.Metric != "demand-forecast" && derivedReports[q.Metric] == ""
}

func reportComponents(report string, fact LineageFact, raw []byte) (map[string]int64, error) {
	_, kind, _ := networkanalytics.Classify(fact.EventName, fact.Authority)
	common := map[networkanalytics.Kind]string{networkanalytics.KindMissionCommitted: "committed", networkanalytics.KindMissionAssigned: "assigned", networkanalytics.KindMissionCompleted: "completed", networkanalytics.KindMissionFailed: "failed", networkanalytics.KindMissionCancelled: "cancelled", networkanalytics.KindGuaranteeBreached: "guarantee_breaches", networkanalytics.KindRecoveryResolved: "recovery_resolved", networkanalytics.KindRecoveryFailed: "recovery_failed"}
	out := map[string]int64{}
	switch report {
	case "mission-outcomes":
		if key := common[kind]; key != "" {
			out[key] = 1
		} else {
			return nil, ErrSource
		}
	case "recovery":
		key := map[networkanalytics.Kind]string{networkanalytics.KindRecoveryStarted: "started", networkanalytics.KindRecoveryReplaced: "replaced", networkanalytics.KindRecoveryResolved: "resolved", networkanalytics.KindRecoveryFailed: "failed"}[kind]
		if key == "" {
			return nil, ErrSource
		}
		out[key] = 1
	case "capability-mismatches":
		if kind != networkanalytics.KindCapabilityMismatch {
			return nil, ErrSource
		}
		out["mismatches"] = 1
	case "declines", "provider-quality":
		if kind == networkanalytics.KindProviderDeclined {
			var payload map[string]any
			if json.Unmarshal(raw, &payload) != nil {
				return nil, ErrSource
			}
			reason, _ := payload["reason_code"].(string)
			if reason == "" {
				reason, _ = payload["decline_reason"].(string)
			}
			if reason == "" {
				reason, _ = payload["reason"].(string)
			}
			if reason == "" {
				return nil, ErrSource
			}
			key := "declines"
			if networkanalytics.IsSafetyDecline(reason) {
				key = "safety_declines"
			}
			out[key] = 1
		} else if report == "provider-quality" {
			key := map[networkanalytics.Kind]string{networkanalytics.KindMissionCompleted: "completed", networkanalytics.KindMissionFailed: "failed", networkanalytics.KindMissionCancelled: "cancelled", networkanalytics.KindProviderArrived: "arrivals", networkanalytics.KindRecoveryStarted: "recoveries_as_original", networkanalytics.KindRecoveryReplaced: "recoveries_as_replacement"}[kind]
			if key == "" {
				return nil, ErrSource
			}
			out[key] = 1
		} else {
			return nil, ErrSource
		}
	case "demand", "demand-forecast":
		// A committed mission is one individual request; an owner's explicit
		// unmatched outcome is one unmatched request. Other failures are not
		// demand facts and contribute nothing (never an invented zero/one).
		switch kind {
		case networkanalytics.KindMissionCommitted:
			out["requests"] = 1
			return out, nil
		case networkanalytics.KindMissionFailed:
			if unmatchedEvents[fact.EventName] {
				out["unmatched"] = 1
			}
			return out, nil
		}
		if kind != networkanalytics.KindDemandObserved {
			return nil, ErrUnmapped
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var payload map[string]any
		if decoder.Decode(&payload) != nil {
			return nil, ErrSource
		}
		requests, ok := payload["requests"].(json.Number)
		if !ok {
			return nil, ErrUnmapped
		}
		n, err := requests.Int64()
		if err != nil || n != 1 {
			return nil, ErrUnmapped
		}
		// A multi-request bucket has no individual roster, even with an entity ID.
		unmatched, ok := payload["unmatched"].(bool)
		if !ok {
			return nil, ErrUnmapped
		}
		out["requests"] = 1
		if unmatched {
			out["unmatched"] = 1
		}
	default:
		return nil, ErrUnmapped
	}
	return out, nil
}

// unmatchedEvents are the owner events that record a request nobody served.
// An authority without such an event has an UNKNOWN unmatched count.
var unmatchedEvents = map[string]bool{"taxi.booking.unmatched": true, "hauling.booking.unmatched.v1": true}

// unmatchedKnown reports whether an authority publishes an explicit unmatched
// outcome. Dispatch publishes delivery failure, which is not unmatched demand.
func unmatchedKnown(authority string) bool {
	return authority == networkanalytics.AuthorityTaxi || authority == networkanalytics.AuthorityHauling
}
