package integration_test

// Section 26: the group finance report, against real PostgreSQL.
//
// The seed is raw SQL with explicit timestamps so window boundaries are exact.
// Every seeded journal balances, so the ledger invariant suites stay valid.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	financereporthttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/http"
	financereportrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/repositories"
	financereportusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/financereports/usecases"
	"cosmicforge/logistics/shared/go/financereport"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func financeReportPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testPool(t) // also truncates finance_% and restores the catalogue
	if _, err := pool.Exec(context.Background(), `TRUNCATE managed_settlement_reconciliations, managed_settlements,
		ledger_entries, ledger_transactions, wallet_accounts, refunds, withdrawals, provider_bank_accounts,
		paystack_webhook_events, payment_intents, admin_owner_read_nonces CASCADE`); err != nil {
		t.Fatalf("clean report source tables: %v", err)
	}
	return pool
}

type reportSeed struct {
	t    *testing.T
	pool *pgxpool.Pool
}

func at(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func (s reportSeed) exec(sql string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("seed: %v\n%s", err, sql)
	}
}

func (s reportSeed) account(ownerType, ownerID, accountType, currency, normal string) string {
	s.t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), `
		INSERT INTO wallet_accounts (id, owner_type, owner_id, account_type, currency, normal_balance, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,'2024-01-01T00:00:00Z')
		ON CONFLICT (owner_type, owner_id, account_type, currency) DO UPDATE SET owner_id = EXCLUDED.owner_id
		RETURNING id`, uuid.NewString(), ownerType, ownerID, accountType, currency, normal).Scan(&id); err != nil {
		s.t.Fatalf("seed account: %v", err)
	}
	return id
}

type line struct {
	account string
	side    string
	amount  int64
}

