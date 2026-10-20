package adminprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func privacyDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANALYTICS_ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ANALYTICS_ADMIN_TEST_DATABASE_URL required for real disclosure journal")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("isolated Analytics test DB required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "analytics_privacy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	// The owner's migration is idempotent for safe operator reapplication;
	// Analytics' normal runner additionally tracks each applied filename.
	for _, name := range []string{"005_admin_disclosure_guard.sql", "007_admin_disclosure_authorizations.sql", "008_admin_scoped_http_nonces.sql", "009_admin_source_publication.sql"} {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err = pool.Exec(ctx, string(raw)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return pool
}

// This fixture tests disclosure mechanics, not a production metrics adapter.
// Real network reports remain unmapped; no source scopes are synthesized there.
type testCohortSource struct {
	snapshot         Snapshot
	reads            atomic.Int32
	deny             atomic.Bool
	membershipDenied atomic.Bool
}

func (s *testCohortSource) ResolveScope(context.Context, pgx.Tx, Query) (adminpolicy.Facts, error) {
	facts := s.snapshot.Facts
	if s.deny.Load() {
		facts.Scope.Tenant = "foreign"
	}
	return facts, nil
}
func (s *testCohortSource) Snapshot(context.Context, pgx.Tx, Query) (Snapshot, error) {
	s.reads.Add(1)
	out := s.snapshot
	out.Authorization = json.RawMessage(`{"fixture":"TEST_ONLY_ORIGINAL_MEMBERSHIP"}`)
	return out, nil
}
func (s *testCohortSource) AuthorizeRelease(_ context.Context, _ pgx.Tx, _ Query, _ string, raw json.RawMessage) error {
	var witness struct {
		Fixture string `json:"fixture"`
	}
	if s.membershipDenied.Load() || json.Unmarshal(raw, &witness) != nil || witness.Fixture != "TEST_ONLY_ORIGINAL_MEMBERSHIP" {
		return ErrUnmapped
	}
	return nil
}

func TestPostgresFrozenReleaseRequiresOriginalMembershipWithoutResampling(t *testing.T) {
	db := privacyDB(t)
	f, actor, q, snapshot := privacyFixture(t)
	source := &testCohortSource{snapshot: snapshot}
	service := &Service{DB: db, Policy: f.Engine, Source: source}
	ctx := context.Background()
	first, err := service.Disclose(ctx, actor, q)
	if err != nil {
		t.Fatal(err)
	}
	source.membershipDenied.Store(true)
	if _, err := service.Disclose(ctx, actor, q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("frozen members not reauthorized", err)
	}
	source.membershipDenied.Store(false)
	source.snapshot.SourceVersion = "NEW_NATIVE_VERSION"
	source.snapshot.Cells[0].Count = number(99)
	again, err := service.Disclose(ctx, actor, q)
	if err != nil || again.SourceVersion != first.SourceVersion || *again.Cells[0].Value != *first.Cells[0].Value || source.reads.Load() != 1 {
		t.Fatal("membership check resampled or altered frozen data", again, err)
	}
}

func TestPostgresLegacyFrozenReleaseWithoutMembershipFailsClosed(t *testing.T) {
	db := privacyDB(t)
	f, actor, q, snapshot := privacyFixture(t)
	source := &testCohortSource{snapshot: snapshot}
	service := &Service{DB: db, Policy: f.Engine, Source: source}
	ctx := context.Background()
	p, err := f.Engine.Disclosure(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	queryHash := hash(struct {
		Scope                      adminpolicy.Scope
		Capability, Metric, Domain string
		Start, End                 time.Time
		Filters                    map[string]string
	}{q.Scope, q.Capability, q.Metric, p.PrivacyDomain, q.Start, q.End, q.Filters})
	release, err := suppress(q, snapshot, p, f.Engine, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(release)
	if _, err := db.Exec(ctx, `INSERT INTO analytics_admin_disclosures(release_id,query_hash,policy_checksum,released_payload) VALUES($1,$1,$2,$3)`, queryHash, f.Engine.Checksum(), raw); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disclose(ctx, actor, q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("legacy cache invented original membership", err)
	}
	if source.reads.Load() != 0 {
		t.Fatal("legacy cache resampled values")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_query_audit WHERE outcome='FROZEN_MEMBERSHIP_UNPROVEN'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("missing proof not audited", count, err)
	}
}

func TestPostgresDisclosureMembershipAndReleaseRollBackTogetherOnAuditFailure(t *testing.T) {
	db := privacyDB(t)
	f, actor, q, snapshot := privacyFixture(t)
	source := &testCohortSource{snapshot: snapshot}
	service := &Service{DB: db, Policy: f.Engine, Source: source}
	ctx := context.Background()
	if _, err := db.Exec(ctx, `ALTER TABLE analytics_admin_query_audit RENAME TO unavailable_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disclose(ctx, actor, q); err == nil {
		t.Fatal("missing audit released values")
	}
	for _, table := range []string{"analytics_admin_disclosures", "analytics_admin_disclosure_authorizations", "analytics_admin_disclosure_partitions"} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("partial disclosure evidence committed", table, count, err)
		}
	}
}

func TestPostgresDisclosureAuditFrozenReleaseAndReconstructionGuard(t *testing.T) {
	db := privacyDB(t)
	f, actor, q, snapshot := privacyFixture(t)
	source := &testCohortSource{snapshot: snapshot}
	service := &Service{DB: db, Policy: f.Engine, Source: source}
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := service.Disclose(ctx, actor, q)
			if err != nil {
				t.Error(err)
				return
			}
			if len(out.Cells) != 2 || out.Cells[0].Value == nil || *out.Cells[0].Value != 20 {
				t.Errorf("release=%+v", out)
			}
		}()
	}
	wg.Wait()
	if source.reads.Load() != 1 {
		t.Fatalf("concurrent source resampling=%d", source.reads.Load())
	}
	another := actor
	another.Staff = "staff-2"
	if _, err := service.Disclose(ctx, another, q); err != nil {
		t.Fatal(err)
	}
	varied := q
	varied.Filters = map[string]string{"authority": "TAXI", "product_code": "TAXI_GOODS"}
	if _, err := service.Disclose(ctx, actor, varied); !errors.Is(err, ErrReconstruction) {
		t.Fatalf("filter differencing allowed: %v", err)
	}
	overlap := q
	overlap.Start = overlap.Start.Add(time.Hour)
	overlap.End = overlap.End.Add(time.Hour)
	if _, err := service.Disclose(ctx, actor, overlap); !errors.Is(err, ErrReconstruction) {
		t.Fatalf("overlapping-day release allowed: %v", err)
	}
	otherReport := q
	otherReport.Metric = "mission-outcomes"
	if _, err := service.Disclose(ctx, actor, otherReport); !errors.Is(err, ErrReconstruction) {
		t.Fatalf("cross-report differencing allowed: %v", err)
	}
	source.snapshot.Cells[0].Count = number(99)
	source.snapshot.SourceVersion = "test-source-v2"
	cached, err := service.Disclose(ctx, actor, q)
	if err != nil || *cached.Cells[0].Value != 20 || source.reads.Load() != 1 {
		t.Fatalf("source update changed frozen disclosure: %+v %v", cached, err)
	}
	source.deny.Store(true)
	if _, err = service.Disclose(ctx, actor, q); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatalf("cached result bypassed owner authorization: %v", err)
	}
	source.deny.Store(false)
	old := f.Engine
	f.Document.Version = 2
	f.Sign(t)
	service.Policy = f.Engine
	if _, err = service.Disclose(ctx, actor, q); !errors.Is(err, ErrReconstruction) {
		t.Fatalf("new policy resampled old window: %v", err)
	}
	service.Policy = old
	if _, err = service.Disclose(ctx, actor, q); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatalf("approval rollback accepted: %v", err)
	}
	service.Policy = f.Engine
	var audits, releases, guards int
	f.Document.Rules[0].Purposes = append(f.Document.Rules[0].Purposes, "new-purpose")
	f.Sign(t)
	service.Policy = f.Engine
	if _, err = service.Disclose(ctx, actor, q); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatalf("same-version approval rewrite accepted: %v", err)
	}
	db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_query_audit`).Scan(&audits)
	db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_disclosures`).Scan(&releases)
	db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_query_audit WHERE outcome='RECONSTRUCTION_BLOCKED'`).Scan(&guards)
	if audits < 18 || releases != 1 || guards != 3 {
		t.Fatalf("audit/release/guard=%d/%d/%d", audits, releases, guards)
	}
	var payload []byte
	if err = db.QueryRow(ctx, `SELECT released_payload FROM analytics_admin_disclosures`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(payload) || strings.Contains(string(payload), "SECRET") || strings.Contains(string(payload), "cohort_size") || strings.Contains(string(payload), "staff-1") {
		t.Fatalf("raw source/actor leaked in released cache: %s", payload)
	}
	if _, err = db.Exec(ctx, `UPDATE analytics_admin_disclosures SET released_payload='{}'`); err == nil {
		t.Fatal("disclosure mutable")
	}
	if _, err = db.Exec(ctx, `DELETE FROM analytics_admin_disclosure_partitions`); err == nil {
		t.Fatal("reconstruction guard could be erased")
	}
}
func TestPostgresDisclosureBudgetAndAuditFailureDenyData(t *testing.T) {
	db := privacyDB(t)
	f, actor, q, snapshot := privacyFixture(t)
	f.Document.Analytics.MaxQueriesPerStaffWindow = 2
	f.Sign(t)
	source := &testCohortSource{snapshot: snapshot}
	service := &Service{DB: db, Policy: f.Engine, Source: source}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := service.Disclose(ctx, actor, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Disclose(ctx, actor, q); !errors.Is(err, ErrRate) {
		t.Fatalf("query budget failed open: %v", err)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE analytics_admin_query_audit RENAME TO unavailable_audit`); err != nil {
		t.Fatal(err)
	}
	actor.Staff = "staff-3"
	q.Start = q.Start.Add(-24 * time.Hour)
	q.End = q.End.Add(-24 * time.Hour)
	out, err := service.Disclose(ctx, actor, q)
	if err == nil || len(out.Cells) != 0 {
		t.Fatalf("audit failure released data: %+v %v", out, err)
	}
	var releases int
	db.QueryRow(ctx, `SELECT count(*) FROM analytics_admin_disclosures`).Scan(&releases)
	if releases != 1 {
		t.Fatal("failed audit left disclosure committed")
	}
}
func TestMissingApprovalAndUnmappedSourceCannotDisclose(t *testing.T) {
	f, actor, q, _ := privacyFixture(t)
	if _, err := (&Service{}).Disclose(context.Background(), actor, q); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("missing approval allowed", err)
	}
	if _, err := (&Service{Policy: f.Engine}).Disclose(context.Background(), actor, q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("unmapped source allowed", err)
	}
	f.Document.Analytics = nil
	f.Sign(t)
	if _, err := (&Service{Policy: f.Engine}).Disclose(context.Background(), actor, q); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("missing suppression values acquired defaults", err)
	}
}
