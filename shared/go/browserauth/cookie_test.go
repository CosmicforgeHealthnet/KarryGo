package browserauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestProductionPolicyFailsClosed(t *testing.T) {
	t.Setenv("TEST_BROWSER_WEB_ORIGINS", "https://partner.example.test")
	p, err := Load("TEST_BROWSER", "production", "refresh", "/auth")
	if err != nil || !p.Secure || p.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe production defaults")
	}
	t.Setenv("TEST_BROWSER_COOKIE_SECURE", "false")
	if _, err = Load("TEST_BROWSER", "production", "refresh", "/auth"); err == nil {
		t.Fatal("insecure production cookie accepted")
	}
	t.Setenv("TEST_BROWSER_COOKIE_SAMESITE", "none")
	if _, err = Load("TEST_BROWSER", "development", "refresh", "/auth"); err == nil {
		t.Fatal("None without Secure accepted")
	}
	t.Setenv("TEST_BROWSER_COOKIE_SECURE", "true")
	t.Setenv("TEST_BROWSER_COOKIE_NAME", "__Host-refresh")
	if _, err = Load("TEST_BROWSER", "production", "refresh", "/auth"); err == nil {
		t.Fatal("auth-scoped Host prefix must not be silently dropped by browsers")
	}
	t.Setenv("TEST_BROWSER_COOKIE_NAME", "refresh")
	for _, origin := range []string{"*", "https://*.example.test", "null", "http://partner.example.test", "https://partner.example.test/path", ""} {
		t.Setenv("TEST_BROWSER_WEB_ORIGINS", origin)
		if _, err = Load("TEST_BROWSER", "production", "refresh", "/auth"); err == nil {
			t.Fatal("invalid production origin accepted")
		}
	}
}

func TestCookieAndDeletionScope(t *testing.T) {
	p := Policy{Name: "refresh", Path: "/auth", Secure: true, SameSite: http.SameSiteLaxMode}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	expiry := time.Now().Add(time.Hour)
	p.Set(c, "opaque-secret", expiry)
	v := w.Result().Cookies()[0]
	if !v.HttpOnly || !v.Secure || v.Domain != "" || v.Path != "/auth" || v.SameSite != http.SameSiteLaxMode || v.MaxAge < 3590 || v.MaxAge > 3600 {
		t.Fatal("invalid issuance cookie")
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	p.Clear(c)
	d := w.Result().Cookies()[0]
	if d.Path != v.Path || d.Domain != v.Domain || d.SameSite != v.SameSite || d.Secure != v.Secure || !d.HttpOnly || d.MaxAge >= 0 || !d.Expires.Before(time.Now()) {
		t.Fatal("invalid deletion scope")
	}
}
