package mediauploadhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/customer-service/internal/config"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type allowSessions struct{}

func (allowSessions) Allow(context.Context, string, string, int, time.Duration) error { return nil }

type denySessions struct{}

func (denySessions) Allow(context.Context, string, string, int, time.Duration) error {
	return apperrors.RateLimited("Too many requests.", errors.New("limit"))
}

func TestCustomerMediaSessionRequiresOwnedComplaintAndSignsPrivateGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var supportCalls, mediaCalls int
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supportCalls++
		if r.URL.Path != "/api/v1/support-disputes/complaints/complaint-1" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("unscoped Support read: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"complaint-1","complainant_id":"customer-1","complainant_type":"customer"}}`))
	}))
	defer owner.Close()
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaCalls++
		if r.Header.Get("X-Service-Name") != "customer-service" || r.Header.Get("X-Service-Signature-Version") != "2" ||
			r.Header.Get("X-Service-Nonce") == "" || r.Header.Get("X-Organization-ID") != "customer_customer-1" {
			t.Error("private Media request lacks scoped HMAC identity")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["ownerReference"] != "customer-1" || body["bindingId"] != "complaint-1" || body["uploadPurpose"] != "DISPUTE_ATTACHMENT" {
			t.Errorf("unscoped grant: %v", body)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"uploadAuthorizationId":"auth-1","mediaObjectId":"media-1","uploadToken":"short-lived","expiresAt":"2030-01-01T12:00:00Z","maxSizeBytes":1024,"allowedMimeTypes":["image/png"]}}`))
	}))
	defer media.Close()
	signer := sharedauth.NewTokenSigner([]byte("customer-secret"))
	token, err := signer.Sign(sharedauth.Claims{Subject: "customer-1", Role: "customer", Service: "customer", SessionID: "session-1", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	Register(engine.Group("/api/v1/customer"), signer, Handler{Config: config.Config{MediaUploadSessionEnabled: true,
		SupportBaseURL: owner.URL, MediaBaseURL: media.URL, MediaPublicBaseURL: media.URL, MediaPrivateSecret: []byte("media-secret"),
	}, Limiter: allowSessions{}})
	call := func(payload string, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/customer/media-upload-sessions", bytes.NewBufferString(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	good := `{"purpose":"DISPUTE_ATTACHMENT","source_authority":"SUPPORT_DISPUTE","source_ref":"complaint-1","mime_type":"image/png","byte_size":1024}`
	if got := call(good, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: %d", got.Code)
	}
	if got := call(strings.Replace(good, "complaint-1", "../other", 1), token); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid ref: %d", got.Code)
	}
	if got := call(good, token); got.Code != http.StatusCreated || !strings.Contains(got.Body.String(), "customer-uploads/media-1/content") {
		t.Fatalf("grant: %d %s", got.Code, got.Body.String())
	}
	if supportCalls != 1 || mediaCalls != 1 {
		t.Fatalf("calls support=%d media=%d", supportCalls, mediaCalls)
	}
}

func TestCustomerMediaSessionRateLimitPrecedesSupportAndMediaCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	signer := sharedauth.NewTokenSigner([]byte("secret"))
	token, _ := signer.Sign(sharedauth.Claims{Subject: "customer-1", Role: "customer", Service: "customer", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	Register(engine.Group("/api/v1/customer"), signer, Handler{Config: config.Config{MediaUploadSessionEnabled: true,
		SupportBaseURL: "http://unreachable.invalid", MediaBaseURL: "http://unreachable.invalid", MediaPublicBaseURL: "https://media.example", MediaPrivateSecret: []byte("secret"),
	}, Limiter: denySessions{}})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/customer/media-upload-sessions", strings.NewReader(`{"purpose":"DISPUTE_ATTACHMENT"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", w.Code)
	}
}

func TestCustomerMediaSessionOpenAPIPathParity(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/openapi/customer-evidence-upload.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Paths) != 1 || document.Paths["/api/v1/customer/media-upload-sessions"] == nil {
		t.Fatalf("route parity: %v", document.Paths)
	}
}

func TestCustomerMediaSessionDeniesCrossCustomerSupportCase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"complaint-1","complainant_id":"other","complainant_type":"customer"}}`))
	}))
	defer owner.Close()
	signer := sharedauth.NewTokenSigner([]byte("secret"))
	token, _ := signer.Sign(sharedauth.Claims{Subject: "customer-1", Role: "customer", Service: "customer", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	Register(engine.Group("/api/v1/customer"), signer, Handler{Config: config.Config{MediaUploadSessionEnabled: true,
		SupportBaseURL: owner.URL, MediaBaseURL: "http://media.invalid", MediaPublicBaseURL: "https://media.example", MediaPrivateSecret: []byte("secret"),
	}, Limiter: allowSessions{}})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/customer/media-upload-sessions", strings.NewReader(`{"purpose":"DISPUTE_ATTACHMENT","source_authority":"SUPPORT_DISPUTE","source_ref":"complaint-1","mime_type":"image/png","byte_size":1024}`))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", uuid.NewString())
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-customer case: %d %s", w.Code, w.Body.String())
	}
}

// Customer V4 §24: with CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED off, a valid,
// authenticated request against a fully configured deployment is refused
// server-side, and no Support or Media call is made.
func TestCustomerMediaSessionFlagOffRefusesServerSide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++ }))
	defer upstream.Close()
	signer := sharedauth.NewTokenSigner([]byte("secret"))
	token, _ := signer.Sign(sharedauth.Claims{Subject: "customer-1", Role: "customer", Service: "customer", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	Register(engine.Group("/api/v1/customer"), signer, Handler{Config: config.Config{
		SupportBaseURL: upstream.URL, MediaBaseURL: upstream.URL, MediaPublicBaseURL: upstream.URL, MediaPrivateSecret: []byte("media-secret"),
	}, Limiter: allowSessions{}})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/customer/media-upload-sessions",
		bytes.NewBufferString(`{"purpose":"DISPUTE_ATTACHMENT","source_authority":"SUPPORT_DISPUTE","source_ref":"complaint-1","mime_type":"image/png","byte_size":1024}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", uuid.NewString())
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || calls != 0 {
		t.Fatalf("flag off: status=%d upstream calls=%d", w.Code, calls)
	}
}
