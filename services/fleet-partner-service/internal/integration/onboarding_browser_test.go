//go:build fleet_auth_browser

package integration_test

import (
	"context"
	"encoding/json"
	"net"
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
	server, h := startOnboardingBrowserAPI(t)
	defer server.Close()
	output := runFleetNodeScript(t, "scripts/onboarding-e2e.mjs", "FLEET_AUTH_E2E_API="+server.URL)
	var complete int
	err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_organization_onboarding_stages s JOIN fleet_partner_organizations o ON o.id=s.organization_id WHERE o.status='BUSINESS_SUBMITTED' AND s.stage IN ('OWNER_CONTACT','BUSINESS_PROFILE','HEADQUARTERS','AUTHORIZED_REPRESENTATIVE','OPERATING_CAPABILITIES','BUSINESS_DOCUMENTS') AND s.status='COMPLETE'`).Scan(&complete)
	if err != nil || complete != 6 {
		t.Fatalf("submitted customer's durable activation prerequisites: complete=%d error=%v", complete, err)
	}
	t.Log(string(output))
}

// TestOnboardingDocumentsPerformance measures the documents step against the
// same disposable API and Media authority. It is a measurement, not a gate:
// results are written to .codex-cache/fleet-onboarding-perf/<label>.json.
func TestOnboardingDocumentsPerformance(t *testing.T) {
	if os.Getenv("FLEET_AUTH_BROWSER_TEST") != "1" {
		t.Fatal("FLEET_AUTH_BROWSER_TEST=1 required for this opt-in browser suite")
	}
	server, _ := startOnboardingBrowserAPI(t)
	defer server.Close()
	label := os.Getenv("FLEET_PERF_LABEL")
	if label == "" {
		label = "run"
	}
	output := runFleetNodeScript(t, "scripts/onboarding-perf.mjs", "FLEET_AUTH_E2E_API="+server.URL, "FLEET_PERF_LABEL="+label)
	t.Log(string(output))
}

// startOnboardingBrowserAPI serves the real onboarding router plus the two
// test-only endpoints the browser scripts need (latest OTP, OTP cooldown).
func startOnboardingBrowserAPI(t *testing.T) (*httptest.Server, *authHarness) {
	t.Helper()
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
	// A production Next build fixes its API rewrite target at build time, so
	// the measurement can pin the API to a known address.
	if addr := os.Getenv("FLEET_PERF_API_ADDR"); addr != "" {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewUnstartedServer(mux)
		server.Listener.Close()
		server.Listener = listener
		server.Start()
		return server, h
	}
	return httptest.NewServer(mux), h
}

func runFleetNodeScript(t *testing.T, script string, env ...string) []byte {
	t.Helper()
	dir, err := filepath.Abs("../../../../web/fleet")
	if override := os.Getenv("FLEET_WEB_DIR"); override != "" {
		// Lets a measurement run the same script against another checkout,
		// such as the revision before a change.
		dir = override
	}
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "node", script)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", script, err, output)
	}
	return output
}

// TestOnboardingDocumentsScreenshots captures the documents step for visual
// regression comparison (.codex-cache/fleet-onboarding-screens/<label>).
func TestOnboardingDocumentsScreenshots(t *testing.T) {
	if os.Getenv("FLEET_AUTH_BROWSER_TEST") != "1" {
		t.Fatal("FLEET_AUTH_BROWSER_TEST=1 required for this opt-in browser suite")
	}
	server, _ := startOnboardingBrowserAPI(t)
	defer server.Close()
	output := runFleetNodeScript(t, "scripts/onboarding-docs-screenshots.mjs", "FLEET_AUTH_E2E_API="+server.URL, "FLEET_SHOT_LABEL="+os.Getenv("FLEET_SHOT_LABEL"))
	t.Log(string(output))
}
