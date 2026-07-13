package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

const testRawKey = "sk_test_abcdefghijklmnopqrstuvwxyz012345"

type fakeKeyRepo struct {
	record *partnerauth.APIKey
}

func (f *fakeKeyRepo) FindByPrefix(_ context.Context, prefix string) (*partnerauth.APIKey, error) {
	if f.record != nil && f.record.KeyPrefix == prefix {
		return f.record, nil
	}
	return nil, nil
}

func (f *fakeKeyRepo) UpdateLastUsed(_ context.Context, _ string) error { return nil }

func newTestRepo(t *testing.T) *fakeKeyRepo {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testRawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword() error = %v", err)
	}
	return &fakeKeyRepo{record: &partnerauth.APIKey{
		ID:          "key-1",
		PartnerID:   "partner-1",
		KeyHash:     string(hash),
		KeyPrefix:   testRawKey[:12],
		Environment: partnerauth.EnvironmentSandbox,
		Status:      partnerauth.APIKeyStatusActive,
		Permissions: partnerauth.AllPermissions(),
	}}
}

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	svc, _ := newTestService()
	return newRouterWithService(t, svc)
}

func requestBody(r *gin.Engine, method, path, body string, withKey bool) *httptest.ResponseRecorder {
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
	if withKey {
		req.Header.Set("X-API-Key", testRawKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBasePathIsPartnerV1(t *testing.T) {
	if PartnerAPIBasePath != "/api/v1/partner/v1" {
		t.Fatalf("PartnerAPIBasePath = %q, want /api/v1/partner/v1", PartnerAPIBasePath)
	}
}

func TestWebhookRoutesReturn501WithValidKey(t *testing.T) {
	r := newTestRouter(t)
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/partner/v1/webhooks"},
		{http.MethodGet, "/api/v1/partner/v1/webhooks"},
		{http.MethodDelete, "/api/v1/partner/v1/webhooks/abc"},
		{http.MethodPost, "/api/v1/partner/v1/webhooks/abc/test"},
	}
	for _, tc := range cases {
		w := requestBody(r, tc.method, tc.path, "", true)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s: status = %d, want 501 (body=%s)", tc.method, tc.path, w.Code, w.Body.String())
		}
		var body partnerauth.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: unmarshal: %v", tc.method, tc.path, err)
		}
		if body.Error.Code != "not_implemented" {
			t.Fatalf("%s %s: code = %q, want not_implemented", tc.method, tc.path, body.Error.Code)
		}
	}
}

func TestImplementedRoutesAreRegistered(t *testing.T) {
	r := newTestRouter(t)
	// GET /deliveries reaches the handler and returns 200 (empty list).
	if w := requestBody(r, http.MethodGet, "/api/v1/partner/v1/deliveries", "", true); w.Code != http.StatusOK {
		t.Fatalf("GET /deliveries status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	// POST /quotes with an invalid body reaches the handler and validates (400, not 404/501).
	if w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", `{}`, true); w.Code != http.StatusBadRequest {
		t.Fatalf("POST /quotes status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestPartnerRouteMissingKeyReturns401(t *testing.T) {
	r := newTestRouter(t)
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", "", false)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	var body partnerauth.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Code != "missing_api_key" {
		t.Fatalf("code = %q, want missing_api_key", body.Error.Code)
	}
}

func TestPartnerRouteWrongBasePathIsNotRegistered(t *testing.T) {
	r := newTestRouter(t)
	w := requestBody(r, http.MethodPost, "/api/v1/partner/quotes", "", true)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for wrong base path", w.Code)
	}
}
