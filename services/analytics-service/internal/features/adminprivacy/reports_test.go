package adminprivacy

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/events"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresScopedReportsNativeReductionSignedTransportAndRevocation(t *testing.T) {
	cases := []struct{ name, event, owner, authority, product, field, entity, key string }{
		{"mission-outcomes", "taxi.booking.completed", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id", "booking", "completed"},
		{"recovery", "taxi.recovery.started", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id", "booking", "started"},
		{"capability-mismatches", "dispatch.capability_mismatch.v1", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_DELIVERY", "provider_id", "trip", "mismatches"},
		{"declines", "dispatch.offer.declined.v1", "driver-dispatch-delivery-service", "DISPATCH", "DISPATCH_DELIVERY", "provider_id", "trip", "declines"},
		{"provider-quality", "taxi.booking.completed", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id", "booking", "completed"},
		{"demand", "network.demand.observed.v1", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "source_entity_id", "booking", "requests"},
		{"readiness", "vendor.branch.readiness_observed.v1", "vendor-service", "VENDOR", "VENDOR_FOOD", "source_entity_id", "vendor_order", "ready_count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := lineageDB(t)
			ctx := context.Background()
			fixture, actor, q, _ := privacyFixture(t)
			report, _ := analyticsadmin.ReportNamed(tc.name)
			q.Metric = tc.name
			q.Capability = report.Permission
			q.Filters = map[string]string{"authority": tc.authority, "product_code": tc.product}
			actor.Permissions = []string{q.Capability}
			issuer := contextfixture.New(t, tc.owner)
			var source Source = &ReportSource{Report: tc.name, Authorities: map[string]admincontext.Authority{tc.owner: issuer.Authority}, UnitField: tc.field}
			if tc.name == "readiness" {
				source = &ReadinessSource{Authorities: map[string]admincontext.Authority{tc.owner: issuer.Authority}, UnitField: tc.field}
			}
			fixture.Document.Rules[0].Capability = q.Capability
			fixture.Document.Rules[0].Permission = q.Capability
			fixture.Document.Rules[0].SourceContract = report.Contract + ":" + tc.field
			fixture.Document.Analytics.AllowedCellKeys = report.Keys
			fixture.Document.Analytics.AllowedFilterValues["authority"] = []string{tc.authority}
			fixture.Document.Analytics.AllowedFilterValues["product_code"] = []string{tc.product}
			fixture.Sign(t)
			payload, _ := json.Marshal(map[string]any{"authority": tc.authority, "product_code": tc.product, "provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "reason_code": "BUSY", "requests": 1, "unmatched": false, "branch_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "on_time": true, "lateness_seconds": 0, "prep_seconds": 30})
			id := "event-" + tc.name
			entity := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			status, err := usecases.NewService(repositories.NewRepository(db)).Ingest(ctx, tc.owner, events.Envelope{EventID: id, EventName: tc.event, SchemaVersion: 1, SourceService: tc.owner, SourceEntityType: tc.entity, SourceEntityID: entity, SourceVersion: 1, OccurredAt: q.Start.Add(time.Minute), Payload: payload})
			if err != nil || status != "PROCESSED" {
				t.Fatal(status, err)
			}
			reader := LineageReader{Owner: tc.owner}
			store := admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
			kind, _ := LineageKind(tc.name, tc.field)
			tx, _ := db.Begin(ctx)
			binding, err := reader.Binding(ctx, tx, kind, id)
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			approval := issuer.Approval("allocation-"+id, kind, id, "analytics.cohorts."+tc.name+".export", binding, q.Scope)
			if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
				t.Fatal(err)
			}
			tx, _ = db.Begin(ctx)
			snapshot, err := source.Snapshot(ctx, tx, q)
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, cell := range snapshot.Cells {
				if cell.Key == tc.key {
					found = cell.Count != nil && *cell.Count == 1 && cell.CohortSize != nil && *cell.CohortSize == 1
				}
			}
			if !found || !snapshot.Partial {
				t.Fatal("native components or partial coverage wrong", snapshot)
			}
			service := &Service{DB: db, Policy: fixture.Engine, Source: source}
			foreign := q
			foreign.Scope.Tenant = "foreign"
			if _, err := service.Disclose(ctx, actor, foreign); err == nil {
				t.Fatal("cross tenant release")
			}
			_, err = db.Exec(ctx, `CREATE FUNCTION fail_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'TEST_ONLY_AUDIT_FAILURE';END$$; CREATE TRIGGER fail_report_audit BEFORE INSERT ON analytics_admin_query_audit FOR EACH ROW EXECUTE FUNCTION fail_report_audit()`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Disclose(ctx, actor, q); err == nil {
				t.Fatal("audit failure disclosed")
			}
			db.Exec(ctx, `DROP TRIGGER fail_report_audit ON analytics_admin_query_audit`)
			key := bytes.Repeat([]byte{82}, 32)
			engine := gin.New()
			(&Handler{Enabled: true, Secret: key, Service: service, Reports: map[string]*Service{tc.name: service}}).Register(engine)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorded := httptest.NewRecorder()
				engine.ServeHTTP(recorded, r)
				for key, values := range recorded.Header() {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(recorded.Code)
				w.Write(recorded.Body.Bytes())
				if recorded.Code == 200 {
					if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Error(err)
						}
						if err := os.WriteFile(filepath.Join(dir, "scoped-analytics-owner-"+tc.name+".json"), recorded.Body.Bytes(), 0600); err != nil {
							t.Error(err)
						}
					}
				}
			}))
			defer server.Close()
			client, err := analyticsadmin.NewClient(server.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			request := adminread.Query{Scope: adminread.Scope{Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, Staff: actor.Staff, Role: "staff", Purpose: q.Purpose, PolicyID: fixture.Engine.ID(), PolicyVersion: fixture.Engine.Version(), PolicyChecksum: fixture.Engine.Checksum(), Start: q.Start, End: q.End, Limit: 1}
			readers := map[string]func(context.Context, adminread.Query, analyticsadmin.Filters) (analyticsadmin.Release, error){"mission-outcomes": client.MissionOutcomes, "recovery": client.Recovery, "capability-mismatches": client.CapabilityMismatches, "declines": client.Declines, "provider-quality": client.ProviderQuality, "demand": client.Demand, "readiness": client.Readiness}
			out, err := readers[tc.name](ctx, request, analyticsadmin.Filters{Authority: tc.authority, ProductCode: tc.product})
			if err != nil {
				t.Fatal(err)
			}
			if out.Metric != tc.name || out.Coverage != "PARTIAL_COVERAGE" || len(out.Cells) != len(report.Keys) {
				t.Fatal(out)
			}
			for _, cell := range out.Cells {
				if cell.Value != nil {
					t.Fatal("partial value exposed")
				}
			}
			approval.Revision++
			approval.State = "REVOKED"
			if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Disclose(ctx, actor, q); err == nil {
				t.Fatal("revoked member released")
			}
		})
	}
}
func TestScopedReportsDoNotManufactureSupplyMembershipOrForecastModel(t *testing.T) {
	for _, name := range []string{"supply", "demand-forecast"} {
		report, _ := analyticsadmin.ReportNamed(name)
		q := Query{Metric: name, Capability: report.Permission}
		if _, err := (&ReportSource{Report: name}).Snapshot(context.Background(), nil, q); !errors.Is(err, ErrUnmapped) {
			t.Fatal(name, err)
		}
	}
}
