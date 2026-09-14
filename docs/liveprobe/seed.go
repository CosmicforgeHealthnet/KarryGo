package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// seedLiveHeldJob puts a real customer payment into escrow in the running
// service's own database, which is the state every completion path starts from.
//
// Seeded directly rather than through an API because the customer payment flow
// is not what Part E is certifying: the money path from Fleet's instruction
// onwards is.
func seedLiveHeldJob() string {
	jobID := fmt.Sprintf("live_job_%d", time.Now().UnixNano())
	intentID := fmt.Sprintf("11111111-1111-4111-8111-%012d", time.Now().UnixNano()%1_000_000_000_000)
	escrowID := fmt.Sprintf("22222222-2222-4222-8222-%012d", time.Now().UnixNano()%1_000_000_000_000)
	transactionID := fmt.Sprintf("33333333-3333-4333-8333-%012d", time.Now().UnixNano()%1_000_000_000_000)

	statements := []string{
		fmt.Sprintf(`INSERT INTO payment_intents (id,reference,source_service,source_reference,customer_id,amount_kobo,platform_fee_kobo,currency,payment_method,status)
			VALUES ('%s','pay_%s','taxi','%s','44444444-4444-4444-8444-444444444444',10000,1000,'NGN','wallet','held')`,
			intentID, jobID, jobID),
		fmt.Sprintf(`INSERT INTO wallet_accounts (id,owner_type,owner_id,account_type,currency,normal_balance)
			VALUES ('%s','job','taxi:%s','job_escrow','NGN','credit')`, escrowID, jobID),
		fmt.Sprintf(`INSERT INTO ledger_transactions (id,reference,transaction_type,status,source_service,source_reference,idempotency_key)
			VALUES ('%s','ldt_%s','job_funding','posted','taxi','fund_%s','fund-%s')`,
			transactionID, jobID, jobID, jobID),
		fmt.Sprintf(`INSERT INTO ledger_entries (transaction_id,account_id,side,amount_kobo,currency,memo)
			VALUES ('%s','%s','credit',10000,'NGN','Customer paid into job escrow')`, transactionID, escrowID),
	}
	for _, statement := range statements {
		command := exec.CommandContext(context.Background(), "psql",
			"-h", "127.0.0.1", "-p", "5499", "-U", "postgres", "-d", "wallet_e2e_test",
			"-v", "ON_ERROR_STOP=1", "-q", "-c", statement)
		if output, err := command.CombinedOutput(); err != nil {
			fmt.Printf("seed failed: %v\n%s\n", err, strings.TrimSpace(string(output)))
			return jobID
		}
	}
	return jobID
}

// countJobCompletionJournals counts legacy direct-payout journals for one job.
//
// The question a status code cannot answer: did the legacy path actually move
// money for work Fleet had already settled?
func countJobCompletionJournals(jobID string) int {
	command := exec.CommandContext(context.Background(), "psql",
		"-h", "127.0.0.1", "-p", "5499", "-U", "postgres", "-d", "wallet_e2e_test",
		"-tAc", fmt.Sprintf(
			`SELECT count(*) FROM ledger_transactions
			 WHERE source_reference = '%s' AND transaction_type NOT IN ('job_funding','managed_work_settlement','managed_work_settlement_reversal')`,
			jobID))
	output, err := command.Output()
	if err != nil {
		return -1
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	return count
}
