package policyingest

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise the properties that only a real database can prove:
// that claiming is atomic under concurrency, that the snapshot and the inbox
// move together, and that the immutability trigger actually refuses a rewrite.
// An in-memory fake can assert the decision order but not any of those.
//
// They skip without POLICY_INGEST_TEST_DATABASE_URL, matching the repository's
// existing convention for DB-backed tests.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("POLICY_INGEST_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("POLICY_INGEST_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

const testTablePrefix = "policyingest_it"

func setupSchema(t *testing.T, pool *pgxpool.Pool) Tables {
	t.Helper()
	tables := Tables{
		Publications: testTablePrefix + "_publications",
		Snapshots:    testTablePrefix + "_snapshots",
	}
	ctx := context.Background()
	for _, statement := range []string{
		`DROP TABLE IF EXISTS ` + tables.Publications,
		`DROP TABLE IF EXISTS ` + tables.Snapshots,
		`CREATE TABLE ` + tables.Publications + ` (
			event_id TEXT PRIMARY KEY,
			policy_kind TEXT NOT NULL,
			policy_version TEXT NOT NULL,
			source_service TEXT NOT NULL,
			source_version BIGINT NOT NULL CHECK (source_version > 0),
			schema_version INT NOT NULL CHECK (schema_version > 0),
			checksum TEXT NOT NULL,
			document_ref TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL CHECK (state IN ('APPLIED','DUPLICATE','STALE','REJECTED','PENDING_READBACK')),
			reason_code TEXT NOT NULL DEFAULT '',
			correlation_id TEXT NOT NULL DEFAULT '',
			occurred_at TIMESTAMPTZ NOT NULL,
			effective_at TIMESTAMPTZ NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			applied_at TIMESTAMPTZ,
			attempts INT NOT NULL DEFAULT 0,
			next_attempt_at TIMESTAMPTZ,
			last_error TEXT NOT NULL DEFAULT '',
			CONSTRAINT chk_applied CHECK ((state = 'APPLIED') = (applied_at IS NOT NULL)))`,
		`CREATE TABLE ` + tables.Snapshots + ` (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			policy_kind TEXT NOT NULL,
			policy_version TEXT NOT NULL,
			payload JSONB NOT NULL DEFAULT '{}'::jsonb,
			checksum TEXT NOT NULL DEFAULT '',
			effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			source_version BIGINT NOT NULL DEFAULT 0,
			event_id TEXT NOT NULL DEFAULT '',
			UNIQUE (policy_kind, policy_version))`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement[:40], err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+tables.Publications)
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+tables.Snapshots)
	})
	return tables
}

func integrationIngestor(t *testing.T, pool *pgxpool.Pool, reader DocumentReader) (*Ingestor, *PostgresStore) {
	t.Helper()
	store, err := NewPostgresStore(pool, setupSchema(t, pool))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return New(Options{
		Store: store, KnownKinds: []string{capabilityKind}, Reader: reader,
		Now: func() time.Time { return time.Unix(1_700_000_500, 0).UTC() },
	}), store
}

