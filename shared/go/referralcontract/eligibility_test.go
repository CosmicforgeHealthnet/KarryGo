package referralcontract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestHashBankAccount(t *testing.T) {
	pepper := []byte("pepper")
	a := HashBankAccount(pepper, "058", "0123456789")
	if a == "" || a != HashBankAccount(pepper, " 058 ", " 0123456789 ") {
		t.Fatal("bank hash must be stable and ignore surrounding spaces")
	}
	if a == HashBankAccount(pepper, "044", "0123456789") || a == HashPhone(pepper, "0123456789") {
		t.Fatal("bank hash must depend on the bank and differ from other identifier kinds")
	}
	if HashBankAccount(pepper, "058", "") != "" {
		t.Fatal("no account number, no hash")
	}
}

func TestCheckWithdrawalSignsAndDecodes(t *testing.T) {
	secret := []byte("wallet-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"payment-wallet-service": secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != WithdrawalEligibilityPath {
			t.Errorf("path = %s", r.URL.Path)
		}
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != "payment-wallet-service" {
			t.Errorf("request not signed as the wallet: %q %v", caller, err)
		}
		var body WithdrawalEligibilityRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ActorID != "drv_1" {
			t.Errorf("body = %+v, %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"eligible":false,"reason":"under_review"}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL + "/", ServiceName: "payment-wallet-service", Secret: secret}
	answer, err := client.CheckWithdrawal(context.Background(), WithdrawalEligibilityRequest{
		ActorType: ActorHauling, ActorID: "drv_1", AmountKobo: 500_000, BankAccountHash: "h",
	})
	if err != nil || answer.Eligible || answer.Reason != IneligibleUnderReview {
		t.Fatalf("CheckWithdrawal() = %+v, %v", answer, err)
	}
}

func TestCheckWithdrawalFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"success":false,"error":{"message":"down"}}`))
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, ServiceName: "payment-wallet-service", Secret: []byte("s")}
	if _, err := client.CheckWithdrawal(context.Background(), WithdrawalEligibilityRequest{
		ActorType: ActorTaxi, ActorID: "d", AmountKobo: 1, BankAccountHash: "h",
	}); err == nil {
		t.Fatal("an unavailable referral-service must be an error, never a yes")
	}
	if _, err := client.CheckWithdrawal(context.Background(), WithdrawalEligibilityRequest{ActorType: ActorCustomer, ActorID: "c", AmountKobo: 1, BankAccountHash: "h"}); err == nil {
		t.Fatal("customers cannot cash out referral earnings")
	}
}
