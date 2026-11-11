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

func TestAuthV2BrowserWorkflow(t *testing.T) {
	if os.Getenv("FLEET_AUTH_BROWSER_TEST") != "1" {
		t.Skip("Set FLEET_AUTH_BROWSER_TEST=1 with installed web/fleet Playwright browser dependencies")
	}
	router, h := buildAuthRouter(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/__test/otp", func(w http.ResponseWriter, r *http.Request) {
		message, ok := h.sender.LastOTP()
		if !ok {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"code": message.Code})
	})
	mux.HandleFunc("/__test/cooldown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		_, err := h.pool.Exec(r.Context(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`)
		if err != nil {
			w.WriteHeader(500)
		}
	})
	mux.Handle("/", router)
	server := httptest.NewServer(mux)
	defer server.Close()
	dir, err := filepath.Abs("../../../../web/fleet")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "scripts/auth-e2e.mjs")
	command.Dir = dir
	command.Env = append(os.Environ(), "FLEET_AUTH_E2E_API="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Browser workflow failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
