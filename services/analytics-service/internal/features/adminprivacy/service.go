package adminprivacy

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source must read only authoritative Analytics-local projections in the
// supplied transaction, and independently resolve scope facts. It is not a
// generic SQL executor and must never replace missing tenant facts with query
// parameters. No legacy unscoped report is wired to this port.
type Source interface {
	ResolveScope(context.Context, pgx.Tx, Query) (adminpolicy.Facts, error)
	Snapshot(context.Context, pgx.Tx, Query) (Snapshot, error)
}
type Service struct {
	DB     *pgxpool.Pool
	Policy *adminpolicy.Engine
	Source Source
	Now    func() time.Time
}

func (s *Service) Disclose(ctx context.Context, actor adminpolicy.Actor, q Query) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var empty Release
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	q.Start = q.Start.UTC()
	q.End = q.End.UTC()
	if err := s.Policy.AuthorizeDelegation(actor, q.Capability, q.Purpose, q.Scope, now); err != nil {
		return empty, err
	}
	p, err := s.Policy.Disclosure(now)
	if err != nil {
		return empty, err
	}
	if s.DB == nil || s.Source == nil {
		return empty, ErrUnmapped
	}
	if err = adminpolicy.PinRevision(ctx, s.DB, "analytics_admin_policy_revisions", s.Policy); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	queryHash := hash(struct {
		Scope                      adminpolicy.Scope
		Capability, Metric, Domain string
		Start, End                 time.Time
		Filters                    map[string]string
	}{q.Scope, q.Capability, q.Metric, p.PrivacyDomain, q.Start, q.End, q.Filters})
	audit := func(outcome string) error {
		_, e := tx.Exec(ctx, `INSERT INTO analytics_admin_query_audit(staff_ref,policy_id,policy_version,query_hash,purpose_code,outcome) VALUES($1,$2,$3,$4,$5,$6)`, actor.Staff, s.Policy.ID(), s.Policy.Version(), queryHash, q.Purpose, outcome)
		return e
	}
	finishDenied := func(problem error, outcome string) (Release, error) {
		if e := audit(outcome); e != nil {
			return empty, e
		}
		if e := tx.Commit(ctx); e != nil {
			return empty, e
		}
		return empty, problem
	}
	window := time.Unix(now.Unix()/int64(p.QueryWindowSeconds)*int64(p.QueryWindowSeconds), 0).UTC()
	var count int
	err = tx.QueryRow(ctx, `INSERT INTO analytics_admin_query_budgets(staff_hash,window_start,query_count) VALUES($1,$2,1)
 ON CONFLICT(staff_hash,window_start) DO UPDATE SET query_count=analytics_admin_query_budgets.query_count+1
 WHERE analytics_admin_query_budgets.query_count<$3 RETURNING query_count`, hash(struct{ Product, Staff string }{actor.Product, actor.Staff}), window, p.MaxQueriesPerStaffWindow).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return finishDenied(ErrRate, "QUERY_BUDGET_EXHAUSTED")
	}
	if err != nil {
		return empty, err
	}
	if err = validateQuery(q, p, now); err != nil {
		return finishDenied(err, "FILTER_OR_WINDOW_RESTRICTED")
	}
	facts, err := s.Source.ResolveScope(ctx, tx, q)
	if errors.Is(err, ErrUnmapped) {
		return finishDenied(ErrUnmapped, "SOURCE_SCOPE_UNMAPPED")
	}
	if err != nil {
		return finishDenied(ErrSource, "SOURCE_UNAVAILABLE")
	}
	if err = s.Policy.AuthorizeOwner(actor, q.Capability, q.Purpose, q.Scope, facts, now); err != nil {
		return finishDenied(err, "SOURCE_SCOPE_DENIED")
	}
	// Guard all intersecting UTC days, across actors, reports and source
	// versions. This intentionally refuses overlapping release partitions even
	// when a filter is individually allowlisted. Keeping guard records is a
	// privacy requirement, not a disposable cache optimization.
	partitions := []string{}
	for day := q.Start.Truncate(24 * time.Hour); day.Before(q.End); day = day.Add(24 * time.Hour) {
		partition := hash(struct {
			Scope       adminpolicy.Scope
			Domain, Day string
		}{q.Scope, p.PrivacyDomain, day.Format("2006-01-02")})
		raw, _ := hex.DecodeString(partition)
		lock := int64(binary.BigEndian.Uint64(raw[:8]))
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lock); err != nil {
			return empty, err
		}
		partitions = append(partitions, partition)
	}
	for _, partition := range partitions {
		var existing string
		err = tx.QueryRow(ctx, `SELECT release_id FROM analytics_admin_disclosure_partitions WHERE partition_hash=$1`, partition).Scan(&existing)
		if err == nil && existing != queryHash {
			return finishDenied(ErrReconstruction, "RECONSTRUCTION_BLOCKED")
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return empty, err
		}
	}
	var raw []byte
	var approvedChecksum string
	err = tx.QueryRow(ctx, `SELECT released_payload,policy_checksum FROM analytics_admin_disclosures WHERE release_id=$1`, queryHash).Scan(&raw, &approvedChecksum)
	if err == nil {
		if approvedChecksum != s.Policy.Checksum() {
			return finishDenied(ErrReconstruction, "POLICY_CHANGED_RELEASE_FROZEN")
		}
		var cached Release
		if json.Unmarshal(raw, &cached) != nil {
			return empty, ErrSource
		}
		cached = suppressStale(cached, p, now)
		if err = audit("FROZEN_RELEASE_READ"); err != nil {
			return empty, err
		}
		if err = tx.Commit(ctx); err != nil {
			return empty, err
		}
		return cached, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	snapshot, err := s.Source.Snapshot(ctx, tx, q)
	if errors.Is(err, ErrUnmapped) {
		return finishDenied(ErrUnmapped, "SOURCE_SCOPE_UNMAPPED")
	}
	if err != nil {
		return finishDenied(ErrSource, "SOURCE_UNAVAILABLE")
	}
	if err = s.Policy.AuthorizeOwner(actor, q.Capability, q.Purpose, q.Scope, snapshot.Facts, now); err != nil {
		return finishDenied(err, "SOURCE_SCOPE_DENIED")
	}
	if snapshot.Facts != facts {
		return finishDenied(adminpolicy.ErrDenied, "SOURCE_SCOPE_CHANGED")
	}
	release, err := suppress(q, snapshot, p, s.Policy, now)
	if err != nil {
		return finishDenied(err, "SOURCE_INVALID")
	}
	raw, err = json.Marshal(release)
	if err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO analytics_admin_disclosures(release_id,query_hash,policy_checksum,released_payload) VALUES($1,$1,$2,$3)`, queryHash, s.Policy.Checksum(), raw); err != nil {
		return empty, err
	}
	for _, partition := range partitions {
		if _, err = tx.Exec(ctx, `INSERT INTO analytics_admin_disclosure_partitions(partition_hash,release_id) VALUES($1,$2)`, partition, queryHash); err != nil {
			return empty, err
		}
	}
	if err = audit("RELEASED"); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return release, nil
}
