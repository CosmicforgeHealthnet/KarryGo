package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/browserauth"
	"github.com/gin-gonic/gin"
)

func TestBrowserOpenAPIAuthRouteParity(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/openapi/browser-authentication.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewHandler(nil, nil)
	h.RegisterPublicRoutes(r.Group("/fleet-partner/v1"))
	h.RegisterAuthenticatedRoutes(r.Group("/fleet-partner/v1"))
	actual := map[string]bool{}
	for _, route := range r.Routes() {
		if route.Method == "POST" && strings.HasPrefix(route.Path, "/fleet-partner/v1/auth/") {
			actual[route.Path] = true
		}
	}
	if len(actual) != len(doc.Paths) {
		t.Fatalf("auth operation count %d vs %d", len(actual), len(doc.Paths))
	}
	for path := range doc.Paths {
		if !actual[path] {
			t.Fatalf("unregistered auth contract %s", path)
		}
	}
	if strings.Contains(string(raw), `"refreshToken"`) || strings.Contains(string(raw), `"refresh_token"`) {
		t.Fatal("refresh credential in JSON schema")
	}
}

func TestSessionSerializationNeverExposesRefreshCredential(t *testing.T) {
	pair := &authusecases.TokenPair{AccessToken: "access", RefreshToken: "never-browser-readable", RefreshExpiresAt: time.Now().Add(time.Hour)}
	for _, body := range []any{pair, verifyResponse(authusecases.VerifyResult{Tokens: pair})} {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), pair.RefreshToken) || strings.Contains(strings.ToLower(string(data)), "refreshtoken") || strings.Contains(string(data), "refresh_token") {
			t.Fatal("refresh credential serialized")
		}
	}
}

func TestAuthCookieBoundaryRejectsHostileOriginBeforeUsecase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := browserauth.Policy{Name: "karrygo_fleet_refresh", Path: "/fleet-partner/v1/auth", Secure: true, SameSite: http.SameSiteLaxMode, Origins: []string{"https://fleet.example.test"}}
	h := NewHandler(nil, nil, p)
	r := gin.New()
	h.RegisterPublicRoutes(r.Group("/fleet-partner/v1"))
	for _, path := range []string{"refresh", "logout", "verify"} {
		req := httptest.NewRequest("POST", p.Path+"/"+path, nil)
		req.Header.Set("Origin", "https://hostile.example")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("hostile origin reached usecase")
		}
	}
	req := httptest.NewRequest("OPTIONS", p.Path+"/refresh", nil)
	req.Header.Set("Origin", p.Origins[0])
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != p.Origins[0] || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("preflight failed")
	}
}
