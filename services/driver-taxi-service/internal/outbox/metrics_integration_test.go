package outbox

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/outboxmetrics"
)

// The configurations this service compiles in must match the table its own
// migrations create.
//
// This assertion belongs HERE and not in shared/go: shared/go owns no schema,
// so a test placed there can only reach taxi_outbox by borrowing another
// service's database. Run against the migrated schema, a drifted column name
// fails the build instead of producing a live service whose metrics are
// silently always missing.
func TestMetricsSourcesMatchTheMigratedOutbox(t *testing.T) {
	// testDB applies this service's migrations, so the assertion below runs
	// against the schema the service really ships.
	db := testDB(t)
	ctx := context.Background()

	sources := MetricsSources()
	if len(sources) != 2 {
		t.Fatalf("expected one source per destination, got %d", len(sources))
	}
	// Both destinations must be reported separately: a Vendor backlog and a
	// Fleet backlog are different incidents with different responders.
	targets := map[string]bool{}
	for _, source := range sources {
		targets[source.Config.Target] = true
		t.Run(source.Config.Target, func(t *testing.T) {
			if err := source.Config.Valid(); err != nil {
				t.Fatalf("configuration is invalid: %v", err)
			}
			if _, err := outboxmetrics.Collect(ctx, db, source.Config); err != nil {
				t.Fatalf("collect against the migrated outbox: %v", err)
			}
			if _, _, err := outboxmetrics.CollectStates(ctx, db, source.Config, source.Unresolved); err != nil {
				t.Fatalf("collect states against the migrated outbox: %v", err)
			}
		})
	}
	for _, want := range []string{TargetFleet, TargetVendor} {
		if !targets[want] {
			t.Fatalf("no metrics source reports target %q", want)
		}
	}
}
