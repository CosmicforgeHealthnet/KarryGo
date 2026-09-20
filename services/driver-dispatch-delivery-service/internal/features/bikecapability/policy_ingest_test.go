package bikecapability

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/policyingest"
)

// These cover the service-level behaviours the shared package cannot: that the
// snapshot ingestion writes is the one MATCHING reads, that an Admin outage
// leaves matching on its last good snapshot, and that a booking committed
// under one version keeps pointing at that version afterwards.
func policyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("DISPATCH_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// A clean slate, so a leftover snapshot from another test does not decide
	// which version "latest" means.
	for _, statement := range []string{
		`DELETE FROM dispatch_policy_publications`,
		`DELETE FROM dispatch_policy_snapshots`,
	} {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	return pool
}

func catalogueDocument(version string) json.RawMessage {
	return json.RawMessage(`{"version":"` + version +
		`","products":[{"code":"DISPATCH_STANDARD","certification_required":false}]}`)
}

func publish(t *testing.T, eventID, version string, sourceVersion int64) events.Envelope {
	t.Helper()
	document := catalogueDocument(version)
	checksum, err := policyingest.Checksum(document)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	payload, err := json.Marshal(policyingest.Publication{
		PolicyKind: PolicyKindCapability, PolicyVersion: version, Checksum: checksum,
		EffectiveAt: time.Now().UTC().Add(-time.Minute), Document: document,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return events.Envelope{
		EventID: eventID, EventName: policyingest.EventName,
		SchemaVersion: policyingest.SchemaVersion, SourceService: "admin-backoffice-service",
		SourceEntityType: "network_catalogue", SourceEntityID: "catalogue",
		SourceVersion: sourceVersion, OccurredAt: time.Now().UTC(), Payload: payload,
	}
}

func ingestFor(t *testing.T, pool *pgxpool.Pool) *PolicyIngest {
	t.Helper()
	ingest := NewPolicyIngest(PolicyIngestOptions{DB: pool})
	if ingest == nil {
		t.Fatal("policy ingest was not constructed")
	}
	return ingest
}

// The whole point of ingestion: what lands becomes the version matching pins.
func TestIngestedVersionBecomesTheOneMatchingReads(t *testing.T) {
	pool := policyPool(t)
	service := New(Options{Repository: NewRepository(pool)})

	// Before anything is published, matching runs on the built-in default. A
	// deployment that has never reached Admin still matches.
	if before := service.CapabilityPolicyVersion(context.Background()); before != DefaultCapabilityPolicyVersion {
		t.Fatalf("version before ingestion = %s, want the built-in default", before)
	}

	result, err := ingestFor(t, pool).Ingestor().Ingest(context.Background(), publish(t, "evt-1", "dispatch_capability_v7", 7))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != policyingest.OutcomeApplied {
		t.Fatalf("outcome = %s", result.Outcome)
	}

	if after := service.CapabilityPolicyVersion(context.Background()); after != "dispatch_capability_v7" {
		t.Fatalf("version after ingestion = %s, want the published one", after)
	}
}

// Matching must keep working against the last good snapshot when the control
// plane is unreachable. It is the reason policy is pulled into a local table
// at all rather than read through on every assignment.
func TestMatchingKeepsUsingTheLocalSnapshotDuringAnAdminOutage(t *testing.T) {
	pool := policyPool(t)
	service := New(Options{Repository: NewRepository(pool)})
	ingest := ingestFor(t, pool)

	if _, err := ingest.Ingestor().Ingest(context.Background(), publish(t, "evt-1", "dispatch_capability_v7", 7)); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Admin now goes away entirely: a referenced publication that cannot be
	// read back. No reader is configured, so it parks.
	document := catalogueDocument("dispatch_capability_v8")
	checksum, _ := policyingest.Checksum(document)
	payload, _ := json.Marshal(policyingest.Publication{
		PolicyKind: PolicyKindCapability, PolicyVersion: "dispatch_capability_v8",
		Checksum: checksum, EffectiveAt: time.Now().UTC(),
		DocumentRef: "https://admin.internal/catalogue/v8",
	})
	outage := publish(t, "evt-2", "dispatch_capability_v8", 8)
	outage.Payload = payload

	result, err := ingest.Ingestor().Ingest(context.Background(), outage)
	if err != nil {
		t.Fatalf("ingest during outage: %v", err)
	}
	if result.Outcome != policyingest.OutcomeReconciliationRequired {
		t.Fatalf("outcome = %s, want RECONCILIATION_REQUIRED", result.Outcome)
	}

	// Matching is untouched: still the last version that actually landed.
	if current := service.CapabilityPolicyVersion(context.Background()); current != "dispatch_capability_v7" {
		t.Fatalf("version during outage = %s, want the last good snapshot", current)
	}
}

// A booking committed under one policy version must keep pointing at that
// version after a newer one lands, or the rules a past decision was made under
// become unknowable.
func TestCommittedWorkStaysPinnedToItsOriginalPolicyVersion(t *testing.T) {
	pool := policyPool(t)
	service := New(Options{Repository: NewRepository(pool)})
	ingest := ingestFor(t, pool)
	ctx := context.Background()

	if _, err := ingest.Ingestor().Ingest(ctx, publish(t, "evt-1", "dispatch_capability_v7", 7)); err != nil {
		t.Fatalf("ingest v7: %v", err)
	}
	pinned := service.CapabilityPolicyVersion(ctx)

	// A match decision is recorded under v7, exactly as the matcher records
	// one.
	if _, err := pool.Exec(ctx, `
INSERT INTO dispatch_match_decisions (mission_id, provider_id, decision, reason_codes, policy_version)
VALUES ('mission-pinned', gen_random_uuid(), 'ELIGIBLE', 'ELIGIBLE', $1)`, pinned); err != nil {
		t.Fatalf("record decision: %v", err)
	}

	// The network moves on.
	if _, err := ingest.Ingestor().Ingest(ctx, publish(t, "evt-2", "dispatch_capability_v8", 8)); err != nil {
		t.Fatalf("ingest v8: %v", err)
	}
	if current := service.CapabilityPolicyVersion(ctx); current != "dispatch_capability_v8" {
		t.Fatalf("current version = %s, want v8", current)
	}

	var recorded string
	if err := pool.QueryRow(ctx,
		`SELECT policy_version FROM dispatch_match_decisions ORDER BY created_at DESC LIMIT 1`).
		Scan(&recorded); err != nil {
		t.Fatalf("read decision: %v", err)
	}
	if recorded != pinned {
		t.Fatalf("committed evidence moved from %s to %s", pinned, recorded)
	}

	// And the old snapshot is still there, unmodified.
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM dispatch_policy_snapshots WHERE policy_version=$1`, pinned).Scan(&payload); err != nil {
		t.Fatalf("old snapshot is gone: %v", err)
	}
	if err := policyingest.VerifyChecksum(payload, mustSum(t, catalogueDocument("dispatch_capability_v7"))); err != nil {
		t.Fatalf("old snapshot changed: %v", err)
	}
}

// The immutability trigger is the last line of defence: even a direct UPDATE
// cannot rewrite a snapshot committed work is pinned to.
func TestSnapshotRewriteIsRefusedByTheDatabase(t *testing.T) {
	pool := policyPool(t)
	if _, err := ingestFor(t, pool).Ingestor().Ingest(context.Background(),
		publish(t, "evt-1", "dispatch_capability_v7", 7)); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	_, err := pool.Exec(context.Background(),
		`UPDATE dispatch_policy_snapshots SET payload='{"tampered":true}'::jsonb WHERE policy_version=$1`,
		"dispatch_capability_v7")
	if err == nil {
		t.Fatal("a committed snapshot was rewritten")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The publication carries no pricing, and a document that does is refused
// before it can reach an immutable snapshot.
func TestPricingBearingCatalogueIsRefused(t *testing.T) {
	pool := policyPool(t)
	document := json.RawMessage(`{"products":[{"code":"DISPATCH_STANDARD","per_km":250}]}`)
	checksum, _ := policyingest.Checksum(document)
	payload, _ := json.Marshal(policyingest.Publication{
		PolicyKind: PolicyKindCapability, PolicyVersion: "dispatch_capability_v9",
		Checksum: checksum, EffectiveAt: time.Now().UTC(), Document: document,
	})
	envelope := publish(t, "evt-price", "dispatch_capability_v9", 9)
	envelope.Payload = payload

	result, err := ingestFor(t, pool).Ingestor().Ingest(context.Background(), envelope)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != policyingest.OutcomeRejected {
		t.Fatalf("outcome = %s, want REJECTED", result.Outcome)
	}
	var snapshots int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_policy_snapshots`).Scan(&snapshots)
	if snapshots != 0 {
		t.Fatal("a pricing-bearing catalogue reached the snapshot table")
	}
}

// A catalogue for another authority is ignored without consuming this
// service's event id.
func TestForeignCatalogueIsIgnored(t *testing.T) {
	pool := policyPool(t)
	document := catalogueDocument("foreign_v1")
	checksum, _ := policyingest.Checksum(document)
	payload, _ := json.Marshal(policyingest.Publication{
		PolicyKind: "HAULING_CAPABILITY", PolicyVersion: "hauling_v1",
		Checksum: checksum, EffectiveAt: time.Now().UTC(), Document: document,
	})
	envelope := publish(t, "evt-foreign", "foreign_v1", 1)
	envelope.Payload = payload

	result, err := ingestFor(t, pool).Ingestor().Ingest(context.Background(), envelope)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != policyingest.OutcomeIgnored {
		t.Fatalf("outcome = %s, want IGNORED", result.Outcome)
	}
	var claimed int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_policy_publications WHERE event_id='evt-foreign'`).Scan(&claimed)
	if claimed != 0 {
		t.Fatal("a foreign catalogue consumed this service's event id")
	}
}

func mustSum(t *testing.T, document json.RawMessage) string {
	t.Helper()
	sum, err := policyingest.Checksum(document)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	return sum
}
