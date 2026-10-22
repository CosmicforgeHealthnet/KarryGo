package adminprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/events"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lineageDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := privacyDB(t)
	for _, name := range []string{"003_network_analytics.sql", "004_network_supply_demand_outcomes.sql", "006_admin_cohort_lineage_contexts.sql", "010_admin_lineage_preparation_audit.sql", "011_admin_coverage_manifests.sql"} {
		raw, err := os.ReadFile(filepath.Join("../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "006_admin_cohort_lineage_contexts.sql" && !strings.Contains(string(raw), strings.ReplaceAll(admincontext.Schema, "\r\n", "\n")) {
			t.Fatal("normal lineage migration drifted from source context schema")
		}
		if _, err := db.Exec(context.Background(), string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func ingestLineage(t *testing.T, db *pgxpool.Pool, id, name, owner, entityType, entityID string, version int64, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	status, err := usecases.NewService(repositories.NewRepository(db)).Ingest(context.Background(), owner, events.Envelope{EventID: id, EventName: name, SchemaVersion: 1, SourceService: owner, SourceEntityType: entityType, SourceEntityID: entityID, SourceVersion: version, OccurredAt: time.Now().UTC().Add(-time.Minute), Payload: raw})
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestPostgresAnalyticsLineageUsesCanonicalEventsAndIndependentOwnerContexts(t *testing.T) {
	db := lineageDB(t)
	ctx := context.Background()
	owner := "driver-dispatch-delivery-service"
	reader := LineageReader{Owner: owner}
	for _, id := range []string{"completed-1", "completed-2"} {
		if status := ingestLineage(t, db, id, "trip.completed.v1", owner, "trip", id, 1, map[string]any{"provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "original_provider_id": "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "market_id": "operational-market", "zone": "gh5:operational", "recipient_name": "PRIVATE_RECIPIENT"}); status != models.StatusProcessed {
			t.Fatal(status)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := reader.ReadUnit(ctx, tx, "mission-success", "provider_id", "completed-1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := reader.ReadUnit(ctx, tx, "provider-quality", "original_provider_id", "completed-2")
	if err != nil {
		t.Fatal(err)
	}
	if first.SubjectHash != again.SubjectHash || first.UnitFamily != "PROVIDER" || first.SubjectHash == "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatal("native provider duplicated across reports or role fields")
	}
	kind, err := LineageKind("mission-success", "provider_id")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := reader.Binding(ctx, tx, kind, "completed-1")
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Canonical != (adminpolicy.Scope{Product: "karryam"}) {
		t.Fatal("operational geography/tenant became authorization")
	}
	issuer := contextfixture.New(t, owner)
	store := admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	capability := "analytics.cohorts.mission-success.export"
	approval := issuer.Approval("lineage-owner-context", kind, "completed-1", capability, binding, scope)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := store.Resolve(ctx, tx, kind, "completed-1", capability, scope, time.Now())
	if err != nil || facts.Owner != owner {
		t.Fatal(err, facts)
	}
	foreign := scope
	foreign.Tenant = "foreign-tenant"
	if _, err := store.Resolve(ctx, tx, kind, "completed-1", capability, foreign, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("foreign allocation accepted: %v", err)
	}
	if _, err := (LineageReader{Owner: "driver-hauling-service"}).ReadUnit(ctx, tx, "mission-success", "provider_id", "completed-1"); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("foreign source owner accepted: %v", err)
	}
	if _, err := reader.ReadUnit(ctx, tx, "declines", "provider_id", "completed-1"); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("foreign report kind accepted: %v", err)
	}
	expiredTx, expiredErr := db.Begin(ctx)
	if expiredErr != nil {
		t.Fatal(expiredErr)
	}
	if _, expiredErr = store.Resolve(ctx, expiredTx, kind, "completed-1", capability, scope, approval.ExpiresAt.Add(time.Second)); !errors.Is(expiredErr, admincontext.ErrUnmapped) {
		t.Fatalf("expired owner lineage context accepted: %v", expiredErr)
	}
	expiredTx.Rollback(ctx)
	tx.Rollback(ctx)
	revoked := approval
	revoked.Revision = 2
	revoked.State = "REVOKED"
	if err := store.Import(ctx, issuer.Sign(t, revoked), time.Now()); err != nil {
		t.Fatal(err)
	}
	revokedTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Resolve(ctx, revokedTx, kind, "completed-1", capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("revoked owner lineage context accepted: %v", err)
	}
	revokedTx.Rollback(ctx)
	// A changed payload cannot retain a valid binding by leaving its old digest.
	if _, err := db.Exec(ctx, `UPDATE analytics_network_events SET payload=jsonb_set(payload,'{provider_id}','"provider-foreign"') WHERE event_id='completed-1'`); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := reader.ReadUnit(ctx, tx, "mission-success", "provider_id", "completed-1"); !errors.Is(err, ErrSource) {
		t.Fatalf("tampered native projection accepted: %v", err)
	}
}

func TestPostgresAnalyticsLineageNeverCountsAggregateBucketsAsSubjects(t *testing.T) {
	db := lineageDB(t)
	ctx := context.Background()
	owner := "driver-dispatch-delivery-service"
	for _, event := range []struct {
		id, name, entity string
		payload          map[string]any
	}{
		{"supply-1", "network.supply.snapshot.v1", "network_supply", map[string]any{"authority": "DISPATCH", "zone": "gh5:operational", "online_count": 100, "eligible_count": 90, "occupied_count": 10, "provider_id": "incidental-provider", "state": "AVAILABLE"}},
		{"demand-1", "network.demand.bucket.v1", "network_demand", map[string]any{"authority": "DISPATCH", "zone": "gh5:operational", "demand_count": 100, "provider_id": "incidental-provider"}},
	} {
		if status := ingestLineage(t, db, event.id, event.name, owner, event.entity, event.id, 1, event.payload); status != models.StatusProcessed {
			t.Fatal(status)
		}
		for _, report := range []string{"supply", "demand", "demand-forecast"} {
			for _, field := range []string{"provider_id", "source_entity_id"} {
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				_, err = (LineageReader{Owner: owner}).ReadUnit(ctx, tx, report, field, event.id)
				tx.Rollback(ctx)
				if !errors.Is(err, ErrUnmapped) {
					t.Fatalf("%s %s treated a native aggregate as an individual cohort: %v", report, field, err)
				}
			}
		}
	}
	// Older versions are retained by the native store but never projected twice.
	ingestLineage(t, db, "new-version", "trip.completed.v1", owner, "trip", "trip-same", 2, map[string]any{"provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	if status := ingestLineage(t, db, "stale-version", "trip.completed.v1", owner, "trip", "trip-same", 1, map[string]any{"provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}); status != models.StatusStale {
		t.Fatal(status)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := (LineageReader{Owner: owner}).ReadUnit(ctx, tx, "mission-success", "provider_id", "stale-version"); !errors.Is(err, ErrUnmapped) {
		t.Fatalf("stale event acquired lineage: %v", err)
	}
}

func TestAnalyticsLineageDictionaryMatchesAllTenReports(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/admin-metrics-dictionary.json")
	if err != nil {
		t.Fatal(err)
	}
	var dict struct {
		Reports []struct {
			ID string `json:"id"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(raw, &dict); err != nil {
		t.Fatal(err)
	}
	if len(dict.Reports) != 10 || len(lineageReports) != len(dict.Reports) {
		t.Fatal("report coverage drift")
	}
	for _, report := range dict.Reports {
		if _, err := LineageKind(report.ID, "provider_id"); err != nil {
			t.Fatal(report.ID, err)
		}
	}
	for _, input := range [][2]string{{"arbitrary-report", "provider_id"}, {"supply", "count"}, {"mission-success", "tenant_id"}, {"mission-success", "provider_id;DROP TABLE"}} {
		if _, err := LineageKind(input[0], input[1]); !errors.Is(err, ErrUnmapped) {
			t.Fatal("uncompiled lineage selector accepted", input)
		}
	}
}