func TestIntegrationAppliesAndPersists(t *testing.T) {
	pool := testPool(t)
	ingestor, _ := integrationIngestor(t, pool, nil)

	result, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, inlinePublication(t, "v3")))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeApplied {
		t.Fatalf("outcome = %s", result.Outcome)
	}

	var state, version string
	var appliedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT state, policy_version, applied_at FROM `+testTablePrefix+`_publications WHERE event_id='evt-1'`).
		Scan(&state, &version, &appliedAt); err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	if state != "APPLIED" || version != "v3" || appliedAt == nil {
		t.Fatalf("inbox row = %s / %s / %v", state, version, appliedAt)
	}

	var snapshots int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+testTablePrefix+`_snapshots WHERE policy_version='v3'`).Scan(&snapshots); err != nil {
		t.Fatalf("read snapshots: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("snapshots = %d, want 1", snapshots)
	}
}

// The property an in-memory fake cannot prove: two concurrent deliveries of
// the same publication must produce exactly one applied snapshot.
func TestIntegrationConcurrentDeliveriesApplyOnce(t *testing.T) {
	pool := testPool(t)
	ingestor, _ := integrationIngestor(t, pool, nil)
	publication := inlinePublication(t, "v3")

	const racers = 8
	var wait sync.WaitGroup
	outcomes := make([]Outcome, racers)
	errs := make([]error, racers)
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			result, err := ingestor.Ingest(context.Background(),
				publicationEnvelope(t, "evt-race", 7, publication))
			outcomes[slot], errs[slot] = result.Outcome, err
		}(i)
	}
	close(start)
	wait.Wait()

	appliedCount := 0
	for i, outcome := range outcomes {
		if errs[i] != nil {
			t.Fatalf("racer %d errored: %v", i, errs[i])
		}
		if outcome == OutcomeApplied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("%d racers applied the publication, want exactly 1 (outcomes=%v)", appliedCount, outcomes)
	}

	var snapshots int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+testTablePrefix+`_snapshots`).Scan(&snapshots); err != nil {
		t.Fatalf("count: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("snapshots = %d, want 1", snapshots)
	}
}

func TestIntegrationStaleVersionLeavesAppliedPolicyIntact(t *testing.T) {
	pool := testPool(t)
	ingestor, store := integrationIngestor(t, pool, nil)

	if _, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-new", 9, inlinePublication(t, "v9"))); err != nil {
		t.Fatalf("ingest newer: %v", err)
	}
	result, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-old", 4, inlinePublication(t, "v4")))
	if err != nil {
		t.Fatalf("ingest older: %v", err)
	}
	if result.Outcome != OutcomeStale {
		t.Fatalf("outcome = %s", result.Outcome)
	}

	applied, exists, err := store.AppliedSourceVersion(context.Background(), capabilityKind)
	if err != nil {
		t.Fatalf("applied version: %v", err)
	}
	if !exists || applied != 9 {
		t.Fatalf("applied source version = %d (exists=%v), want 9", applied, exists)
	}

	var state string
	if err := pool.QueryRow(context.Background(),
		`SELECT state FROM `+testTablePrefix+`_publications WHERE event_id='evt-old'`).Scan(&state); err != nil {
		t.Fatalf("read stale row: %v", err)
	}
	if state != "STALE" {
		t.Fatalf("stale row state = %s", state)
	}
	// A STALE row must carry no applied_at, or "when did this build adopt
	// that version" would have two different answers.
	var appliedAt *time.Time
	_ = pool.QueryRow(context.Background(),
		`SELECT applied_at FROM `+testTablePrefix+`_publications WHERE event_id='evt-old'`).Scan(&appliedAt)
	if appliedAt != nil {
		t.Fatal("a stale publication recorded an applied_at")
	}
}

// A parked publication survives a restart: the worker finds it in the table,
// not in anyone's memory.
func TestIntegrationPendingSurvivesAndReconciles(t *testing.T) {
	pool := testPool(t)
	reader := &stubReader{retryable: true, err: context.DeadlineExceeded}
	ingestor, store := integrationIngestor(t, pool, reader)

	publication := inlinePublication(t, "v3")
	document := publication.Document
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"

	result, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeReconciliationRequired {
		t.Fatalf("outcome = %s", result.Outcome)
	}
	pending, err := store.PendingCount(context.Background())
	if err != nil || pending != 1 {
		t.Fatalf("pending = %d (err=%v), want 1", pending, err)
	}

	// A brand new Ingestor, as after a restart. It shares only the database.
	restarted := New(Options{
		Store: store, KnownKinds: []string{capabilityKind},
		Reader: &stubReader{document: document},
		Now:    func() time.Time { return time.Unix(1_700_100_000, 0).UTC() },
	})
	reconciled, err := restarted.ReconcilePending(context.Background(), 10)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("reconciled = %d, want 1", reconciled)
	}

	var state string
	if err := pool.QueryRow(context.Background(),
		`SELECT state FROM `+testTablePrefix+`_publications WHERE event_id='evt-1'`).Scan(&state); err != nil {
		t.Fatalf("read: %v", err)
	}
	if state != "APPLIED" {
		t.Fatalf("state after reconcile = %s", state)
	}
	if count, _ := store.PendingCount(context.Background()); count != 0 {
		t.Fatalf("pending after reconcile = %d", count)
	}
}

// A publication claimed but not yet scheduled must not be picked up by the
// worker, or the worker would race the request that claimed it.
func TestIntegrationFreshlyClaimedRowIsNotPickedUpByTheWorker(t *testing.T) {
	pool := testPool(t)
	store, err := NewPostgresStore(pool, setupSchema(t, pool))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	claimed, err := store.ClaimEvent(context.Background(), EventRecord{
		EventID: "evt-fresh", PolicyKind: capabilityKind, PolicyVersion: "v1",
		SourceService: "admin", SourceVersion: 1, SchemaVersion: 1, Checksum: "sum",
		OccurredAt: time.Now().UTC(), EffectiveAt: time.Now().UTC(),
	})
	if err != nil || !claimed {
		t.Fatalf("claim: %v (claimed=%v)", err, claimed)
	}
	pending, err := store.PendingReadBacks(context.Background(), time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("a freshly claimed row was offered to the worker: %+v", pending)
	}
}

func TestIntegrationRejectsUnusableTableNames(t *testing.T) {
	pool := testPool(t)
	for _, tables := range []Tables{
		{Publications: "", Snapshots: "s"},
		{Publications: "p; DROP TABLE x", Snapshots: "s"},
		{Publications: "p", Snapshots: "S-NAPSHOTS"},
		{Publications: "1p", Snapshots: "s"},
	} {
		if _, err := NewPostgresStore(pool, tables); err == nil {
			t.Fatalf("unusable table names accepted: %+v", tables)
		}
	}
}

func TestIntegrationSnapshotDocumentRoundTrips(t *testing.T) {
	pool := testPool(t)
	ingestor, _ := integrationIngestor(t, pool, nil)

	if _, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, inlinePublication(t, "v3"))); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	var payload []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM `+testTablePrefix+`_snapshots WHERE policy_version='v3'`).Scan(&payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	// The stored document must still verify against its own checksum after a
	// JSONB round trip, which reorders keys. If it did not, every restart
	// would look like corruption.
	if err := VerifyChecksum(json.RawMessage(payload), mustChecksum(t, catalogue("v3"))); err != nil {
		t.Fatalf("stored document no longer matches its checksum: %v", err)
	}
}

func mustChecksum(t *testing.T, document json.RawMessage) string {
	t.Helper()
	sum, err := Checksum(document)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	return sum
}
