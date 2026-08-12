package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func TestFinancialPostingAdapterRoutesToAuthoritativeService(t *testing.T) {
	t.Helper()
	walletCalls := 0
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		walletCalls++
		if r.Header.Get("X-Service-Name") != "fleet-partner-service" || r.Header.Get("X-Service-Signature") == "" {
			t.Fatal("wallet request was not HMAC-v2 signed")
		}
		wantPath := "/api/v1/payment-wallet/internal/finance/provider-allocations"
		if r.Method == http.MethodGet {
			wantPath = "/api/v1/payment-wallet/internal/finance/operations/PROVIDER_ALLOCATION/by-idempotency"
		}
		if r.URL.Path != wantPath {
			t.Fatalf("wallet path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": "wal-1", "status": "POSTED", "version": 2}})
	}))
	defer wallet.Close()

	accountingCalls := 0
	accounting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountingCalls++
		if r.Header.Get("X-Service-Name") != "fleet-partner-service" || r.Header.Get("X-Service-Signature") == "" {
			t.Fatal("accounting request was not HMAC-v2 signed")
		}
		if r.Method == http.MethodPost {
			if r.URL.Path != "/api/v1/accounting/internal/postings" {
				t.Fatalf("accounting path = %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["operationType"] != "EXPENSE_POST" || body["recordId"] != "exp-1" || body["amountMinor"] != float64(1250) {
				t.Fatalf("unexpected accounting body: %#v", body)
			}
		} else if r.URL.Path != "/api/v1/accounting/internal/postings/by-idempotency" || r.URL.Query().Get("idempotencyKey") != "key-2" {
			t.Fatalf("accounting read-back = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": "acct-1", "status": "CONFIRMED", "version": 4}})
	}))
	defer accounting.Close()

	adapter := financialPostingAdapter{
		wallet:     walletPostingAdapter{client: platformclients.New(platformclients.Options{ServiceName: "payment-wallet-service", BaseURL: wallet.URL, Secret: []byte("test-secret")})},
		accounting: accountingPostingAdapter{client: platformclients.New(platformclients.Options{ServiceName: "accounting-service", BaseURL: accounting.URL, Secret: []byte("test-secret")})},
	}
	meta := platformclients.CallMeta{OrganizationID: "org-1", IdempotencyKey: "key-1"}
	if _, err := adapter.Submit(context.Background(), meta, financeops.PostingRequest{OrganizationID: "org-1", RecordType: "PAYROLL_ITEM", RecordID: "item-1", OperationType: "PROVIDER_ALLOCATION", IdempotencyKey: "key-1", Payload: json.RawMessage(`{"amountMinor":500}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadByKey(context.Background(), meta, financeops.PostingRequest{OrganizationID: "org-1", OperationType: "PAYROLL_REVERSAL", IdempotencyKey: "key-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Submit(context.Background(), meta, financeops.PostingRequest{OrganizationID: "org-1", RecordType: "EXPENSE", RecordID: "exp-1", OperationType: "EXPENSE_POST", IdempotencyKey: "key-2", AmountMinor: 1250, Currency: "NGN"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadByKey(context.Background(), meta, financeops.PostingRequest{OrganizationID: "org-1", OperationType: "EXPENSE_POST", IdempotencyKey: "key-2"}); err != nil {
		t.Fatal(err)
	}
	if !adapter.Configured("SETTLEMENT_REQUEST") || walletCalls != 2 || accountingCalls != 2 {
		t.Fatalf("wallet calls=%d accounting calls=%d", walletCalls, accountingCalls)
	}
}

func TestFinancialPostingAdapterFailsClosedForUnknownOperation(t *testing.T) {
	adapter := financialPostingAdapter{}
	if adapter.Configured("UNKNOWN") {
		t.Fatal("unknown operation must not be configured")
	}
}
