package integration_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
)

// TestAuthStartOriginBoundary pins the exact-origin rule that produced the
// staging 403: only an origin listed in FLEET_PARTNER_AUTH_WEB_ORIGINS may
// start a challenge, and a request with no Origin fails closed.
func TestAuthStartOriginBoundary(t *testing.T) {
	router, h := buildAuthRouter(t)
	body := []byte(`{"email":"origin@example.com","phone":"+2348012345678"}`)

	send := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/auth/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	for _, origin := range []string{"", "https://evil.example", "https://fleet.example.test/", "http://fleet.example.test"} {
		resp := send(origin)
		if resp.Code != http.StatusForbidden {
			t.Fatalf("origin %q: status = %d, want 403; body=%s", origin, resp.Code, resp.Body.String())
		}
		if code := originErrorCode(t, resp); code != "untrusted_origin" {
			t.Fatalf("origin %q: code = %q, want untrusted_origin", origin, code)
		}
		if resp.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("origin %q must not receive CORS grants", origin)
		}
	}
	if h.sender.OTPCount() != 0 {
		t.Fatal("no OTP may be sent for an untrusted origin")
	}

	resp := send("https://fleet.example.test")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("trusted origin status = %d, body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "https://fleet.example.test" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if h.sender.OTPCount() != 1 {
		t.Fatalf("trusted origin should send exactly one OTP, sent %d", h.sender.OTPCount())
	}
}

// TestAuthStartReportsTheFieldAtFault: the sign-up form highlights the field
// the server names, so a bad phone must not be reported as a bad email.
func TestAuthStartReportsTheFieldAtFault(t *testing.T) {
	router, _ := buildAuthRouter(t)
	cases := []struct {
		body  map[string]string
		field string
	}{
		{map[string]string{"email": "field@example.com", "phone": "+12"}, "phone"},
		{map[string]string{"email": "field@example.com"}, "phone"},
		{map[string]string{"email": "nope", "phone": "+2348012345678"}, "email"},
	}
	for _, c := range cases {
		resp := postJSON(router, "/fleet-partner/v1/auth/start", c.body, nil)
		if resp.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%v: status = %d, body=%s", c.body, resp.Code, resp.Body.String())
		}
		env := decodeEnvelope(t, resp)
		errObj, _ := env["error"].(map[string]any)
		details, _ := errObj["details"].([]any)
		if len(details) != 1 {
			t.Fatalf("%v: details = %v", c.body, details)
		}
		if field := details[0].(map[string]any)["field"]; field != c.field {
			t.Fatalf("%v: field = %v, want %s", c.body, field, c.field)
		}
	}
}

func originErrorCode(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	env := decodeEnvelope(t, resp)
	errObj, _ := env["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	return code
}

// TestDuplicateRegistrationNumberIsAConflict: registering a company whose
// registration number already exists must tell the user so (409), not fail
// with an internal error.
func TestDuplicateRegistrationNumberIsAConflict(t *testing.T) {
	pool := testPool(t)
	registration := "RC-DUP-" + strings.ReplaceAll(t.Name(), "/", "")
	createOrganization(t, pool, "First Ltd", registration)

	repo := repositories.NewOrganizationRepository(pool)
	err := repo.Create(context.Background(), pool, &models.Organization{
		LegalName: "Second Ltd", RegistrationNumber: registration,
		CountryCode: "NG", Timezone: "Africa/Lagos", DefaultCurrency: "NGN",
	})
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("duplicate registration err = %v, want a 409 app error", err)
	}
	if appErr.Code != "organization_registration_exists" {
		t.Fatalf("code = %q", appErr.Code)
	}
}
