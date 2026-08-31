package support

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"
)

func TestCreateRefundCaseUsesHMACAndSendsNoMoneyInstruction(t *testing.T) {
	secret := []byte("support-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/support-disputes/internal/commerce-cases" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		caller, err := verifier.VerifyRequest(r)
		if err != nil || caller != "vendor-service" {
			t.Fatalf("caller=%q err=%v", caller, err)
		}
		var body map[string]any
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"amount", "amount_minor", "refund_amount", "wallet_reference"} {
			if _, exists := body[forbidden]; exists {
				t.Fatalf("support handoff included financial instruction %q", forbidden)
			}
		}
		ctx, _ := body["context"].(map[string]any)
		if ctx["commerce_order_ref"] != "parent-a" || ctx["vendor_order_ref"] != "vendor-order-a" || body["description"] != "AMBIGUOUS_DELIVERY_FAILURE" {
			t.Fatalf("body=%v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"case_ref":"case-a","operation_ref":"support-key-a","category":"REFUND_PROBLEM","status":"open"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, string(secret), time.Second)
	result, err := client.CreateRefundCase(context.Background(), orders.RefundSupportRequest{
		OrganizationReference: "business-a", IdempotencyKey: "support-key-a",
		ReasonCode: "AMBIGUOUS_DELIVERY_FAILURE", CustomerReference: "customer-a",
		MissionReference: "mission-a", ParentOrderReference: "parent-a", VendorOrderReference: "vendor-order-a",
	})
	if err != nil || result.CaseID != "case-a" || result.Status != "open" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCreateCommerceCaseReadsBackUnknownOutcome(t *testing.T) {
	secret := []byte("support-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret}, time.Minute)
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != "vendor-service" {
			t.Fatalf("caller=%q err=%v", caller, err)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "response lost after commit", http.StatusGatewayTimeout)
			return
		}
		if r.Method != http.MethodGet || r.URL.Query().Get("vendor_ref") != "vendor-a" {
			t.Fatalf("read-back=%s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"case_ref":"case-stable","operation_ref":"op-stable","category":"QUALITY_ISSUE","status":"open","version":1}}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, string(secret), time.Second)
	value, err := client.CreateCommerceCase(context.Background(), supportcase.CreateRequest{SchemaVersion: 1, OperationRef: "op-stable", Category: supportcase.QualityIssue, Subject: "Quality issue", Description: "A bounded quality report", CustomerRef: "customer-a", Context: supportcase.Context{CommerceOrderRef: "order-a", VendorOrderRef: "vendor-order-a", VendorRef: "vendor-a"}})
	if err != nil || value.CaseRef != "case-stable" || posts != 1 {
		t.Fatalf("value=%+v posts=%d err=%v", value, posts, err)
	}
}