func (s reportSeed) post(txType, sourceService, sourceRef, currency string, when time.Time, lines ...line) {
	s.t.Helper()
	id := uuid.NewString()
	s.exec(`INSERT INTO ledger_transactions (id, reference, transaction_type, source_service, source_reference, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, id, "ldt_"+id, txType, sourceService, sourceRef, when)
	var debit, credit int64
	for _, l := range lines {
		s.exec(`INSERT INTO ledger_entries (transaction_id, account_id, side, amount_kobo, currency, created_at)
			VALUES ($1,$2,$3,$4,$5,$6)`, id, l.account, l.side, l.amount, currency, when)
		if l.side == "debit" {
			debit += l.amount
		} else {
			credit += l.amount
		}
	}
	if debit != credit {
		s.t.Fatalf("unbalanced seed journal %s", txType)
	}
}

func (s reportSeed) intent(sourceService, sourceRef, currency, status string, amount, fee int64, paystackRef string) string {
	s.t.Helper()
	id := uuid.NewString()
	var ps any
	if paystackRef != "" {
		ps = paystackRef
	}
	s.exec(`INSERT INTO payment_intents (id, reference, source_service, source_reference, customer_id, provider_id, provider_type,
		amount_kobo, platform_fee_kobo, currency, payment_method, status, paystack_reference, created_at)
		VALUES ($1,$2,$3,$4,$5,'prov-secret-1','taxi',$6,$7,$8,'wallet',$9,$10,'2024-12-01T00:00:00Z')`,
		id, "pay_"+id, sourceService, sourceRef, "cust-secret-1", amount, fee, currency, status, ps)
	return id
}

// seedTwoCurrencyLedger builds the fixture described in the expectations of
// TestFinanceReportExactFigures.
func seedTwoCurrencyLedger(t *testing.T, pool *pgxpool.Pool) {
	s := reportSeed{t, pool}
	cust := func(cur string) string {
		return s.account("customer", "cust-secret-1", "customer_available", cur, "credit")
	}
	prov := func(cur string) string {
		return s.account("provider", "taxi:prov-secret-1", "provider_payable", cur, "credit")
	}
	plat := func(cur string) string { return s.account("platform", "karry-go", "platform_revenue", cur, "credit") }
	escrow := func(src, ref, cur string) string { return s.account("job", src+":"+ref, "job_escrow", cur, "credit") }

	// NGN A: held in window, settled in window.
	s.intent("taxi", "jobA", "NGN", "completed", 10000, 1500, "")
	s.post("wallet_payment_hold", "taxi", "jobA", "NGN", at("2025-01-05T10:00:00Z"),
		line{cust("NGN"), "debit", 10000}, line{escrow("taxi", "jobA", "NGN"), "credit", 10000})
	s.post("job_settlement", "taxi", "jobA", "NGN", at("2025-01-10T10:00:00Z"),
		line{escrow("taxi", "jobA", "NGN"), "debit", 10000}, line{prov("NGN"), "credit", 8500}, line{plat("NGN"), "credit", 1500})

	// NGN B: held in January, settled late (February).
	s.intent("taxi", "jobB", "NGN", "completed", 20000, 3000, "")
	s.post("wallet_payment_hold", "taxi", "jobB", "NGN", at("2025-01-20T10:00:00Z"),
		line{cust("NGN"), "debit", 20000}, line{escrow("taxi", "jobB", "NGN"), "credit", 20000})
	s.post("job_settlement", "taxi", "jobB", "NGN", at("2025-02-03T10:00:00Z"),
		line{escrow("taxi", "jobB", "NGN"), "debit", 20000}, line{prov("NGN"), "credit", 17000}, line{plat("NGN"), "credit", 3000})

	// NGN C: held before the window, refunded to the wallet inside it.
	cIntent := s.intent("dispatch", "jobC", "NGN", "refunded", 5000, 750, "")
	refundPending := s.account("system", "refunds", "refund_pending", "NGN", "credit")
	s.post("wallet_payment_hold", "dispatch", "jobC", "NGN", at("2024-12-31T23:59:59Z"),
		line{cust("NGN"), "debit", 5000}, line{escrow("dispatch", "jobC", "NGN"), "credit", 5000})
	s.post("refund_reserved", "dispatch", "jobC", "NGN", at("2025-01-15T10:00:00Z"),
		line{escrow("dispatch", "jobC", "NGN"), "debit", 5000}, line{refundPending, "credit", 5000})
	s.post("wallet_refund_processed", "dispatch", "jobC", "NGN", at("2025-01-16T10:00:00Z"),
		line{refundPending, "debit", 5000}, line{cust("NGN"), "credit", 5000})
	s.exec(`INSERT INTO refunds (id, reference, payment_intent_id, amount_kobo, currency, status, created_at, updated_at)
		VALUES ($1,'rf_C',$2,5000,'NGN','processed','2025-01-15T10:00:00Z','2025-01-16T10:00:00Z')`, uuid.NewString(), cIntent)

	// NGN D: paid by Paystack in the window, still held.
	receivable := s.account("system", "paystack", "paystack_receivable", "NGN", "debit")
	s.intent("hauling", "jobD", "NGN", "held", 8000, 800, "PSK_D")
	s.post("paystack_charge_success", "hauling", "jobD", "NGN", at("2025-01-25T10:00:00Z"),
		line{receivable, "debit", 8000}, line{escrow("hauling", "jobD", "NGN"), "credit", 8000})
	s.exec(`INSERT INTO paystack_webhook_events (id, event_key, event_type, reference, payload, processed_at, received_at)
		VALUES ($1,'evt_D','charge.success','PSK_D','{}','2025-01-25T10:00:01Z','2025-01-25T10:00:00Z')`, uuid.NewString())

	// NGN E: managed settlement in the window, reversed in the window.
	s.intent("taxi", "jobE", "NGN", "held", 10000, 1000, "")
	org := s.account("organization", "org-secret-1", "organization_payable", "NGN", "credit")
	s.post("wallet_payment_hold", "taxi", "jobE", "NGN", at("2025-01-02T10:00:00Z"),
		line{cust("NGN"), "debit", 10000}, line{escrow("taxi", "jobE", "NGN"), "credit", 10000})
	s.post("managed_work_settlement", "taxi", "jobE", "NGN", at("2025-01-12T10:00:00Z"),
		line{escrow("taxi", "jobE", "NGN"), "debit", 10000}, line{plat("NGN"), "credit", 1000},
		line{org, "credit", 7500}, line{prov("NGN"), "credit", 1500})
	s.post("managed_work_settlement_reversal", "taxi", "jobE", "NGN", at("2025-01-20T12:00:00Z"),
		line{plat("NGN"), "debit", 1000}, line{org, "debit", 7500}, line{prov("NGN"), "debit", 1500},
		line{escrow("taxi", "jobE", "NGN"), "credit", 10000})

	// NGN top-up: wallet funding, never GMV.
	s.intent("payment-wallet-service", "topup-1", "NGN", "completed", 50000, 0, "")
	s.post("paystack_charge_success", "payment-wallet-service", "topup-1", "NGN", at("2025-01-07T10:00:00Z"),
		line{receivable, "debit", 50000}, line{cust("NGN"), "credit", 50000})

	// Withdrawals: one in flight per currency, one finished (excluded).
	for _, w := range []struct {
		cur, status string
		amount      int64
	}{{"NGN", "processing", 3000}, {"NGN", "paid", 2000}, {"GHS", "pending", 1000}} {
		bank := uuid.NewString()
		s.exec(`INSERT INTO provider_bank_accounts (id, provider_type, provider_id, bank_code, account_number, account_name, recipient_code, currency)
			VALUES ($1,'taxi','prov-secret-1','058',$2,'Secret Name',$3,$4)`, bank, uuid.NewString()[:10], "RCP_"+bank, w.cur)
		s.exec(`INSERT INTO withdrawals (id, reference, provider_type, provider_id, bank_account_id, amount_kobo, currency, status, created_at, updated_at)
			VALUES ($1,$2,'taxi','prov-secret-1',$3,$4,$5,$6,'2025-01-28T00:00:00Z','2025-01-28T00:00:00Z')`,
			uuid.NewString(), "wd_"+uuid.NewString(), bank, w.amount, w.cur, w.status)
	}

	// GHS G: held and settled in the window.
	s.intent("taxi", "jobG", "GHS", "completed", 40000, 4000, "")
	s.post("wallet_payment_hold", "taxi", "jobG", "GHS", at("2025-01-03T10:00:00Z"),
		line{cust("GHS"), "debit", 40000}, line{escrow("taxi", "jobG", "GHS"), "credit", 40000})
	s.post("job_settlement", "taxi", "jobG", "GHS", at("2025-01-04T10:00:00Z"),
		line{escrow("taxi", "jobG", "GHS"), "debit", 40000}, line{prov("GHS"), "credit", 36000}, line{plat("GHS"), "credit", 4000})
}

type fig struct{ amount, count *int64 }

func figures(t *testing.T, r financereport.Report) map[string]fig {
	t.Helper()
	out := map[string]fig{}
	for _, f := range r.Figures {
		out[f.Measure+"/"+f.Currency] = fig{f.AmountMinor, f.Count}
	}
	return out
}

func wantFigure(t *testing.T, got map[string]fig, key string, amount, count int64) {
	t.Helper()
	f, ok := got[key]
	if !ok {
		t.Fatalf("%s missing", key)
	}
	if f.amount == nil || f.count == nil {
		t.Fatalf("%s is unknown, want amount=%d count=%d", key, amount, count)
	}
	if *f.amount != amount || *f.count != count {
		t.Errorf("%s = amount %d count %d, want amount %d count %d", key, *f.amount, *f.count, amount, count)
	}
}

func reportFor(t *testing.T, pool *pgxpool.Pool, start, end string) financereport.Report {
	t.Helper()
	svc := financereportusecases.Service{Reader: financereportrepositories.New(pool)}
	r, err := svc.Report(context.Background(), financereportusecases.Request{PeriodStart: at(start), PeriodEnd: at(end), Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return r
}

// sourceFingerprint is every public table's row count plus the ledger's
// signed total; a report must leave it untouched.
func sourceFingerprint(t *testing.T, pool *pgxpool.Pool, skip ...string) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		tables = append(tables, name)
	}
	rows.Close()
	out := map[string]int64{}
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[s] = true
	}
	for _, table := range tables {
		if skipped[table] {
			continue
		}
		var n int64
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[table] = n
	}
	var debit, credit int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_kobo) FILTER (WHERE side='debit'),0), COALESCE(sum(amount_kobo) FILTER (WHERE side='credit'),0) FROM ledger_entries`).Scan(&debit, &credit); err != nil {
		t.Fatal(err)
	}
	out["~ledger_debits"], out["~ledger_credits"] = debit, credit
	return out
}

