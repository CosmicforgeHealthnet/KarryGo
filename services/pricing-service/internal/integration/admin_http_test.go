package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	approvalrepos "cosmicforge/logistics/services/pricing-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/pricing-service/internal/features/approvals/usecases"
	policyhttp "cosmicforge/logistics/services/pricing-service/internal/features/policies/http"
	policyrepos "cosmicforge/logistics/services/pricing-service/internal/features/policies/repositories"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	rolloutrepos "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/repositories"
	"cosmicforge/logistics/services/pricing-service/internal/platform/audit"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	testAdminCaller = "admin-backoffice-service"
	testAdminSecret = "test-admin-secret"
)

// adminServer builds the real router the way cmd/main.go does, so these tests
// exercise the production wiring including signing, replay and caller checks.
type adminServer struct {
	server   *httptest.Server
	editor   string
	approver string
}

func newAdminServer(t *testing.T) adminServer {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	editor := unique("http-editor")
	approver := unique("http-approver")
	for id, roles := range map[string]string{
		editor:   "{EDITOR}",
		approver: "{APPROVER}",
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO pricing_operators (operator_id, roles) VALUES ($1, $2::text[])`, id, roles); err != nil {
			t.Fatalf("seed operator: %v", err)
		}
	}

	policyService := policyusecases.NewService(policyusecases.Options{
		Pool:  pool,
		Store: func(q database.Querier) policyusecases.Store { return policyrepos.NewStore(q) },
		Approvals: func(q database.Querier) policyusecases.Approvals {
			return approvalusecases.NewService(approvalrepos.NewStore(q))
		},
		Ops:             operators.NewStore(pool),
		Audit:           audit.NewPostgresRecorder(),
		FourEyeRequired: true,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	policyhttp.RegisterAdminRoutes(router, policyhttp.AdminOptions{
		Handler: policyhttp.NewHandler(policyService,
			approvalusecases.NewService(approvalrepos.NewStore(pool)),
			rolloutrepos.NewStore(pool)),
		Verifier:     serviceauth.NewVerifier(serviceauth.Secrets{testAdminCaller: []byte(testAdminSecret)}, 5*time.Minute),
		NonceStore:   pool,
		AdminCallers: []string{testAdminCaller},
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return adminServer{server: server, editor: editor, approver: approver}
}

// do signs and sends a request, returning the status and decoded body.
func (a adminServer) do(t *testing.T, method, path string, payload any, sign func(*http.Request)) (int, map[string]any) {
	t.Helper()

	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		body = encoded
	}

	request, err := http.NewRequest(method, a.server.URL+"/internal/pricing/v1/admin"+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if sign == nil {
		if err := serviceauth.SignRequest(request, testAdminCaller, []byte(testAdminSecret), body, time.Now()); err != nil {
			t.Fatalf("sign: %v", err)
		}
	} else {
		sign(request)
	}

	response, err := a.server.Client().Do(request)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	defer response.Body.Close()

	var decoded map[string]any
	_ = json.NewDecoder(response.Body).Decode(&decoded)
	return response.StatusCode, decoded
}

func seedPayload(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "seeds", "v1", name))
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	var seed map[string]any
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	return seed
}

func TestAdminAPIRefusesUnsignedAndForeignCallers(t *testing.T) {
	admin := newAdminServer(t)

	t.Run("unsigned", func(t *testing.T) {
		status, _ := admin.do(t, "GET", "/approvals", nil, func(*http.Request) {})
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", status)
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		status, _ := admin.do(t, "GET", "/approvals", nil, func(r *http.Request) {
			if err := serviceauth.SignRequest(r, testAdminCaller, []byte("not-the-secret"), nil, time.Now()); err != nil {
				t.Fatalf("sign: %v", err)
			}
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", status)
		}
	})

	t.Run("a service with no admin authority", func(t *testing.T) {
		// A booking service may hold a pricing secret to ask for quotes. That must
		// never let it change a rate.
		status, _ := admin.do(t, "GET", "/approvals", nil, func(r *http.Request) {
			if err := serviceauth.SignRequest(r, "taxi-service", []byte(testAdminSecret), nil, time.Now()); err != nil {
				t.Fatalf("sign: %v", err)
			}
		})
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatalf("status = %d, want 401 or 403", status)
		}
	})

	t.Run("a replayed request", func(t *testing.T) {
		request, err := http.NewRequest("GET", admin.server.URL+"/internal/pricing/v1/admin/approvals", nil)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if err := serviceauth.SignRequest(request, testAdminCaller, []byte(testAdminSecret), nil, time.Now()); err != nil {
			t.Fatalf("sign: %v", err)
		}

		first, err := admin.server.Client().Do(request)
		if err != nil {
			t.Fatalf("first send: %v", err)
		}
		first.Body.Close()
		if first.StatusCode != http.StatusOK {
			t.Fatalf("first status = %d, want 200", first.StatusCode)
		}

		// Replaying the identical signed request, nonce included.
		replay, err := http.NewRequest("GET", request.URL.String(), nil)
		if err != nil {
			t.Fatalf("build replay: %v", err)
		}
		for key, values := range request.Header {
			replay.Header[key] = values
		}
		second, err := admin.server.Client().Do(replay)
		if err != nil {
			t.Fatalf("replay send: %v", err)
		}
		defer second.Body.Close()
		if second.StatusCode != http.StatusUnauthorized {
			t.Fatalf("replay status = %d, want 401: a signed request must be single-use", second.StatusCode)
		}
	})
}

// TestAdminAPIWalksTheApprovalJourney is the HTTP-level version of the lifecycle
// test: it proves an operator using the real API cannot shortcut approval.
func TestAdminAPIWalksTheApprovalJourney(t *testing.T) {
	admin := newAdminServer(t)

	// A throwaway state so this test owns its own policy lineage.
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO pricing_countries (code, name, currency) VALUES ('ZZ', 'Test Country', 'NGN') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed country: %v", err)
	}
	stateID := unique("st")
	if _, err := pool.Exec(ctx,
		`INSERT INTO pricing_states (id, country_code, code, name) VALUES ($1, 'ZZ', $2, 'Test State')`,
		stateID, unique("ST")); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	seed := seedPayload(t, "taxi_tariff_ng.json")
	draft := map[string]any{
		"kind":            seed["kind"],
		"geography_level": "STATE",
		"geography_ref":   stateID,
		"schema_version":  seed["schema_version"],
		"body":            seed["body"],
		"performed_by":    admin.editor,
		"reason":          "V1 price-neutral baseline",
	}

	status, body := admin.do(t, "POST", "/policies/drafts", draft, nil)
	if status != http.StatusCreated {
		t.Fatalf("create draft status = %d, body = %v", status, body)
	}
	data := body["data"].(map[string]any)
	versionID := data["id"].(string)
	if data["status"] != "DRAFT" {
		t.Fatalf("new version status = %v, want DRAFT", data["status"])
	}
	bodyHash := data["body_hash"].(string)

	t.Run("a draft cannot be activated", func(t *testing.T) {
		status, _ := admin.do(t, "POST", "/policy-versions/"+versionID+"/activate",
			map[string]any{"performed_by": admin.approver, "reason": "skip the queue"}, nil)
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409", status)
		}
	})

	t.Run("an invalid body is refused at draft time", func(t *testing.T) {
		bad := map[string]any{}
		for key, value := range draft {
			bad[key] = value
		}
		bad["body"] = map[string]any{"schema_version": 1, "currency": "NGN", "classes": map[string]any{},
			"distance_basis": "TELEPORT", "distance_rounding": "TRUNCATE",
			"duration_unit": "WHOLE_MINUTES_FLOOR", "time_basis": "ROUTE_DURATION",
			"total_rounding":         map[string]any{"step_kobo": 50, "mode": "HALF_UP"},
			"minimum_after_rounding": true, "low_confidence_route": "ALLOW_WITH_FLAG",
			"hourly_mode": "PRICE_AS_POINT_TO_POINT"}
		status, response := admin.do(t, "POST", "/policies/drafts", bad, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %v", status, response)
		}
	})

	t.Run("submitting opens an approval", func(t *testing.T) {
		status, response := admin.do(t, "POST", "/policy-versions/"+versionID+"/submit",
			map[string]any{"performed_by": admin.editor, "reason": "ready for review"}, nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %v", status, response)
		}
		if response["data"].(map[string]any)["approval_request_id"] == "" {
			t.Fatal("no approval request id returned")
		}

		queueStatus, queue := admin.do(t, "GET", "/approvals", nil, nil)
		if queueStatus != http.StatusOK {
			t.Fatalf("approvals status = %d", queueStatus)
		}
		pending := queue["data"].(map[string]any)["pending"]
		if pending == nil {
			t.Fatal("the review queue is empty after submitting")
		}
	})

	t.Run("the submitter cannot approve their own change", func(t *testing.T) {
		// Grant the editor the approver role so only four-eye stands in the way.
		if _, err := pool.Exec(ctx,
			`UPDATE pricing_operators SET roles = '{EDITOR,APPROVER}' WHERE operator_id = $1`, admin.editor); err != nil {
			t.Fatalf("grant role: %v", err)
		}
		status, response := admin.do(t, "POST", "/policy-versions/"+versionID+"/approve",
			map[string]any{"performed_by": admin.editor, "reason": "my own work"}, nil)
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %v", status, response)
		}
		message := response["error"].(map[string]any)["message"].(string)
		if message == "" {
			t.Fatal("a refusal must carry a readable message")
		}
	})

	t.Run("a second operator approves and activates", func(t *testing.T) {
		status, response := admin.do(t, "POST", "/policy-versions/"+versionID+"/approve",
			map[string]any{"performed_by": admin.approver, "reason": "verified against the golden corpus"}, nil)
		if status != http.StatusOK {
			t.Fatalf("approve status = %d, body = %v", status, response)
		}

		status, response = admin.do(t, "POST", "/policy-versions/"+versionID+"/activate",
			map[string]any{"performed_by": admin.approver, "reason": "go live"}, nil)
		if status != http.StatusOK {
			t.Fatalf("activate status = %d, body = %v", status, response)
		}
	})

	t.Run("resolve now returns that version", func(t *testing.T) {
		status, response := admin.do(t, "GET", "/resolve?kind=taxi.tariff&state_id="+stateID+"&country_code=ZZ", nil, nil)
		if status != http.StatusOK {
			t.Fatalf("resolve status = %d, body = %v", status, response)
		}
		resolved := response["data"].(map[string]any)["version"].(map[string]any)
		if resolved["id"] != versionID {
			t.Fatalf("resolved %v, want %s", resolved["id"], versionID)
		}
		if resolved["body_hash"] != bodyHash {
			t.Fatal("the resolved body hash differs from the approved one")
		}
	})

	t.Run("simulate prices against the version without storing anything", func(t *testing.T) {
		status, response := admin.do(t, "POST", "/simulate", map[string]any{
			"version_id": versionID, "scope": "TAXI", "channel": "CUSTOMER_APP",
			"class": "economy", "distance_meters": 8000, "duration_seconds": 1230,
		}, nil)
		if status != http.StatusOK {
			t.Fatalf("simulate status = %d, body = %v", status, response)
		}
		data := response["data"].(map[string]any)
		result := data["result"].(map[string]any)
		if gross := result["gross_kobo"].(float64); gross != 170_000 {
			t.Fatalf("simulated gross = %v, want 170000", gross)
		}
		// The response must say where the commission came from, so a zero fee can
		// never be mistaken for a real one.
		if data["commission_source"] == "" || data["commission_source"] == nil {
			t.Fatal("simulate must state which commission it used")
		}
		if explanation := result["explanation"]; explanation == nil {
			t.Fatal("simulate must return the derivation in words")
		}
	})

	t.Run("an approved version can no longer be edited", func(t *testing.T) {
		status, _ := admin.do(t, "PUT", "/policy-versions/"+versionID,
			map[string]any{"performed_by": admin.editor, "reason": "sneak a change in", "body": seed["body"]}, nil)
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409", status)
		}
	})
}

func TestAdminAPIManagesRolloutsAndFreezes(t *testing.T) {
	admin := newAdminServer(t)
	marketCode := unique("HTTPMKT")

	status, response := admin.do(t, "POST", "/rollouts", map[string]any{
		"scope": "TAXI", "capability": "QUOTE", "market_code": marketCode, "mode": "SHADOW",
		"performed_by": admin.approver, "reason": "begin shadow comparison",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("rollout status = %d, body = %v", status, response)
	}

	t.Run("an unknown mode is refused", func(t *testing.T) {
		status, _ := admin.do(t, "POST", "/rollouts", map[string]any{
			"scope": "TAXI", "capability": "QUOTE", "market_code": marketCode, "mode": "YOLO",
			"performed_by": admin.approver, "reason": "typo",
		}, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})

	t.Run("freeze and unfreeze are recorded", func(t *testing.T) {
		status, _ := admin.do(t, "POST", "/markets/"+marketCode+"/freeze",
			map[string]any{"performed_by": admin.approver, "reason": "investigating a fare spike"}, nil)
		if status != http.StatusOK {
			t.Fatalf("freeze status = %d", status)
		}

		status, response := admin.do(t, "GET", "/markets/"+marketCode+"/freeze", nil, nil)
		if status != http.StatusOK {
			t.Fatalf("freeze state status = %d", status)
		}
		if frozen := response["data"].(map[string]any)["frozen"].(bool); !frozen {
			t.Fatal("the market should report as frozen")
		}

		status, _ = admin.do(t, "POST", "/markets/"+marketCode+"/unfreeze",
			map[string]any{"performed_by": admin.approver, "reason": "resolved"}, nil)
		if status != http.StatusOK {
			t.Fatalf("unfreeze status = %d", status)
		}
	})

	t.Run("a freeze without a reason is refused", func(t *testing.T) {
		status, _ := admin.do(t, "POST", "/markets/"+marketCode+"/freeze",
			map[string]any{"performed_by": admin.approver}, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}
