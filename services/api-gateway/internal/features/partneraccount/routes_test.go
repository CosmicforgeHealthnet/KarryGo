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

func TestHTTPSignupOTPFlow(t *testing.T) {
	r, _ := newTestRouter()

	w := do(r, http.MethodPost, "/api/v1/partner/v1/auth/signup/start", `{"business_email":" Dev@PharmaCare.com "}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("signup start status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "042391") || strings.Contains(w.Body.String(), hashOTPCode("042391")) {
		t.Fatalf("signup start leaked OTP material: %s", w.Body.String())
	}

	w = do(r, http.MethodPost, "/api/v1/partner/v1/auth/signup/verify", `{"business_email":"dev@pharmacare.com","otp_code":"042391"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("signup verify status = %d, want 201 (body=%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") || strings.Contains(w.Body.String(), "api_key") {
		t.Fatalf("signup verify leaked forbidden fields: %s", w.Body.String())
	}
	var verify struct {
		Data struct {
			Token   string `json:"token"`
			Account struct {
				ID            string  `json:"id"`
				BusinessEmail string  `json:"business_email"`
				BusinessName  *string `json:"business_name"`
				Status        string  `json:"status"`
				Plan          string  `json:"plan"`
			} `json:"account"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &verify); err != nil {
		t.Fatalf("unmarshal verify: %v", err)
	}
	if verify.Data.Token == "" || verify.Data.Account.BusinessName != nil || verify.Data.Account.Status != StatusActive || verify.Data.Account.Plan != PlanSandbox {
		t.Fatalf("bad verify payload: %s", w.Body.String())
	}
	auth := map[string]string{"Authorization": "Bearer " + verify.Data.Token}
	if w := do(r, http.MethodGet, "/api/v1/partner/v1/auth/me", "", auth); w.Code != http.StatusOK {
		t.Fatalf("/me with OTP session = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	w = do(r, http.MethodPost, "/api/v1/partner/v1/auth/login", `{"email":"dev@pharmacare.com","password":"supersecret"}`, nil)
	if w.Code != http.StatusUnauthorized || errCode(t, w.Body.Bytes()) != "invalid_credentials" {
		t.Fatalf("password login for OTP account = %d/%s", w.Code, w.Body.String())
	}
}

func TestHTTPSignupResendTooSoonIncludesSecondsRemaining(t *testing.T) {
	r, _ := newTestRouter()
	if w := do(r, http.MethodPost, "/api/v1/partner/v1/auth/signup/start", `{"business_email":"dev@pharmacare.com"}`, nil); w.Code != http.StatusOK {
		t.Fatalf("signup start = %d (body=%s)", w.Code, w.Body.String())
	}
	w := do(r, http.MethodPost, "/api/v1/partner/v1/auth/signup/resend", `{"business_email":"dev@pharmacare.com"}`, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("resend status = %d, want 429 (body=%s)", w.Code, w.Body.String())
	}
	var env struct {
		Error struct {
			Code             string `json:"code"`
			SecondsRemaining int    `json:"seconds_remaining"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error.Code != "resend_too_soon" || env.Error.SecondsRemaining <= 0 {
		t.Fatalf("bad resend error: %s", w.Body.String())
	}
}

func TestHTTPProfilePartialUpdatesIncludePhotoURLs(t *testing.T) {
	r, h := newTestRouter()
	res := h.mustRegister(t)
	auth := map[string]string{"Authorization": "Bearer " + res.Token}

	w := do(r, http.MethodPatch, "/api/v1/partner/v1/profile", `{"business_name":"PharmaCare Nigeria Ltd","business_phone":"+2348033445566","state":"Lagos","city":"Ikeja"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("profile business info status = %d (body=%s)", w.Code, w.Body.String())
	}
	w = do(r, http.MethodPatch, "/api/v1/partner/v1/profile", `{"business_logo_url":"https://storage.example/logo.jpg"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("profile logo status = %d (body=%s)", w.Code, w.Body.String())
	}
	w = do(r, http.MethodPatch, "/api/v1/partner/v1/profile", `{"profile_photo_url":"https://storage.example/photo.jpg"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("profile photo status = %d (body=%s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "password_hash") {
		t.Fatalf("profile leaked password hash: %s", w.Body.String())
	}
	var env struct {
		Data struct {
			Account Account `json:"account"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if env.Data.Account.BusinessName == nil || *env.Data.Account.BusinessName != "PharmaCare Nigeria Ltd" {
		t.Fatalf("business_name not retained: %+v", env.Data.Account)
	}
	if env.Data.Account.BusinessLogoURL == nil || *env.Data.Account.BusinessLogoURL != "https://storage.example/logo.jpg" {
		t.Fatalf("logo not retained: %+v", env.Data.Account)
	}
	if env.Data.Account.ProfilePhotoURL == nil || *env.Data.Account.ProfilePhotoURL != "https://storage.example/photo.jpg" {
		t.Fatalf("profile photo not updated: %+v", env.Data.Account)
	}
}
