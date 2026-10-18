package partnerauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// --- test doubles -----------------------------------------------------------

type fakeRepo struct {
	record  *APIKey
	findErr error
	updated chan string
}

func (f *fakeRepo) FindByPrefix(_ context.Context, prefix string) (*APIKey, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.record != nil && f.record.KeyPrefix == prefix {
		return f.record, nil
	}
	return nil, nil
}

func (f *fakeRepo) UpdateLastUsed(_ context.Context, keyID string) error {
	if f.updated != nil {
		f.updated <- keyID
	}
	return nil
}

const validRawKey = "sk_test_abcdefghijklmnopqrstuvwxyz012345"

func hashOf(t *testing.T, raw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword() error = %v", err)
	}
	return string(h)
}

func activeRecord(t *testing.T) *APIKey {
	t.Helper()
	return &APIKey{
		ID:          "key-1",
		PartnerID:   "partner-1",
		KeyHash:     hashOf(t, validRawKey),
		KeyPrefix:   validRawKey[:keyPrefixLength],
		Environment: EnvironmentSandbox,
		Status:      APIKeyStatusActive,
	}
}

// buildRouter mounts the middleware plus a terminal handler that reports the
// partner identity it observed in context.
func buildRouter(repo APIKeyRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/partner/v1")
	g.Use(RequirePartnerAPIKey(repo))
	g.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"partner_id":          PartnerID(c),
			"partner_environment": PartnerEnvironment(c),
		})
	})
	return r
}

func do(r *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/partner/v1/ping", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", w.Body.String(), err)
	}
	return body
}

// --- tests ------------------------------------------------------------------

func TestMissingKeyReturns401(t *testing.T) {
	w := do(buildRouter(&fakeRepo{}), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, w).Error.Code; got != "missing_api_key" {
		t.Fatalf("code = %q, want missing_api_key", got)
	}
}

func TestTooShortKeyReturns401Invalid(t *testing.T) {
	w := do(buildRouter(&fakeRepo{}), map[string]string{"X-API-Key": "short"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, w).Error.Code; got != "invalid_api_key" {
		t.Fatalf("code = %q, want invalid_api_key", got)
	}
}

func TestUnknownPrefixReturns401Invalid(t *testing.T) {
	// repo has no record, so FindByPrefix returns (nil, nil).
	w := do(buildRouter(&fakeRepo{}), map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, w).Error.Code; got != "invalid_api_key" {
		t.Fatalf("code = %q, want invalid_api_key", got)
	}
}

func TestWrongPasswordReturns401Invalid(t *testing.T) {
	rec := activeRecord(t)
	rec.KeyHash = hashOf(t, "sk_test_some_other_key_entirely_9999") // hash of a different key
	w := do(buildRouter(&fakeRepo{record: rec}), map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, w).Error.Code; got != "invalid_api_key" {
		t.Fatalf("code = %q, want invalid_api_key", got)
	}
}

func TestRepoErrorReturns401Invalid(t *testing.T) {
	w := do(buildRouter(&fakeRepo{findErr: errors.New("db down")}),
		map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if got := decodeError(t, w).Error.Code; got != "invalid_api_key" {
		t.Fatalf("code = %q, want invalid_api_key", got)
	}
}

func TestRevokedKeyReturns403(t *testing.T) {
	rec := activeRecord(t)
	rec.Status = APIKeyStatusRevoked
	w := do(buildRouter(&fakeRepo{record: rec}), map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	if got := decodeError(t, w).Error.Code; got != "api_key_revoked" {
		t.Fatalf("code = %q, want api_key_revoked", got)
	}
}

func TestValidKeyViaXAPIKeyPasses(t *testing.T) {
	w := do(buildRouter(&fakeRepo{record: activeRecord(t)}),
		map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestValidKeyViaBearerPasses(t *testing.T) {
	w := do(buildRouter(&fakeRepo{record: activeRecord(t)}),
		map[string]string{"Authorization": "Bearer " + validRawKey})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestXAPIKeyTakesPriorityOverBearer(t *testing.T) {
	// X-API-Key is valid; Authorization carries a bogus key. Priority => success.
	w := do(buildRouter(&fakeRepo{record: activeRecord(t)}), map[string]string{
		"X-API-Key":     validRawKey,
		"Authorization": "Bearer sk_test_this_is_the_wrong_key_00000",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestSuccessSetsPartnerIdentity(t *testing.T) {
	w := do(buildRouter(&fakeRepo{record: activeRecord(t)}),
		map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		PartnerID   string `json:"partner_id"`
		Environment string `json:"partner_environment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if body.PartnerID != "partner-1" {
		t.Fatalf("partner_id = %q, want partner-1", body.PartnerID)
	}
	if body.Environment != EnvironmentSandbox {
		t.Fatalf("partner_environment = %q, want %q", body.Environment, EnvironmentSandbox)
	}
}

func TestSuccessTriggersUpdateLastUsed(t *testing.T) {
	repo := &fakeRepo{record: activeRecord(t), updated: make(chan string, 1)}
	w := do(buildRouter(repo), map[string]string{"X-API-Key": validRawKey})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	select {
	case gotID := <-repo.updated:
		if gotID != "key-1" {
			t.Fatalf("UpdateLastUsed keyID = %q, want key-1", gotID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateLastUsed was not called asynchronously")
	}
}
