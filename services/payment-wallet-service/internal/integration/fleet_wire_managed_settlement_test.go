package integration_test

// FLEET-13.11: the managed-settlement route as Fleet actually calls it.
//
// The other managed-settlement tests drive the repository directly. This one
// goes through the production gin route and HMAC middleware with the exact
// body Fleet's posting adapter sends (financeops.ManagedSettlementSubmission
// payload plus the adapter's envelope fields), signed the way Fleet's
// platform client signs, so a drift in field names, casing, the source and
// provider identity Fleet maps to, or the caller identity is caught here.
// Fleet pins the same body from its side in
// services/fleet-partner-service/internal/integration/managed_settlement_delivery_test.go.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	wallethttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/http"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const fleetWireSecret = "fleet-wire-test-secret"

func fleetWireRouter(t *testing.T, repo *walletrepositories.PostgresWalletRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	service := walletusecases.NewWalletService(walletusecases.Options{Repository: repo})
	wallethttp.RegisterRoutes(router.Group("/api/v1/payment-wallet"), service,
		[]byte("customer-secret"), map[string][]byte{}, serviceauth.Secrets{
			"fleet-partner-service": []byte(fleetWireSecret),
			"driver-taxi-service":   []byte("taxi-secret"),
		})
	return router
}

func fleetWireCall(t *testing.T, router *gin.Engine, caller, secret, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, caller, []byte(secret), payload, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// fleetWireBody is the body Fleet's adapter sends, field for field.
func fleetWireBody(source, providerType, jobID string, gross, provider, organization int64) map[string]any {
	return map[string]any{
		// financeops.ManagedSettlementSubmission.payload()
		"instruction_id": "mwsi_" + jobID, "instruction_version": 1,
		"organization_id": "org_wire", "membership_id": "mem_wire",
		"provider_id": "prov_wire", "provider_type": providerType,
		"source_service": source, "source_job_id": jobID, "source_assignment_id": "asg_" + jobID,
		"currency": "NGN", "gross_basis_minor": gross,
		"provider_share_minor": provider, "organization_share_minor": organization,
		"compensation_plan_id": "cplan_wire", "compensation_plan_version": 1,
		"compensation_resolution_version": 1, "correlation_id": "corr_" + jobID,
		"idempotency_key": "settlement:" + jobID,
		// walletPostingAdapter envelope fields, ignored by this contract.
		"payloadVersion": 1, "organizationId": "org_wire",
		"idempotencyKey": "settlement:" + jobID, "expectedVersion": 0,
	}
}

func TestFleetWireBodyPostsThroughTheProductionRoute(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := fleetWireRouter(t, repo)
	const base = "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"

	cases := []struct {
		source, providerType, ownerID string
	}{
		{"taxi", "taxi", "taxi:prov_wire"},
		{"dispatch", "dispatch", "dispatch:prov_wire"},
		// Hauling providers read their balance under truck_provider.
		{"hauling", "truck_provider", "truck_provider:prov_wire"},
	}
	for _, c := range cases {
		jobID := "job_wire_" + c.source
		heldJob(t, pool, c.source, jobID, 10000, 1000)
		body := fleetWireBody(c.source, c.providerType, jobID, 9000, 1500, 7500)

		status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, base, body)
		if status != http.StatusOK {
			t.Fatalf("%s: post status=%d body=%v", c.source, status, response)
		}
		data, _ := response["data"].(map[string]any)
		id, _ := data["id"].(string)
		if data["status"] != "POSTED" || id == "" || data["instruction_version"].(float64) != 1 {
			t.Fatalf("%s: response data=%v", c.source, data)
		}
		if got := accountBalanceMinor(t, pool, "provider", c.ownerID, "provider_payable"); got != 1500 {
			t.Fatalf("%s: provider payable=%d, want 1500", c.source, got)
		}

		// A retry of the same body (Fleet after a lost response) returns the
		// original posting.
		status, response = fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, base, body)
		if again, _ := response["data"].(map[string]any); status != http.StatusOK || again["id"] != id {
			t.Fatalf("%s: retry status=%d data=%v", c.source, status, response)
		}

		// Fleet's read-back route.
		status, response = fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodGet, base+"/by-key/settlement:"+jobID, nil)
		if read, _ := response["data"].(map[string]any); status != http.StatusOK || read["id"] != id || read["status"] != "POSTED" {
			t.Fatalf("%s: read back status=%d data=%v", c.source, status, response)
		}
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != len(cases) {
		t.Fatalf("journals=%d, want one per instruction (%d)", got, len(cases))
	}
	assertEveryJournalBalances(t, pool)

	// Fleet's read-back of a key Wallet never saw answers 404, which Fleet
	// treats as "safe to send", never as "posted".
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodGet, base+"/by-key/settlement:never-sent", nil); status != http.StatusNotFound {
		t.Fatalf("unknown key read back status=%d, want 404", status)
	}
	// Only Fleet may instruct a settlement.
	jobID := "job_wire_foreign"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if status, _ := fleetWireCall(t, router, "driver-taxi-service", "taxi-secret", http.MethodPost, base, fleetWireBody("taxi", "taxi", jobID, 9000, 1500, 7500)); status != http.StatusForbidden {
		t.Fatalf("execution service instructing a settlement: status=%d, want 403", status)
	}
	// An unsigned call never reaches the handler.
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", "wrong-secret", http.MethodPost, base, fleetWireBody("taxi", "taxi", jobID, 9000, 1500, 7500)); status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("badly signed call: status=%d", status)
	}
}
