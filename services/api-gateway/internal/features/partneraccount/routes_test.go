package partneraccount

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testInternalKey = "internal-service-key-value"

func newTestRouter() (*gin.Engine, *harness) {
	h := newHarness()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAccountRoutes(r, Dependencies{Service: h.svc, InternalServiceKey: testInternalKey})
	return r, h
}

func do(r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHTTPRegisterLoginMeLogoutFlow(t *testing.T) {
	r, _ := newTestRouter()

	// Register
	w := do(r, http.MethodPost, "/api/v1/partner/v1/auth/register",
		`{"business_name":"PharmaCare","business_email":"dev@pharmacare.com","password":"supersecret"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	// Must NOT leak password_hash or key_hash.
	if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "key_hash") {
		t.Fatalf("register response leaked a hash: %s", w.Body.String())
	}
	var reg struct {
		Data struct {
			Token   string `json:"token"`
			Account struct {
				ID string `json:"id"`
			} `json:"account"`
			APIKey struct {
				Key string `json:"key"`
			} `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if reg.Data.Token == "" || !strings.HasPrefix(reg.Data.APIKey.Key, "sk_test_") {
		t.Fatalf("register missing token/api_key: %s", w.Body.String())
	}

	auth := map[string]string{"Authorization": "Bearer " + reg.Data.Token}

	// /me works with the session token
	if w := do(r, http.MethodGet, "/api/v1/partner/v1/auth/me", "", auth); w.Code != http.StatusOK {
		t.Fatalf("/me status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	// logout revokes the session
	if w := do(r, http.MethodPost, "/api/v1/partner/v1/auth/logout", "", auth); w.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", w.Code)
	}

	// /me now fails with the same token
	w = do(r, http.MethodGet, "/api/v1/partner/v1/auth/me", "", auth)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("/me after logout status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	if code := errCode(t, w.Body.Bytes()); code != "session_revoked" {
		t.Fatalf("code = %q, want session_revoked", code)
	}
}

func TestHTTPMeRequiresToken(t *testing.T) {
	r, _ := newTestRouter()
	w := do(r, http.MethodGet, "/api/v1/partner/v1/auth/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if code := errCode(t, w.Body.Bytes()); code != "missing_token" {
		t.Fatalf("code = %q, want missing_token", code)
	}
}

func TestHTTPMeRejectsGarbageToken(t *testing.T) {
	r, _ := newTestRouter()
	w := do(r, http.MethodGet, "/api/v1/partner/v1/auth/me", "",
		map[string]string{"Authorization": "Bearer not.a.jwt"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if code := errCode(t, w.Body.Bytes()); code != "invalid_token" {
		t.Fatalf("code = %q, want invalid_token", code)
	}
}

func TestHTTPInternalRequiresServiceKey(t *testing.T) {
	r, h := newTestRouter()
	res := h.mustRegister(t)
	path := "/api/v1/internal/partner/accounts/" + res.Account.ID + "/activate-live"

	// missing service key -> 401
	if w := do(r, http.MethodPost, path, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no service key status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	// valid service key -> 200 and plan upgraded
	w := do(r, http.MethodPost, path, "", map[string]string{"X-Internal-Service-Key": testInternalKey})
	if w.Code != http.StatusOK {
		t.Fatalf("activate-live status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if h.accounts.byID[res.Account.ID].Plan != PlanLive {
		t.Fatal("plan not upgraded to live via internal endpoint")
	}
}

func TestHTTPInternalUnknownPartner(t *testing.T) {
	r, _ := newTestRouter()
	w := do(r, http.MethodPost, "/api/v1/internal/partner/accounts/acct-999/activate-live", "",
		map[string]string{"X-Internal-Service-Key": testInternalKey})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
	if code := errCode(t, w.Body.Bytes()); code != "partner_not_found" {
		t.Fatalf("code = %q, want partner_not_found", code)
	}
}

func TestHTTPKeysRequireSession(t *testing.T) {
	r, _ := newTestRouter()
	// No session token -> 401 missing_token.
	if w := do(r, http.MethodGet, "/api/v1/partner/v1/keys", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /keys without session = %d, want 401", w.Code)
	}
}

func errCode(t *testing.T, raw []byte) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal error envelope: %v (%s)", err, raw)
	}
	return env.Error.Code
}
