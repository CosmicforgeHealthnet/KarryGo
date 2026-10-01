package mobilityhttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	mobilityusecases "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type recordingRepo struct {
	mobilityusecases.Repository
	savedFor string
}

func (r *recordingRepo) GetPreferences(context.Context, string) (mobilitymodels.MobilityPreferences, bool, error) {
	return mobilitymodels.MobilityPreferences{}, false, nil
}

func (r *recordingRepo) SavePreferences(_ context.Context, p mobilitymodels.MobilityPreferences, _ int64) (mobilitymodels.MobilityPreferences, error) {
	r.savedFor = p.CustomerID
	p.Version = 1
	return p, nil
}

func newRouter(t *testing.T, repo mobilityusecases.Repository) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	signer := sharedauth.NewTokenSigner([]byte("test-secret"))
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	RegisterRoutes(router.Group("/api/v1/customer"), mobilityusecases.NewService(repo, mobilityusecases.Flags{MobilityPreferences: true, GuestTravellers: true}), signer)
	token, err := signer.Sign(sharedauth.Claims{Subject: "token-customer", Role: authusecases.CustomerRole, Service: authusecases.CustomerService, SessionID: "s1", Type: sharedauth.TokenTypeAccess, ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return router, token
}

func do(router *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestPreferencesRequireAuthentication(t *testing.T) {
	router, _ := newRouter(t, &recordingRepo{})
	if got := do(router, http.MethodGet, "/api/v1/customer/mobility-preferences", "", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got.Code)
	}
}

func TestPreferencesIdentityComesFromTokenOnly(t *testing.T) {
	repo := &recordingRepo{}
	router, token := newRouter(t, repo)
	got := do(router, http.MethodPut, "/api/v1/customer/mobility-preferences", token, `{"customer_id":"someone-else","expected_version":0}`)
	if got.Code != http.StatusBadRequest {
		t.Fatalf("body customer_id status = %d, want 400 body=%s", got.Code, got.Body.String())
	}
	got = do(router, http.MethodPut, "/api/v1/customer/mobility-preferences", token, `{"luggage_default":"standard","expected_version":0}`)
	if got.Code != http.StatusOK || repo.savedFor != "token-customer" {
		t.Fatalf("status = %d savedFor=%q body=%s", got.Code, repo.savedFor, got.Body.String())
	}
}
