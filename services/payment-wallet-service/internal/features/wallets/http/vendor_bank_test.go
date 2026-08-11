package wallethttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

func TestVendorBankViewNeverExposesFullAccountOrRecipient(t *testing.T) {
	account := walletmodels.ProviderBankAccount{ID: "bank-1", ProviderType: vendorBankProviderType, ProviderID: "vendor-1", BankCode: "058", AccountNumber: "0123456789", AccountName: "Ada Okafor", RecipientCode: "RCP_secret", UpdatedAt: time.Now().UTC()}
	view := bankView(account)
	raw, e := json.Marshal(view)
	if e != nil {
		t.Fatal(e)
	}
	text := string(raw)
	if strings.Contains(text, account.AccountNumber) || strings.Contains(text, account.RecipientCode) {
		t.Fatalf("sensitive bank data leaked: %s", text)
	}
	if view.MaskedAccount != "******6789" || view.Status != "VERIFIED" || view.SourceVersion < 1 {
		t.Fatalf("unsafe/invalid view: %+v", view)
	}
}
func TestVendorBankProviderNamespaceIsIsolated(t *testing.T) {
	if vendorBankProviderType != "vendor_business" {
		t.Fatalf("unexpected provider namespace %q", vendorBankProviderType)
	}
	if maskAccount("1234") != "****" {
		t.Fatal("short account was not fully masked")
	}
}
