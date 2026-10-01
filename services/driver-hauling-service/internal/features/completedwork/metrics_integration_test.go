package completedwork_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/services/hauling-service/internal/features/completedwork"
	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// The configuration this service compiles in must match the table its own
// migrations create.
//
// This assertion belongs HERE and not in shared/go: shared/go owns no schema,
// so a test placed there can only reach hauling_domain_outbox by borrowing
// another service's database — which is how it passed locally against a
// populated database and then failed in CI against an empty one. Run against
// the migrated schema, a drifted column name fails the build instead of
// producing a live service whose metrics are silently always missing.
func TestMetricsSourceMatchesTheMigratedOutbox(t *testing.T) {
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for the outbox metrics schema check")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	// ApplyMigrations reads a relative "migrations" directory, so it only
	// resolves from the service root.
	t.Chdir("../../../")
	if err := database.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	source := completedwork.MetricsSource()
	if err := source.Config.Valid(); err != nil {
		t.Fatalf("configuration is invalid: %v", err)
	}
	if source.Config.Target != completedwork.Target {
		t.Fatalf("metrics target %q does not match the outbox target %q", source.Config.Target, completedwork.Target)
	}
	if _, err := outboxmetrics.Collect(ctx, pool, source.Config); err != nil {
		t.Fatalf("collect against the migrated outbox: %v", err)
	}
	if _, _, err := outboxmetrics.CollectStates(ctx, pool, source.Config, source.Unresolved); err != nil {
		t.Fatalf("collect states against the migrated outbox: %v", err)
	}
}
