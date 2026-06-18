package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestWalletLedgerMigrationContainsRequiredTables(t *testing.T) {
	content, err := os.ReadFile("001_wallet_ledger.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(content)
	for _, table := range []string{
		"wallet_accounts",
		"ledger_transactions",
		"ledger_entries",
		"payment_intents",
		"paystack_webhook_events",
		"provider_bank_accounts",
		"withdrawals",
		"refunds",
		"idempotency_keys",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("expected migration to create %s", table)
		}
	}
}
