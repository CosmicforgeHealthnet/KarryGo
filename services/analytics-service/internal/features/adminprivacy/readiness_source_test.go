package adminprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func TestPostgresReadinessComponentsUseNativeOrdersAndWithholdPartialMembership(t *testing.T) {
	for _, unit := range []string{"branch_id", "source_entity_id"} {
		t.Run(unit, func(t *testing.T) {
			db := lineageDB(t)
			ctx := context.Background()
			policy, actor, q, _ := privacyFixture(t)
			q.Metric = "readiness"
			q.Capability = readinessComponentsPermission
			q.Filters = map[string]string{"authority": networkanalytics.AuthorityVendor, "product_code": "UNSPECIFIED"}
			issuer := contextfixture.New(t, "vendor-service")
			source := &ReadinessSource{Authorities: map[string]admincontext.Authority{"vendor-service": issuer.Authority}, UnitField: unit}
			policy.Document.Rules[0].Capability = q.Capability
			policy.Document.Rules[0].Permission = q.Capability
			policy.Document.Rules[0].SourceContract = source.SourceContract()
			policy.Document.Analytics.AllowedCellKeys = append([]string{}, readinessComponentKeys...)
			policy.Document.Analytics.AllowedFilterValues["authority"] = []string{networkanalytics.AuthorityVendor}
			policy.Document.Analytics.AllowedFilterValues["product_code"] = []string{"UNSPECIFIED"}
			actor.Permissions = []string{q.Capability}
			policy.Sign(t)
			native := usecases.NewService(repositories.NewRepository(db))
			reader := LineageReader{Owner: "vendor-service"}
			store := admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
			kind, _ := LineageKind("readiness", unit)
			var revoke admincontext.Approval
			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("native-ready-%d", i)
				payload, _ := json.Marshal(map[string]any{"branch_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "business_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "on_time": i == 0, "lateness_seconds": int64(i * 10), "prep_seconds": int64((i + 1) * 30)})
				if _, err := native.Ingest(ctx, "vendor-service", events.Envelope{EventID: id, EventName: networkanalytics.EventBranchReadiness, SchemaVersion: 1, SourceService: "vendor-service", SourceEntityType: "vendor_order", SourceEntityID: fmt.Sprintf("cccccccc-cccc-4ccc-8ccc-%012d", i+1), SourceVersion: 1, OccurredAt: q.Start.Add(time.Minute), Payload: payload}); err != nil {
					t.Fatal(err)
				}
				tx, err := db.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				binding, err := reader.Binding(ctx, tx, kind, id)
				tx.Rollback(ctx)
				if err != nil {
					t.Fatal(err)
				}
				a := issuer.Approval("TEST_ONLY_READINESS_"+id, kind, id, "analytics.cohorts.readiness.export", binding, q.Scope)
				if err := store.Import(ctx, issuer.Sign(t, a), time.Now()); err != nil {
					t.Fatal(err)
				}
				revoke = a
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := source.Snapshot(ctx, tx, q)
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !snap.Partial || snap.Facts.Scope != q.Scope || snap.Facts.SourceContract != source.SourceContract() {
				t.Fatal("invented complete coverage or scope", snap)
			}
			expected := map[string]int64{"ready_count": 3, "on_time_count": 1, "late_count": 2, "lateness_seconds_total": 30, "prep_seconds_total": 180, "prep_count": 3}
			for _, cell := range snap.Cells {
				if cell.Count == nil || *cell.Count != expected[cell.Key] {
					t.Fatal("native readiness component lost", cell)
				}
				if cell.Key == "ready_count" {
					want := int64(3)
					if unit == "branch_id" {
						want = 1
					}
					if cell.CohortSize == nil || *cell.CohortSize != want {
						t.Fatal("event count inflated privacy unit", cell)
					}
				}
			}
			service := Service{DB: db, Policy: policy.Engine, Source: source}
			release, err := service.Disclose(ctx, actor, q)
			if err != nil {
				t.Fatal(err)
			}
			if release.Coverage != "PARTIAL_COVERAGE" {
				t.Fatal("subset claimed complete")
			}
			for _, cell := range release.Cells {
				if cell.Value != nil {
					t.Fatal("incomplete readiness released value")
				}
			}
			// Revoking one original member invalidates a frozen release even while the
			// other two approved events keep the general requested source scope valid.
			revoke.Revision++
			revoke.State = "REVOKED"
			if err := store.Import(ctx, issuer.Sign(t, revoke), time.Now()); err != nil {
				t.Fatal(err)
			}
			tx, err = db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := source.Snapshot(ctx, tx, q); err != nil {
				t.Fatal("remaining native scope disappeared", err)
			}
			if err := source.AuthorizeRelease(ctx, tx, q, snap.SourceVersion, snap.Authorization); err == nil {
				t.Fatal("frozen membership followed a new sample")
			}
			tx.Rollback(ctx)
			if _, err := service.Disclose(ctx, actor, q); err == nil {
				t.Fatal("cached readiness outlived original-member revocation")
			}
			// Neither a driver authority nor an arbitrary candidate unit is an alias.
			wrong := q
			wrong.Filters = map[string]string{"authority": "TAXI", "product_code": "UNSPECIFIED"}
			tx, err = db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := source.Snapshot(ctx, tx, wrong); !errors.Is(err, ErrUnmapped) {
				t.Fatal(err)
			}
			invalid := *source
			invalid.UnitField = "provider_id"
			if _, err := invalid.Snapshot(ctx, tx, q); !errors.Is(err, ErrUnmapped) {
				t.Fatal(err)
			}
		})
	}
}

func TestReadinessComponentsNeverInventMissingTimingFacts(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"on_time":false,"lateness_seconds":0}`, `{"on_time":"false","lateness_seconds":0,"prep_seconds":0}`,
		`{"on_time":false,"lateness_seconds":-1,"prep_seconds":1}`, `{"on_time":false,"lateness_seconds":1.5,"prep_seconds":1}`,
		`{"on_time":false,"lateness_seconds":1,"prep_seconds":9223372036854775808}`, `{"on_time":true,"lateness_seconds":3,"prep_seconds":1}`,
	} {
		if _, err := readinessComponents([]byte(raw)); !errors.Is(err, ErrSource) {
			t.Fatal("invalid timing converted to count", raw, err)
		}
	}
	cells, err := readinessComponents([]byte(`{"on_time":true,"lateness_seconds":0,"prep_seconds":0}`))
	if err != nil || cells["on_time_count"] != 1 || cells["late_count"] != 0 || cells["prep_count"] != 0 {
		t.Fatal("explicit zero/true facts lost", cells, err)
	}
}