func TestFinanceReportExactFigures(t *testing.T) {
	pool := financeReportPool(t)
	seedTwoCurrencyLedger(t, pool)
	before := sourceFingerprint(t, pool)

	jan := reportFor(t, pool, "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z")
	if err := jan.Validate(); err != nil {
		t.Fatalf("report invalid: %v", err)
	}
	if jan.AggregationVersion != "wallet-finance-report.v1" || jan.Unit != "MINOR_UNITS" || jan.Timezone != "Africa/Lagos" {
		t.Fatalf("report header wrong: %+v", jan)
	}
	// Snapshot measures (escrow balances) observe B's February settlement.
	if !jan.SourceWatermark.Equal(at("2025-02-03T10:00:00Z")) {
		t.Errorf("watermark = %s, want the latest relevant row (2025-02-03 settlement)", jan.SourceWatermark)
	}
	if jan.SourceWatermark.After(jan.DataAsOf) {
		t.Fatal("watermark after data_as_of")
	}
	got := figures(t, jan)
	// NGN: GMV is A+B+D+E (C was funded before the window, the top-up is not a transaction).
	wantFigure(t, got, "gross_transaction_value/NGN", 48000, 4)
	// Fee: A 1500 + E 1000 - E reversal 1000; the refunded C never posted a fee.
	wantFigure(t, got, "net_platform_fee/NGN", 1500, 3)
	wantFigure(t, got, "provider_earned/NGN", 10000, 2)
	wantFigure(t, got, "successful_settlements/NGN", 20000, 2)
	wantFigure(t, got, "pending_payouts/NGN", 3000, 1)
	wantFigure(t, got, "refunds/NGN", 5000, 1)
	// Escrow still held now: D 8000 + E (restored by its reversal) 10000.
	wantFigure(t, got, "pending_liabilities/NGN", 18000, 2)
	wantFigure(t, got, "reconciled_transactions/NGN", 8000, 1)
	// GHS is reported on its own, never summed into NGN.
	wantFigure(t, got, "gross_transaction_value/GHS", 40000, 1)
	wantFigure(t, got, "net_platform_fee/GHS", 4000, 1)
	wantFigure(t, got, "provider_earned/GHS", 36000, 1)
	wantFigure(t, got, "successful_settlements/GHS", 40000, 1)
	wantFigure(t, got, "pending_payouts/GHS", 1000, 1)
	wantFigure(t, got, "refunds/GHS", 0, 0)
	wantFigure(t, got, "pending_liabilities/GHS", 0, 0)
	if len(jan.Figures) != 2*len(financereport.Measures()) {
		t.Fatalf("want one figure per measure per currency, got %d", len(jan.Figures))
	}

	// Unknown stays unknown: no reconciliation run has completed.
	for _, f := range jan.Figures {
		if f.Measure == "unreconciled_transactions" {
			if f.AmountMinor != nil || f.Count != nil || f.UnknownReason == "" || f.Completeness != financereport.CompletenessPartial {
				t.Fatalf("unreconciled must be null with a reason, got %+v", f)
			}
		}
	}
	if jan.Completeness != financereport.CompletenessPartial {
		t.Fatal("a report with an unknown figure must be PARTIAL_COVERAGE")
	}
	for _, d := range jan.Definitions {
		if d.Measure == "net_platform_fee" && d.Recognition != financereport.SourceFigureOnly {
			t.Fatal("net platform fee must not be presented as recognized revenue")
		}
		if d.Measure != "net_platform_fee" && d.Recognition != financereport.NotRevenue {
			t.Fatalf("%s must be NOT_REVENUE", d.Measure)
		}
	}

	// The late settlement of B is not in January but is in February.
	feb := reportFor(t, pool, "2025-02-01T00:00:00Z", "2025-03-01T00:00:00Z")
	gotFeb := figures(t, feb)
	wantFigure(t, gotFeb, "gross_transaction_value/NGN", 0, 0)
	wantFigure(t, gotFeb, "successful_settlements/NGN", 20000, 1)
	wantFigure(t, gotFeb, "net_platform_fee/NGN", 3000, 1)
	wantFigure(t, gotFeb, "provider_earned/NGN", 17000, 1)

	// Recomputing never double counts.
	again := figures(t, reportFor(t, pool, "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z"))
	for key, f := range got {
		g := again[key]
		if (f.amount == nil) != (g.amount == nil) || f.amount != nil && *f.amount != *g.amount || (f.count == nil) != (g.count == nil) || f.count != nil && *f.count != *g.count {
			t.Fatalf("%s changed on recompute", key)
		}
	}

	// No aggregate report row names a customer, provider or organization.
	raw, err := json.Marshal(jan)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cust-secret-1", "prov-secret-1", "org-secret-1", "jobA", "PSK_D", "rf_C", "Secret Name"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("report leaks identifier %q", secret)
		}
	}

	// Reporting wrote nothing.
	after := sourceFingerprint(t, pool)
	for key, n := range before {
		if after[key] != n {
			t.Fatalf("%s changed from %d to %d while reporting", key, n, after[key])
		}
	}
	assertEveryJournalBalances(t, pool)
}

