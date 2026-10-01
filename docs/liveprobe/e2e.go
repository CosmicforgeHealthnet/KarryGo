package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

// liveSettlementJourney posts a real managed settlement over HMAC against the
// running service and reads it back the way each execution service does.
//
// This is the whole Section 12 money path exercised as a running system: Fleet
// instructs, Wallet posts one balanced journal, and the service that completed
// the job learns the money moved.
func liveSettlementJourney(jobID string) {
	fmt.Println()
	fmt.Println("--- end-to-end: Fleet instructs, Wallet posts, the execution service reads back ---")

	instruction := map[string]any{
		"instruction_id": "msi_" + jobID, "instruction_version": 1,
		"organization_id": "org_live", "membership_id": "mem_live",
		"provider_id": "prov_live", "provider_type": "taxi",
		"source_service": "taxi", "source_job_id": jobID,
		"currency": "NGN", "gross_basis_minor": 9000,
		"provider_share_minor": 1500, "organization_share_minor": 7500,
		"compensation_plan_id": "plan_live", "compensation_plan_version": 1,
		"compensation_resolution_version": 1,
		"idempotency_key":                 "settlement:" + jobID,
	}
	path := "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"

	status, body := signedCall(http.MethodPost, path, "fleet-partner-service", "live-fleet-secret", instruction)
	check("Fleet posts a managed settlement", status == 200, fmt.Sprintf("-> %d", status))
	if status != 200 {
		fmt.Printf("     body: %s\n", body)
		return
	}

	// Resending the same instruction returns the original outcome rather than
	// posting a second journal.
	repeatStatus, repeatBody := signedCall(http.MethodPost, path, "fleet-partner-service", "live-fleet-secret", instruction)
	check("resending the instruction posts once", repeatStatus == 200, fmt.Sprintf("-> %d", repeatStatus))
	var first, second struct {
		Data struct {
			LedgerTransactionID string `json:"ledger_transaction_id"`
			Status              string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &first)
	_ = json.Unmarshal([]byte(repeatBody), &second)
	check("the retry returns the original journal",
		first.Data.LedgerTransactionID != "" && first.Data.LedgerTransactionID == second.Data.LedgerTransactionID,
		fmt.Sprintf("%s == %s", first.Data.LedgerTransactionID, second.Data.LedgerTransactionID))

	// The same identity with different amounts is a conflict, not a repeat.
	changed := map[string]any{}
	for key, value := range instruction {
		changed[key] = value
	}
	changed["provider_share_minor"] = 2500
	changed["organization_share_minor"] = 6500
	conflictStatus, _ := signedCall(http.MethodPost, path, "fleet-partner-service", "live-fleet-secret", changed)
	check("a changed instruction is a conflict", conflictStatus == 409, fmt.Sprintf("-> %d", conflictStatus))

	// Each execution service reads its own completion back through the shared
	// client, over real HMAC.
	for _, caller := range []struct{ service, secret string }{
		{"taxi-service", "live-taxi-secret"},
		{"driver-hauling-service", "live-hauling-secret"},
	} {
		client := managedsettlement.Client{
			BaseURL: base, ServiceName: caller.service, Secret: []byte(caller.secret),
			HTTPClient: &http.Client{Timeout: 10 * time.Second},
		}
		outcome, err := client.Lookup(context.Background(), managedsettlement.IdentityFor("taxi", jobID))
		check(caller.service+" sees the money moved",
			err == nil && outcome.Settled, fmt.Sprintf("settled=%t err=%v", outcome.Settled, err))
	}

	// The legacy direct-payout path must not pay a job Fleet settled. What
	// matters is whether money moved, not which status code came back: an
	// idempotent no-op on an already-completed intent is a safe answer, a
	// second payout is not.
	legacyPath := "/api/v1/payment-wallet/internal/jobs/taxi/" + jobID + "/complete"
	legacyStatus, _ := signedCall(http.MethodPost, legacyPath, "taxi-service", "live-taxi-secret",
		map[string]any{"idempotency_key": "legacy-" + jobID})
	check("the legacy payout path pays nothing for managed work",
		countJobCompletionJournals(jobID) == 0,
		fmt.Sprintf("status %d, job_completion journals for this job: %d",
			legacyStatus, countJobCompletionJournals(jobID)))

	// Reversal is Fleet's alone.
	reversePath := path + "/msi_" + jobID + "/1/reverse"
	reverseStatus, _ := signedCall(http.MethodPost, reversePath, "taxi-service", "live-taxi-secret",
		map[string]any{"reason_code": "PROBE"})
	check("an execution service cannot reverse a settlement", reverseStatus == 403,
		fmt.Sprintf("-> %d", reverseStatus))

	fleetReverse, _ := signedCall(http.MethodPost, reversePath, "fleet-partner-service", "live-fleet-secret",
		map[string]any{"reason_code": "PROBE_REVERSAL"})
	check("Fleet may reverse a settlement", fleetReverse == 200, fmt.Sprintf("-> %d", fleetReverse))

	// After reversal the job reads back as not settled: the money moved and
	// then moved back.
	client := managedsettlement.Client{
		BaseURL: base, ServiceName: "taxi-service", Secret: []byte("live-taxi-secret"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	outcome, err := client.Lookup(context.Background(), managedsettlement.IdentityFor("taxi", jobID))
	check("a reversed settlement reads back as unpaid",
		err == nil && !outcome.Settled, fmt.Sprintf("settled=%t", outcome.Settled))
}
