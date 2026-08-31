package partneraccount

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBrowserOpenAPIAuthRoutesAndNoRefreshJSON(t *testing.T) {
	data, err := os.ReadFile("../../../docs/partner-browser-authentication.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	r, _ := newTestRouter()
	actual := map[string]bool{}
	for _, route := range r.Routes() {
		if route.Method == "POST" && strings.HasPrefix(route.Path, "/api/v1/partner/v1/auth/") {
			actual[route.Path] = true
		}
	}
	if len(actual) != len(doc.Paths) {
		t.Fatalf("auth route count %d vs %d", len(actual), len(doc.Paths))
	}
	for path := range doc.Paths {
		if !actual[path] {
			t.Fatalf("unregistered auth contract %s", path)
		}
	}
	if strings.Contains(string(data), `"refreshToken"`) || strings.Contains(string(data), `"refresh_token"`) {
		t.Fatal("browser JSON refresh schema present")
	}
}

func assertBrowserSession(t *testing.T, w *httptest.ResponseRecorder) (*http.Cookie, string) {
	t.Helper()
	if w.Code != 200 && w.Code != 201 {
		t.Fatalf("session status %d", w.Code)
	}
	cs := w.Result().Cookies()
	if len(cs) != 1 {
		t.Fatal("missing single cookie")
	}
	c := cs[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Domain != "" || c.Path != "/api/v1/partner/v1/auth" || c.MaxAge <= 0 || !c.Expires.After(time.Now()) || len(c.Value) != 124 {
		t.Fatal("unsafe refresh cookie")
	}
	if strings.Contains(w.Body.String(), c.Value) || strings.Contains(w.Body.String(), "refreshToken") || strings.Contains(w.Body.String(), "refresh_token") {
		t.Fatal("refresh secret exposed in JSON")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("token response cacheable")
	}
	var body struct {
		Data struct {
			Token string `json:"token"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Data.Token == "" {
		t.Fatal("access token missing")
	}
	return c, body.Data.Token
}

func TestBrowserPasswordRefreshRotationLogout(t *testing.T) {
	r, h := newTestRouter()
	h.mustRegister(t)
	w := do(r, "POST", "/api/v1/partner/v1/auth/login", `{"email":"dev@pharmacare.com","password":"supersecret"}`, nil)
	old, access := assertBrowserSession(t, w)
	headers := map[string]string{"Cookie": old.Name + "=" + old.Value}
	w = do(r, "POST", "/api/v1/partner/v1/auth/refresh", "", headers)
	next, nextAccess := assertBrowserSession(t, w)
	if next.Value == old.Value || access == nextAccess {
		t.Fatal("credentials not rotated")
	}
	if do(r, "POST", "/api/v1/partner/v1/auth/refresh", "", headers).Code != 401 {
		t.Fatal("old credential replay accepted")
	}
	if do(r, "GET", "/api/v1/partner/v1/auth/me", "", map[string]string{"Authorization": "Bearer " + nextAccess}).Code != 200 {
		t.Fatal("refreshed access rejected")
	}
	headers["Cookie"] = next.Name + "=" + next.Value
	for i := 0; i < 2; i++ {
		out := do(r, "POST", "/api/v1/partner/v1/auth/logout", "", headers)
		cs := out.Result().Cookies()
		if out.Code != 200 || len(cs) != 1 || cs[0].MaxAge >= 0 || cs[0].Path != next.Path || cs[0].Domain != "" || !cs[0].HttpOnly || !cs[0].Secure || cs[0].SameSite != next.SameSite || !cs[0].Expires.Before(time.Now()) {
			t.Fatal("logout/deletion failed")
		}
	}
	if do(r, "POST", "/api/v1/partner/v1/auth/refresh", "", headers).Code != 401 {
		t.Fatal("revoked session accepted")
	}
	if do(r, "POST", "/api/v1/partner/v1/auth/logout", "", nil).Code != 200 {
		t.Fatal("missing-cookie logout not idempotent")
	}
	for _, cookie := range []string{"", "karrygo_partner_refresh=invalid"} {
		if do(r, "POST", "/api/v1/partner/v1/auth/refresh", `{"refreshToken":"ignored"}`, map[string]string{"Cookie": cookie}).Code != 401 {
			t.Fatal("missing/invalid cookie accepted")
		}
	}
}

func TestBrowserOTPLoginAndSignupSetCookie(t *testing.T) {
	for _, kind := range []string{"signup", "login"} {
		t.Run(kind, func(t *testing.T) {
			r, h := newTestRouter()
			if kind == "login" {
				h.mustRegister(t)
			}
			path := "/api/v1/partner/v1/auth/" + kind
			out := do(r, "POST", path+"/start", `{"business_email":"dev@pharmacare.com"}`, nil)
			if out.Code != 200 {
				t.Fatalf("start status %d", out.Code)
			}
			out = do(r, "POST", path+"/verify", `{"business_email":"dev@pharmacare.com","otp_code":"042391"}`, nil)
			assertBrowserSession(t, out)
		})
	}
}

func TestBrowserRefreshExpiryRevocationAndRace(t *testing.T) {
	h := newHarness()
	session := h.mustRegister(t)
	ctx := context.Background()
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.svc.Refresh(ctx, session.RefreshToken, RequestMeta{}); err == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("refresh winners %d", winners.Load())
	}
	for _, expired := range []bool{true, false} {
		login, err := h.svc.Login(ctx, LoginInput{Email: "dev@pharmacare.com", Password: "supersecret"}, RequestMeta{})
		if err != nil {
			t.Fatal(err)
		}
		h.sessions.mu.Lock()
		s := h.sessions.byHash[hashToken(login.Token)]
		past := time.Now().Add(-time.Hour)
		if expired {
			s.ExpiresAt = past
		} else {
			s.RevokedAt = &past
		}
		h.sessions.mu.Unlock()
		if _, err = h.svc.Refresh(ctx, login.RefreshToken, RequestMeta{}); err == nil {
			t.Fatal("expired/revoked refresh accepted")
		}
	}
}

func TestBrowserTrustedCORSAndHostileOrigin(t *testing.T) {
	r, _ := newTestRouter()
	w := do(r, "OPTIONS", "/api/v1/partner/v1/auth/refresh", "", map[string]string{"Access-Control-Request-Method": "POST"})
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("trusted preflight failed")
	}
	for _, path := range []string{"refresh", "logout", "login"} {
		for _, origin := range []string{"https://hostile.test", "null", ""} {
			w = do(r, "POST", "/api/v1/partner/v1/auth/"+path, "{}", map[string]string{"Origin": origin})
			if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("untrusted origin authorized")
			}
		}
	}
}
