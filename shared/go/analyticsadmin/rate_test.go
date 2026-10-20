package analyticsadmin

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"math"
	"strings"
	"testing"
	"time"
)

func TestMissionRateUsesOnlyDisclosedCountsAndNeverGuessesUnknownValues(t *testing.T) {
	value := func(n int64) *int64 { return &n }
	base := Release{PolicyID: "TEST_ONLY_POLICY", PolicyVersion: 1, PolicyChecksum: strings.Repeat("a", 64), Scope: adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, Metric: "mission-success", SourceVersion: "TEST_ONLY_SNAPSHOT", AsOf: time.Now().UTC(), Freshness: "LIVE", Coverage: "COMPLETE", Cells: []Cell{{Key: "completed", Value: value(3), Suppression: "NONE"}, {Key: "failed", Value: value(1), Suppression: "NONE"}, {Key: "cancelled", Value: value(0), Suppression: "NONE"}, {Key: "guarantee_breaches", Value: value(0), Suppression: "NONE"}}}
	for _, name := range []string{"ratio", "partial", "stale", "small cell", "complementary", "zero denominator", "approved rounded components", "overflow", "missing cell"} {
		t.Run(name, func(t *testing.T) {
			in := base
			in.Cells = append([]Cell{}, base.Cells...)
			want := "NONE"
			expected := .75
			invalid := false
			switch name {
			case "partial":
				in.Coverage = "PARTIAL_COVERAGE"
				want = "PARTIAL_COVERAGE"
				for i := range in.Cells {
					in.Cells[i].Value = nil
					in.Cells[i].Suppression = "PARTIAL_COVERAGE"
				}
			case "stale":
				in.Freshness = "STALE"
				want = "STALE"
				for i := range in.Cells {
					in.Cells[i].Value = nil
					in.Cells[i].Suppression = "STALE"
				}
			case "small cell":
				in.Cells[0].Value = nil
				in.Cells[0].Suppression = "SMALL_CELL"
				want = "SOURCE_SUPPRESSED"
			case "complementary":
				in.Cells[1].Value = nil
				in.Cells[1].Suppression = "COMPLEMENTARY"
				want = "SOURCE_SUPPRESSED"
			case "zero denominator":
				in.Cells[0].Value = value(0)
				in.Cells[1].Value = value(0)
				want = "NO_DENOMINATOR"
			case "approved rounded components":
				in.Cells[0].Value = value(10)
				in.Cells[1].Value = value(5)
				expected = 2.0 / 3
			case "overflow":
				in.Cells[0].Value = value(math.MaxInt64)
				in.Cells[1].Value = value(1)
				invalid = true
			case "missing cell":
				in.Cells = in.Cells[:3]
				invalid = true
			}
			out, err := MissionRate(in)
			if invalid {
				if err == nil {
					t.Fatal("invalid source accepted")
				}
				return
			}
			if err != nil || out.Suppression != want || out.Formula != MissionRateFormula || out.Basis != MissionRateBasis {
				t.Fatal(out, err)
			}
			if want == "NONE" {
				if out.Rate == nil || *out.Rate != expected {
					t.Fatal("wrong disclosed-count ratio", out)
				}
			} else if out.Rate != nil {
				t.Fatal("unknown source became a numeric rate")
			}
		})
	}
	if MissionRateSourceContract("analytics.scoped-mission-success.v1:provider_id") != "analytics.scoped-mission-success-rate.v1:provider_id" || MissionRateSourceContract("unapproved") != "" {
		t.Fatal("invalid source-contract derivation")
	}
}
