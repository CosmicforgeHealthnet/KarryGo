package adminprivacy

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"testing"
	"time"
)

func number(n int64) *int64 { return &n }
func privacyFixture(t *testing.T) (*testfixture.Fixture, adminpolicy.Actor, Query, Snapshot) {
	t.Helper()
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}
	f := testfixture.New(t, scope, "analytics-service", "analytics.network.read", "analytics.scoped-cohort.v1")
	// These are deliberately test-only policy values, signed by an ephemeral
	// test key. Production Verify rejects this entire policy environment.
	f.Document.Analytics = &adminpolicy.Disclosure{PrivacyDomain: "TEST_ONLY_NETWORK_CUBE", MaxLookbackSeconds: 31 * 86400, MinimumCohort: 5, RoundingMultiple: 10, MaxQueriesPerStaffWindow: 100, QueryWindowSeconds: 60, ReleaseBucketSeconds: 3600, FreshnessSeconds: 7200, AllowedCellKeys: []string{"completed", "failed"}, FilterSets: [][]string{{"authority", "product_code"}}, AllowedFilterValues: map[string][]string{"authority": {"TAXI"}, "product_code": {"TAXI_RIDE", "TAXI_GOODS"}}}
	f.Sign(t)
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{"analytics.network.read"}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"O1"}, Tenants: []string{"O1"}}
	start := time.Now().UTC().Truncate(24 * time.Hour).Add(-48 * time.Hour)
	q := Query{Capability: "analytics.network.read", Metric: "mission-success", Scope: scope, Purpose: "case-1", Start: start, End: start.Add(time.Hour), Filters: map[string]string{"authority": "TAXI", "product_code": "TAXI_RIDE"}}
	snapshot := Snapshot{Facts: adminpolicy.Facts{Owner: "analytics-service", Reference: "SECRET_SOURCE_REFERENCE", SourceContract: "analytics.scoped-cohort.v1", Scope: scope}, SourceVersion: "test-source-v1", AsOf: time.Now().UTC().Add(-time.Minute), Cells: []Cell{{Key: "completed", Count: number(23), CohortSize: number(50)}, {Key: "failed", Count: number(17), CohortSize: number(50)}}}
	return f, actor, q, snapshot
}
func TestSmallCellComplementarySuppressionRoundingAndUnknowns(t *testing.T) {
	for _, name := range []string{"rounded", "small", "rare_count", "unknown_count", "unknown_cohort", "known_zero", "stale", "partial", "unapproved_label", "missing_cell"} {
		t.Run(name, func(t *testing.T) {
			f, _, q, s := privacyFixture(t)
			p, _ := f.Engine.Disclosure(time.Now())
			switch name {
			case "rare_count":
				s.Cells[0].Count = number(1)
			case "small":
				s.Cells[0].CohortSize = number(2)
			case "unknown_count":
				s.Cells[0].Count = nil
			case "unknown_cohort":
				s.Cells[0].CohortSize = nil
			case "known_zero":
				s.Cells[0].Count = number(0)
			case "stale":
				s.AsOf = time.Now().Add(-4 * time.Hour)
			case "partial":
				s.Partial = true
			case "unapproved_label":
				s.Cells[0].Key = "SECRET_PHONE"
			case "missing_cell":
				s.Cells = s.Cells[:1]
			}
			out, err := suppress(q, s, p, f.Engine, time.Now())
			if name == "unapproved_label" || name == "missing_cell" {
				if err == nil {
					t.Fatal("incomplete/unapproved source shape disclosed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "rounded" {
				if *out.Cells[0].Value != 20 || *out.Cells[1].Value != 10 {
					t.Fatalf("rounding=%+v", out.Cells)
				}
				return
			}
			if name == "known_zero" {
				if out.Cells[0].Value == nil || *out.Cells[0].Value != 0 {
					t.Fatal("verified zero became unknown")
				}
				return
			}
			for _, cell := range out.Cells {
				if cell.Value != nil {
					t.Fatalf("suppressed value escaped: %+v", out)
				}
			}
			if name == "small" && (out.Cells[0].Suppression != "SMALL_CELL" || out.Cells[1].Suppression != "COMPLEMENTARY") {
				t.Fatalf("complement leaked: %+v", out)
			}
			if name == "stale" && out.Freshness != "STALE" {
				t.Fatal("stale source reported live")
			}
		})
	}
}
func TestRestrictedFilterCombinationsAndNonoverlappingBounds(t *testing.T) {
	for _, name := range []string{"approved", "extra_subject_filter", "missing_dimension", "unapproved_value", "sliding_window", "future_window"} {
		t.Run(name, func(t *testing.T) {
			f, _, q, _ := privacyFixture(t)
			p, _ := f.Engine.Disclosure(time.Now())
			switch name {
			case "extra_subject_filter":
				q.Filters["customer_id"] = "SECRET"
			case "missing_dimension":
				delete(q.Filters, "authority")
			case "unapproved_value":
				q.Filters["product_code"] = "rare-secret-product"
			case "sliding_window":
				q.Start = q.Start.Add(time.Minute)
				q.End = q.End.Add(time.Minute)
			case "future_window":
				q.Start = time.Now().UTC().Add(time.Hour)
				q.End = q.Start.Add(time.Hour)
			}
			if err := validateQuery(q, p, time.Now()); (err == nil) != (name == "approved") {
				t.Fatalf("query %s: %v", name, err)
			}
		})
	}
}