func TestFinanceReportUnreconciledKnownOnlyWhenAttributable(t *testing.T) {
	pool := financeReportPool(t)
	seedTwoCurrencyLedger(t, pool)
	s := reportSeed{t, pool}
	s.exec(`INSERT INTO finance_reconciliation_runs (id, scope, status, started_at, completed_at)
		VALUES ('run-1','ledger','COMPLETED','2025-01-30T00:00:00Z','2025-01-30T00:05:00Z')`)
	// A second event for PSK_D that failed processing: open, attributable to NGN.
	s.exec(`INSERT INTO paystack_webhook_events (id, event_key, event_type, reference, payload, processed_at, processing_error, received_at)
		VALUES ($1,'evt_D2','charge.success','PSK_D','{}','2025-01-26T00:00:00Z','boom','2025-01-26T00:00:00Z')`, uuid.NewString())

	got := figures(t, reportFor(t, pool, "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z"))
	wantFigure(t, got, "unreconciled_transactions/NGN", 8000, 1)
	wantFigure(t, got, "unreconciled_transactions/GHS", 0, 0)

	// An event matched to no Wallet row has no currency: the figure becomes
	// unknown again rather than a misleading count.
	s.exec(`INSERT INTO paystack_webhook_events (id, event_key, event_type, reference, payload, received_at)
		VALUES ($1,'evt_x','transfer.success','UNKNOWN_REF','{}','2025-01-27T00:00:00Z')`, uuid.NewString())
	r := reportFor(t, pool, "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z")
	for _, f := range r.Figures {
		if f.Measure == "unreconciled_transactions" && (f.Count != nil || f.AmountMinor != nil || !strings.Contains(f.UnknownReason, "unattributable")) {
			t.Fatalf("unattributable open item must make the figure unknown, got %+v", f)
		}
	}
}

