// Live wiring probe for KARRYAM Part E.
//
// Drives real HMAC-signed service-to-service calls against a running
// payment-wallet-service, exactly as Fleet and the execution services do.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"cosmicforge/logistics/shared/go/managedsettlement"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const base = "http://127.0.0.1:18106"

var failures int

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failures++
	}
	fmt.Printf("%-4s %-58s %s\n", status, name, detail)
}

// signedCall makes a real HMAC v2 internal call.
func signedCall(method, path, service, secret string, body any) (int, string) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	} else {
		payload = []byte{}
	}
	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, base+path, reader)
	if err != nil {
		return 0, err.Error()
	}
	if len(payload) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(request, service, []byte(secret), payload, time.Now()); err != nil {
		return 0, err.Error()
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return 0, err.Error()
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw)
}

func main() {
	fmt.Println("=== PART E: live wiring certification against a running payment-wallet-service ===")
	fmt.Println()

	// --- Reachability -------------------------------------------------------
	response, err := http.Get(base + "/health")
	if err != nil {
		fmt.Println("service unreachable:", err)
		os.Exit(1)
	}
	check("service is reachable", response.StatusCode == 200, fmt.Sprintf("GET /health -> %d", response.StatusCode))
	response.Body.Close()

	// --- Service identity and HMAC -----------------------------------------
	// A correctly signed Fleet call reaches the handler (whatever it answers
	// about the payload); a wrong secret and an unsigned call do not.
	settlementPath := "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"
	status, _ := signedCall(http.MethodPost, settlementPath, "fleet-partner-service", "live-fleet-secret",
		map[string]any{"instruction_id": "probe", "instruction_version": 1})
	check("Fleet's signature is accepted", status != 401 && status != 0,
		fmt.Sprintf("signed POST -> %d (not 401)", status))

	status, _ = signedCall(http.MethodPost, settlementPath, "fleet-partner-service", "wrong-secret",
		map[string]any{"instruction_id": "probe", "instruction_version": 1})
	check("a wrong secret is refused", status == 401, fmt.Sprintf("bad signature -> %d", status))

	unsigned, err := http.Post(base+settlementPath, "application/json", bytes.NewReader([]byte(`{}`)))
	unsignedStatus := 0
	if err == nil {
		unsignedStatus = unsigned.StatusCode
		unsigned.Body.Close()
	}
	check("an unsigned call is refused", unsignedStatus == 401, fmt.Sprintf("unsigned -> %d", unsignedStatus))

	// --- Caller narrowing ---------------------------------------------------
	// Posting a settlement is Fleet's alone, even for a correctly signed
	// execution service.
	status, body := signedCall(http.MethodPost, settlementPath, "taxi-service", "live-taxi-secret",
		map[string]any{"instruction_id": "probe", "instruction_version": 1})
	check("only Fleet may instruct a settlement", status == 403,
		fmt.Sprintf("Taxi-signed POST -> %d", status))
	_ = body

	// --- Read-back by each execution service (WAL-011B) --------------------
	readPath := "/api/v1/payment-wallet/internal/finance/v1/managed-settlements/by-job/TAXI/probe_job"
	for _, caller := range []struct{ service, secret string }{
		{"taxi-service", "live-taxi-secret"},
		{"dispatch-delivery-service", "live-dispatch-secret"},
		{"driver-hauling-service", "live-hauling-secret"},
		{"fleet-partner-service", "live-fleet-secret"},
	} {
		status, _ := signedCall(http.MethodGet, readPath, caller.service, caller.secret, nil)
		check("read-back reaches "+caller.service, status == 200, fmt.Sprintf("-> %d", status))
	}

	// An unrelated service may not read a settlement at all.
	status, _ = signedCall(http.MethodGet, readPath, "support-dispute-service", "live-support-secret", nil)
	check("an unrelated caller cannot read settlements", status == 403, fmt.Sprintf("-> %d", status))

	// --- The shared client against the real service (WAL-011A/B) -----------
	client := managedsettlement.Client{
		BaseURL: base, ServiceName: "driver-hauling-service",
		Secret: []byte("live-hauling-secret"), HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
	outcome, err := client.Lookup(context.Background(), managedsettlement.IdentityFor("HAULING", "no_such_job"))
	check("shared client reads an unsettled job", err == nil && !outcome.Settled && outcome.Resolved(),
		fmt.Sprintf("outcome=%+v err=%v", outcome, err))

	// Wrong credentials must be Unknown, never "nothing settled".
	badClient := client
	badClient.Secret = []byte("wrong-secret")
	outcome, err = badClient.Lookup(context.Background(), managedsettlement.IdentityFor("HAULING", "no_such_job"))
	check("a refused read is Unknown, not unsettled",
		err != nil && outcome.Unknown && !outcome.Resolved() && !outcome.Settled,
		fmt.Sprintf("outcome=%+v", outcome))

	// An unreachable Wallet likewise.
	unreachable := client
	unreachable.BaseURL = "http://127.0.0.1:1"
	outcome, err = unreachable.Lookup(context.Background(), managedsettlement.IdentityFor("HAULING", "job"))
	check("an unreachable Wallet is Unknown, not unsettled",
		err != nil && outcome.Unknown && !outcome.Settled, fmt.Sprintf("outcome=%+v", outcome))

	// --- Commerce delivery recovery plan (WAL-029) -------------------------
	planPath := "/api/v1/payment-wallet/internal/commerce/allocations/probe_order/delivery-recovery-plan"
	status, _ = signedCall(http.MethodPost, planPath, "vendor-service", "live-vendor-secret",
		map[string]any{"recover_minor": 100})
	check("Vendor may ask for a recovery plan", status != 401 && status != 403,
		fmt.Sprintf("-> %d (reached the handler)", status))
	status, _ = signedCall(http.MethodPost, planPath, "taxi-service", "live-taxi-secret",
		map[string]any{"recover_minor": 100})
	check("an execution service may not plan a recovery", status == 403, fmt.Sprintf("-> %d", status))

	// --- Dispute reversal route (WAL-030) ----------------------------------
	disputePath := "/api/v1/payment-wallet/internal/commerce/allocations/probe_order/dispute-reversals"
	status, _ = signedCall(http.MethodPost, disputePath, "support-dispute-service", "live-support-secret",
		map[string]any{"item_reference": "item_1"})
	check("support-dispute may reverse for a dispute", status != 401 && status != 403,
		fmt.Sprintf("-> %d (reached the handler)", status))
	status, _ = signedCall(http.MethodPost, disputePath, "driver-hauling-service", "live-hauling-secret",
		map[string]any{"item_reference": "item_1"})
	check("an execution service may not reverse a dispute", status == 403, fmt.Sprintf("-> %d", status))

	// --- Provider surface (VIS-013) ----------------------------------------
	providerResponse, err := http.Get(base + "/api/v1/payment-wallet/provider/earnings")
	providerStatus := 0
	if err == nil {
		providerStatus = providerResponse.StatusCode
		providerResponse.Body.Close()
	}
	check("the provider surface refuses an unauthenticated read", providerStatus == 401,
		fmt.Sprintf("-> %d", providerStatus))

	// A provider bearer token must not reach a settlement route at all.
	settlementViaProvider, err := http.Get(base + "/api/v1/payment-wallet/provider/managed-settlements")
	providerSettlementStatus := 0
	if err == nil {
		providerSettlementStatus = settlementViaProvider.StatusCode
		settlementViaProvider.Body.Close()
	}
	check("no settlement route exists on the provider surface",
		providerSettlementStatus == 404 || providerSettlementStatus == 401,
		fmt.Sprintf("-> %d", providerSettlementStatus))

	// --- Timeout handling ---------------------------------------------------
	timeoutClient := client
	timeoutClient.HTTPClient = &http.Client{Timeout: time.Nanosecond}
	outcome, err = timeoutClient.Lookup(context.Background(), managedsettlement.IdentityFor("HAULING", "job"))
	check("a timeout is Unknown, never a confirmed failure",
		err != nil && outcome.Unknown && !outcome.Settled, fmt.Sprintf("outcome=%+v", outcome))

	liveSettlementJourney(seedLiveHeldJob())

	fmt.Println()
	if failures > 0 {
		fmt.Printf("PART E: %d CHECK(S) FAILED\n", failures)
		os.Exit(1)
	}
	fmt.Println("PART E: ALL LIVE WIRING CHECKS PASSED")
}
