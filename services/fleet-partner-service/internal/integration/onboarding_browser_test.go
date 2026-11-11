//go:build fleet_auth_browser

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOnboardingV2BrowserWorkflow(t *testing.T) {
	if os.Getenv("FLEET_AUTH_BROWSER_TEST") != "1" {
		t.Fatal("FLEET_AUTH_BROWSER_TEST=1 required for this opt-in browser suite")
	}
	router, h := buildOnboardingRouter(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/__test/otp", func(w http.ResponseWriter, r *http.Request) {
		message, ok := h.sender.LastOTP()
		if !ok {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"code": message.Code})
	})
	mux.HandleFunc("/__test/cooldown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		h.pool.Exec(r.Context(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`)
	})
	mux.Handle("/", router)
	server := httptest.NewServer(mux)
	defer server.Close()
	dir, err := filepath.Abs("../../../../web/fleet")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "scripts/onboarding-e2e.mjs")
	command.Dir = dir
	command.Env = append(os.Environ(), "FLEET_AUTH_E2E_API="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Onboarding browser workflow: %v\n%s", err, output)
	}
	var complete int
	err = h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_organization_onboarding_stages s JOIN fleet_partner_organizations o ON o.id=s.organization_id WHERE o.status='BUSINESS_SUBMITTED' AND s.stage IN ('OWNER_CONTACT','BUSINESS_PROFILE','HEADQUARTERS','AUTHORIZED_REPRESENTATIVE','OPERATING_CAPABILITIES','BUSINESS_DOCUMENTS') AND s.status='COMPLETE'`).Scan(&complete)
	if err != nil || complete != 6 {
		t.Fatalf("submitted customer's durable activation prerequisites: complete=%d error=%v", complete, err)
	}
	t.Log(string(output))
}