func TestFinanceReportOpenPeriodIsPartial(t *testing.T) {
	pool := financeReportPool(t)
	now := time.Now().UTC().Truncate(time.Second)
	r := reportFor(t, pool, now.Add(-24*time.Hour).Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339))
	if r.Completeness != financereport.CompletenessPartial || !strings.HasPrefix(r.ErrorReason, "period_not_closed") {
		t.Fatalf("open period must be partial, got %s %q", r.Completeness, r.ErrorReason)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

// --- HTTP boundary --------------------------------------------------------

func signedReportRequest(t *testing.T, caller string, secret []byte, query, nonce string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, financereport.Path+"?"+query, nil)
	req.Header.Set(serviceauth.HeaderNonce, nonce)
	if err := serviceauth.SignRequest(req, caller, secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestFinanceReportHTTPBoundary(t *testing.T) {
	pool := financeReportPool(t)
	seedTwoCurrencyLedger(t, pool)
	gin.SetMode(gin.TestMode)
	secret := []byte(strings.Repeat("s", 40))
	reporter := financereportusecases.Service{Reader: financereportrepositories.New(pool)}
	query := url.Values{"period_start": {"2025-01-01T00:00:00Z"}, "period_end": {"2025-02-01T00:00:00Z"}, "timezone": {"Africa/Lagos"}}.Encode()

	serve := func(h *financereporthttp.Handler, req *http.Request) *httptest.ResponseRecorder {
		router := gin.New()
		h.Register(router)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	disabled := &financereporthttp.Handler{Enabled: false, Secret: secret, DB: pool, Reporter: reporter}
	if rec := serve(disabled, signedReportRequest(t, "admin-backoffice-service", secret, query, uuid.NewString())); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("flag off: got %d", rec.Code)
	}

	h := &financereporthttp.Handler{Enabled: true, Secret: secret, DB: pool, Reporter: reporter}
	if rec := serve(h, signedReportRequest(t, "vendor-service", secret, query, uuid.NewString())); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong caller: got %d", rec.Code)
	}
	if rec := serve(h, signedReportRequest(t, "admin-backoffice-service", []byte(strings.Repeat("x", 40)), query, uuid.NewString())); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: got %d", rec.Code)
	}
	unsigned := httptest.NewRequest(http.MethodGet, financereport.Path+"?"+query, nil)
	if rec := serve(h, unsigned); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: got %d", rec.Code)
	}

	before := sourceFingerprint(t, pool, "admin_owner_read_nonces")
	nonce := uuid.NewString()
	rec := serve(h, signedReportRequest(t, "admin-backoffice-service", secret, query, nonce))
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized: got %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("report must be no-store")
	}
	var report financereport.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("served report invalid: %v", err)
	}
	for _, secretID := range []string{"cust-secret-1", "prov-secret-1", "org-secret-1"} {
		if strings.Contains(rec.Body.String(), secretID) {
			t.Fatalf("served report leaks %s", secretID)
		}
	}
	after := sourceFingerprint(t, pool, "admin_owner_read_nonces")
	for key, n := range before {
		if after[key] != n {
			t.Fatalf("%s changed while serving the report", key)
		}
	}

	if rec := serve(h, signedReportRequest(t, "admin-backoffice-service", secret, query, nonce)); rec.Code != http.StatusConflict {
		t.Fatalf("replayed nonce: got %d", rec.Code)
	}
	tooLong := url.Values{"period_start": {"2025-01-01T00:00:00Z"}, "period_end": {"2025-06-01T00:00:00Z"}, "timezone": {"Africa/Lagos"}}.Encode()
	if rec := serve(h, signedReportRequest(t, "admin-backoffice-service", secret, tooLong, uuid.NewString())); rec.Code != http.StatusBadRequest {
		t.Fatalf("window over 93 days: got %d", rec.Code)
	}
	noZone := url.Values{"period_start": {"2025-01-01T00:00:00Z"}, "period_end": {"2025-02-01T00:00:00Z"}}.Encode()
	if rec := serve(h, signedReportRequest(t, "admin-backoffice-service", secret, noZone, uuid.NewString())); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing timezone: got %d", rec.Code)
	}
}

