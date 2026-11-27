package app

import (
	"context"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	financeopsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	if _, err := adapter.Submit(context.Background(), meta, financeopsmodels.PostingRequest{OrganizationID: "org-1", RecordType: "PAYROLL_ITEM", RecordID: "item-1", OperationType: "PROVIDER_ALLOCATION", IdempotencyKey: "key-1", Payload: json.RawMessage(`{"amountMinor":500}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadByKey(context.Background(), meta, financeopsmodels.PostingRequest{OrganizationID: "org-1", OperationType: "PAYROLL_REVERSAL", IdempotencyKey: "key-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Submit(context.Background(), meta, financeopsmodels.PostingRequest{OrganizationID: "org-1", RecordType: "EXPENSE", RecordID: "exp-1", OperationType: "EXPENSE_POST", IdempotencyKey: "key-2", AmountMinor: 1250, Currency: "NGN"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadByKey(context.Background(), meta, financeopsmodels.PostingRequest{OrganizationID: "org-1", OperationType: "EXPENSE_POST", IdempotencyKey: "key-2"}); err != nil {
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

// TestSupportCaseAdapterMessageContract pins the support-case message calls
// to support-dispute-service's organization-case routes: a signed create on
// the organization's own case carrying the client message id, and a signed
// read-back by client message id scoped by organization.
func TestSupportCaseAdapterMessageContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Name") != "fleet-partner-service" || r.Header.Get("X-Service-Signature") == "" {
			t.Fatal("support request was not HMAC-v2 signed")
		}
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path != "/api/v1/support-disputes/admin/organization-cases/case-1/messages" {
				t.Fatalf("create path = %s", r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["organization_reference"] != "org-1" || body["client_message_id"] != "cid-1" || body["sender_reference"] != "usr-1" ||
				body["text"] != "Hello" || body["visibility"] != "INTERNAL" {
				t.Fatalf("create body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if r.URL.Path != "/api/v1/support-disputes/admin/organization-cases/case-1/messages/by-client-id/cid-1" || r.URL.Query().Get("organization_reference") != "org-1" {
				t.Fatalf("read-back = %s?%s", r.URL.Path, r.URL.RawQuery)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": "msg-1", "client_message_id": "cid-1", "content": "Hello",
			"visibility": "INTERNAL", "file_ids": []string{"fil-1"}, "created_at": "2026-11-27T10:00:00Z"}})
	}))
	defer server.Close()
	adapter := supportCaseAdapter{client: platformclients.New(platformclients.Options{ServiceName: "support-dispute-service", BaseURL: server.URL, Secret: []byte("test-secret")})}
	created, err := adapter.CreateMessage(context.Background(), engagementmodels.SupportMessageRequest{OrganizationID: "org-1", SupportCaseID: "case-1",
		ClientMessageID: "cid-1", SenderReference: "usr-1", Text: "Hello", Visibility: "INTERNAL", FileIDs: []string{"fil-1"}})
	if err != nil || created.ID != "msg-1" || created.Text != "Hello" || len(created.FileIDs) != 1 || created.CreatedAt.IsZero() {
		t.Fatalf("create = %+v, %v", created, err)
	}
	back, err := adapter.ReadMessage(context.Background(), "org-1", "case-1", "cid-1")
	if err != nil || back.ID != "msg-1" || back.ClientMessageID != "cid-1" {
		t.Fatalf("read-back = %+v, %v", back, err)
	}
}
