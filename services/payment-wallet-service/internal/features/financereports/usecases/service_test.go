package financereportusecases

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	repo "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/repositories"
	"cosmicforge/logistics/shared/go/financereport"
)

func q(start, end, zone string) map[string][]string {
	v := map[string][]string{}
	if start != "" {
		v["period_start"] = []string{start}
	}
	if end != "" {
		v["period_end"] = []string{end}
	}
	if zone != "" {
		v["timezone"] = []string{zone}
	}
	return v
}

func TestParseRequest(t *testing.T) {
	good := q("2025-01-01T00:00:00Z", "2025-04-04T00:00:00Z", "Africa/Lagos") // exactly 93 days
	if _, err := ParseRequest(good); err != nil {
		t.Fatalf("93-day window refused: %v", err)
	}
	bad := []map[string][]string{
		q("2025-01-01T00:00:00Z", "2025-04-04T00:00:01Z", "Africa/Lagos"), // > 93 days
		q("2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", "Africa/Lagos"), // empty
		q("2025-02-01T00:00:00Z", "2025-01-01T00:00:00Z", "Africa/Lagos"), // reversed
		q("2025-01-01T00:00:00+01:00", "2025-02-01T00:00:00Z", "Africa/Lagos"),
		q("2025-01-01", "2025-02-01T00:00:00Z", "Africa/Lagos"),
		q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", ""),
		q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", "Local"),
		q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", "Mars/Olympus"),
		q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", "WAT"),
	}
	for i, v := range bad {
		if _, err := ParseRequest(v); err == nil {
			t.Errorf("case %d accepted: %v", i, v)
		}
	}
	extra := q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", "UTC")
	extra["customer_id"] = []string{"x"}
	if _, err := ParseRequest(extra); err == nil {
		t.Error("unknown parameter accepted")
	}
	dup := q("2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z", "UTC")
	dup["timezone"] = []string{"UTC", "Africa/Lagos"}
	if _, err := ParseRequest(dup); err == nil {
		t.Error("duplicate parameter accepted")
	}
}

func TestBuildNeverInventsOrSumsAcrossCurrencies(t *testing.T) {
	asOf := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	ts := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	req := Request{PeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Timezone: "UTC"}
	snap := repo.Snapshot{
		DataAsOf: asOf,
		Measures: map[string][]repo.Bucket{
			repo.MeasureGTV:         {{Currency: "NGN", Amount: 100, Count: 1, MaxTS: &ts}, {Currency: "GHS", Amount: 7, Count: 1}},
			repo.MeasureLiabilities: {{Currency: "GHS", Amount: -5, Count: 1}},
		},
		CommerceReleases:        []repo.Bucket{{Currency: "NGN", Count: 2}},
		CompletedReconciliation: true,
	}
	r := Build(req, snap)
	if err := r.Validate(); err != nil {
		t.Fatalf("built report invalid: %v", err)
	}
	if !r.SourceWatermark.Equal(ts) {
		t.Fatalf("watermark %s", r.SourceWatermark)
	}
	seen := map[string]financereport.Figure{}
	for _, f := range r.Figures {
		seen[f.Measure+"/"+f.Currency] = f
	}
	if *seen["gross_transaction_value/NGN"].AmountMinor != 100 || *seen["gross_transaction_value/GHS"].AmountMinor != 7 {
		t.Fatal("currencies mixed")
	}
	if f := seen["pending_liabilities/GHS"]; f.AmountMinor != nil || f.UnknownReason == "" {
		t.Fatal("negative escrow must be unknown, never clamped")
	}
	if f := seen["successful_settlements/NGN"]; f.Completeness != financereport.CompletenessPartial {
		t.Fatal("commerce releases must mark settlements partial")
	}
	if r.Completeness != financereport.CompletenessPartial {
		t.Fatal("partial figure must make the report partial")
	}
	// No source rows at all: watermark falls back to the snapshot, still valid.
	empty := Build(req, repo.Snapshot{DataAsOf: asOf, Measures: map[string][]repo.Bucket{}})
	if err := empty.Validate(); err != nil || !empty.SourceWatermark.Equal(asOf) || len(empty.Figures) != 0 {
		t.Fatalf("empty report: %v %+v", err, empty)
	}
}

// The published definitions must describe the SQL actually executed.
func TestDefinitionsMatchExecutedSQL(t *testing.T) {
	tokens := map[string][]string{
		"gross_transaction_value":   {"wallet_payment_hold", "paystack_charge_success", "commerce_payment_hold", "final_authorized_minor", "payment-wallet-service"},
		"net_platform_fee":          {"platform_revenue"},
		"provider_earned":           {"provider_payable", "withdrawal"},
		"successful_settlements":    {"job_settlement", "managed_work_settlement"},
		"pending_payouts":           {"pending", "processing", "requires_otp", "REQUESTED", "PENDING_APPROVAL", "APPROVED", "PROCESSING", "FAILED_RETRYABLE", "DEAD_LETTER"},
		"refunds":                   {"processed", "refund_processed", "wallet_refund_processed", "COMPLETED", "PARTIAL_REFUND", "APPLIED"},
		"pending_liabilities":       {"job_escrow", "commerce_escrow"},
		"reconciled_transactions":   {"paystack_reference", "paystack_transfer_code", "paystack_refund_reference", "processing_error"},
		"unreconciled_transactions": {"managed_settlement_reconciliations", "OPEN"},
	}
	for _, d := range financereport.Measures() {
		sql, ok := repo.SQLByMeasure[d.Measure]
		if !ok {
			t.Fatalf("no SQL for %s", d.Measure)
		}
		text := d.SourceField + " " + d.Calculation
		for _, token := range tokens[d.Measure] {
			if !strings.Contains(sql, token) || !strings.Contains(text, token) {
				t.Errorf("%s: token %q must appear in both SQL and definition", d.Measure, token)
			}
		}
		if strings.Contains(strings.ToLower(sql), "insert ") || strings.Contains(strings.ToLower(sql), "update ") || strings.Contains(strings.ToLower(sql), "delete ") {
			t.Errorf("%s SQL is not read-only", d.Measure)
		}
	}
}

func TestOpenAPIDocumentsTheRoute(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/openapi/admin-finance-report.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 1 {
		t.Fatalf("want exactly one path, got %d", len(doc.Paths))
	}
	ops := doc.Paths[financereport.Path]
	if _, ok := ops["get"]; !ok || len(ops) != 1 {
		t.Fatalf("want GET %s only", financereport.Path)
	}
	for _, code := range []string{`"200"`, `"400"`, `"401"`, `"403"`, `"409"`, `"503"`} {
		if !strings.Contains(string(raw), code) {
			t.Errorf("response %s undocumented", code)
		}
	}
}