func TestFinanceReportSecretConfiguration(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, raw []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(raw)), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	report := write("report", []byte(strings.Repeat("r", 32)))
	reporter := financereportusecases.Service{}
	pool := &pgxpool.Pool{}

	t.Setenv(financereporthttp.EnvEnabled, "false")
	t.Setenv(financereporthttp.EnvSecret, filepath.Join(dir, "missing"))
	if h, err := financereporthttp.FromEnvironment(pool, reporter); err != nil || h.Enabled {
		t.Fatalf("flag off must not read the secret: %v", err)
	}

	t.Setenv(financereporthttp.EnvEnabled, "true")
	t.Setenv(financereporthttp.EnvSecret, write("short", []byte("too-short")))
	if _, err := financereporthttp.FromEnvironment(pool, reporter); err == nil {
		t.Fatal("short secret accepted")
	}
	t.Setenv(financereporthttp.EnvSecret, report)
	t.Setenv("PAYMENT_WALLET_ADMIN_RESOURCE_SECRET_FILE", write("resource", []byte(strings.Repeat("r", 32))))
	if _, err := financereporthttp.FromEnvironment(pool, reporter); err == nil {
		t.Fatal("secret shared with another admin boundary accepted")
	}
	t.Setenv("PAYMENT_WALLET_ADMIN_RESOURCE_SECRET_FILE", write("resource2", []byte(strings.Repeat("q", 32))))
	h, err := financereporthttp.FromEnvironment(pool, reporter)
	if err != nil || !h.Enabled || len(h.Secret) != 32 {
		t.Fatalf("dedicated secret refused: %v", err)
	}
}
