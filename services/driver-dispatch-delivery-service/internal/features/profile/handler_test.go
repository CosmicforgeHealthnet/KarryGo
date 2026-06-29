package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

// ── Existing ABC tests ────────────────────────────────────────────────────────

func TestPublicRouteDoesNotRequireJWT(t *testing.T) {
	router, _ := buildProfileTestRouter(newFakeProfileRepository())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/11111111-1111-1111-1111-111111111111/public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("public route returned 401; body = %s", w.Body.String())
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing public provider; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestProtectedRoutesReturn401WithoutJWT(t *testing.T) {
	router, _ := buildProfileTestRouter(newFakeProfileRepository())
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/provider/onboarding", `{}`},
		{http.MethodGet, "/api/v1/provider/me", ``},
		{http.MethodPatch, "/api/v1/provider/me", `{}`},
		{http.MethodPost, "/api/v1/provider/emergency-contact", `{}`},
		{http.MethodGet, "/api/v1/provider/emergency-contact", ``},
		{http.MethodPost, "/api/v1/provider/guarantor", `{}`},
		{http.MethodGet, "/api/v1/provider/guarantor", ``},
		{http.MethodGet, "/api/v1/provider/stats", ``},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
			}
			assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
		})
	}
}

func TestPatchMeRejectsReadOnlyFields(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"phone":"+2348012345678","full_name":"Ada Lovelace"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "phone")
}

func TestPublicProfileResponseDoesNotExposePhoneOrEmail(t *testing.T) {
	repo := newFakeProfileRepository()
	providerID := "11111111-1111-1111-1111-111111111111"
	fullName := "Ada Lovelace"
	email := "ada@example.com"
	provider, _ := repo.EnsureProvider(context.Background(), providerID, "+2348000000001")
	provider.FullName = &fullName
	provider.Email = &email
	repo.providers[provider.ID] = provider
	router, _ := buildProfileTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/"+providerID+"/public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["provider_id"] != providerID {
		t.Fatalf("provider_id = %v, want %s", data["provider_id"], providerID)
	}
	if _, ok := data["phone"]; ok {
		t.Fatalf("public profile exposed phone: %s", w.Body.String())
	}
	if _, ok := data["email"]; ok {
		t.Fatalf("public profile exposed email: %s", w.Body.String())
	}
}

func TestPublicProfileRejectsInvalidUUID(t *testing.T) {
	router, _ := buildProfileTestRouter(newFakeProfileRepository())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/not-a-uuid/public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
	assertValidationFieldInBody(t, w.Body.Bytes(), "id")
}

func TestPublicProfileDoesNotExposeLocationOrOperationType(t *testing.T) {
	repo := newFakeProfileRepository()
	providerID := "22222222-2222-2222-2222-222222222222"
	fullName := "Ada Lovelace"
	email := "ada@example.com"
	city := "Ikeja"
	state := "Lagos"
	op := OperationIndividual
	provider, _ := repo.EnsureProvider(context.Background(), providerID, "+2348000000001")
	provider.FullName = &fullName
	provider.Email = &email
	provider.City = &city
	provider.State = &state
	provider.OperationType = &op
	repo.providers[provider.ID] = provider
	router, _ := buildProfileTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/"+providerID+"/public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	for _, forbidden := range []string{"phone", "email", "city", "state", "operation_type"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("public profile exposed %s: %s", forbidden, w.Body.String())
		}
	}
}

func TestPublicProfileReturns404ForInactiveOrSuspendedProvider(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Provider)
	}{
		{name: "inactive", mutate: func(p *Provider) { p.IsActive = false }},
		{name: "suspended", mutate: func(p *Provider) { p.VerificationStatus = StatusSuspended }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeProfileRepository()
			providerID := "33333333-3333-3333-3333-333333333333"
			provider, _ := repo.EnsureProvider(context.Background(), providerID, "+2348000000001")
			tc.mutate(&provider)
			repo.providers[provider.ID] = provider
			router, _ := buildProfileTestRouter(repo)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/"+providerID+"/public", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
			}
			assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
		})
	}
}

func TestPublicProfileRateLimitReturnsRateLimited(t *testing.T) {
	repo := newFakeProfileRepository()
	providerID := "44444444-4444-4444-4444-444444444444"
	repo.mustEnsure(providerID, "+2348000000001")
	router, _ := buildProfileTestRouter(repo)

	var w *httptest.ResponseRecorder
	for i := 0; i < 61; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/"+providerID+"/public", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeRateLimited)
}

func TestStatsReturnsOnlyBasicProfileStats(t *testing.T) {
	repo := newFakeProfileRepository()
	provider, _ := repo.EnsureProvider(context.Background(), "55555555-5555-5555-5555-555555555555", "+2348000000001")
	provider.AvgRating = 4.75
	provider.TotalTrips = 9
	repo.providers[provider.ID] = provider
	repo.ratingCounts["55555555-5555-5555-5555-555555555555"] = 7
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	for _, field := range []string{"avg_rating", "total_trips", "verification_status", "is_active", "ratings_count", "completion_rate"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("stats missing %s: %s", field, w.Body.String())
		}
	}
	for _, forbidden := range []string{"phone", "email", "earnings", "trips", "provider_id", "onboarding_complete"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("stats exposed %s: %s", forbidden, w.Body.String())
		}
	}
}

// ── 2D handler tests ──────────────────────────────────────────────────────────

func TestHandlerOnboardingValidReturns200(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerOnboardingSecondCallReturns409(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("first onboarding status = %d; body = %s", w.Code, w.Body.String())
	}
	// Manually mark complete
	p := repo.providers["55555555-5555-5555-5555-555555555555"]
	p.OnboardingComplete = true
	repo.providers["55555555-5555-5555-5555-555555555555"] = p

	w := do()
	if w.Code != http.StatusConflict {
		t.Fatalf("second onboarding status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeConflict)
}

func TestHandlerOnboardingProviderIDInBodyIgnored(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"provider_id":"evil-id","id":"another-id","full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with body provider_id ignored; body = %s", w.Code, w.Body.String())
	}
	if _, ok := repo.providers["evil-id"]; ok {
		t.Fatal("body provider_id created or updated an unrelated provider")
	}
	if _, ok := repo.providers["another-id"]; ok {
		t.Fatal("body id created or updated an unrelated provider")
	}
	provider := repo.providers["55555555-5555-5555-5555-555555555555"]
	if provider.FullName == nil || *provider.FullName != "Emeka Okafor" {
		t.Fatalf("authenticated provider was not updated: %+v", provider)
	}
}

func TestHandlerOnboardingInvalidOperationTypeReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "operation_type")
}

func TestHandlerOnboardingMissingFullNameReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "full_name")
}

func TestHandlerOnboardingSingleNameReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "full_name")
}

func TestHandlerOnboardingMissingStateReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "state")
}

func TestHandlerOnboardingMissingCityReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","state":"Lagos","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "city")
}

func TestHandlerOnboardingEmailOmittedIsValid(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerOnboardingInvalidEmailReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Emeka Okafor","email":"not-an-email","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "email")
}

// ── 2E handler tests ──────────────────────────────────────────────────────────

func TestHandlerGetMeReturns404WhenProviderMissing(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestHandlerGetMeReturnsSparseProfileBeforeOnboarding(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["provider_id"] != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("provider_id = %v, want provider-123", data["provider_id"])
	}
	if data["has_emergency_contact"] != false {
		t.Fatalf("has_emergency_contact = %v, want false", data["has_emergency_contact"])
	}
	if data["has_guarantor"] != false {
		t.Fatalf("has_guarantor = %v, want false", data["has_guarantor"])
	}
}

func TestHandlerGetMeReturnsFullProfileAfterOnboarding(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	// Onboard first
	onboardBody := `{"full_name":"Emeka Okafor","state":"Lagos","city":"Ikeja","operation_type":"individual"}`
	onboardReq := httptest.NewRequest(http.MethodPost, "/api/v1/provider/onboarding", bytes.NewBufferString(onboardBody))
	onboardReq.Header.Set("Content-Type", "application/json")
	onboardReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(httptest.NewRecorder(), onboardReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["full_name"] != "Emeka Okafor" {
		t.Fatalf("full_name = %v, want Emeka Okafor", data["full_name"])
	}
}

func TestHandlerGetMeHasEmergencyContactAndGuarantorFlags(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	// Pre-populate contact and guarantor directly in repo
	contact := EmergencyContact{ID: "ec-1", ProviderID: "55555555-5555-5555-5555-555555555555", FullName: "Grace Hopper", Phone: "+2348012345678", Relationship: "Sister"}
	repo.contacts["55555555-5555-5555-5555-555555555555"] = contact
	guarantor := Guarantor{ID: "g-1", ProviderID: "55555555-5555-5555-5555-555555555555", FullName: "Alan Turing", Phone: "+2348099999999"}
	repo.guarantors["55555555-5555-5555-5555-555555555555"] = guarantor

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["has_emergency_contact"] != true {
		t.Fatalf("has_emergency_contact = %v, want true", data["has_emergency_contact"])
	}
	if data["has_guarantor"] != true {
		t.Fatalf("has_guarantor = %v, want true", data["has_guarantor"])
	}
}

// ── 2F handler tests ──────────────────────────────────────────────────────────

func TestHandlerPatchMeCityOnly(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	// Set initial state via fake onboarding
	fullName := "Ada Lovelace"
	state := "Lagos"
	city := "Ikeja"
	op := OperationIndividual
	p := repo.providers["55555555-5555-5555-5555-555555555555"]
	p.FullName = &fullName
	p.State = &state
	p.City = &city
	p.OperationType = &op
	repo.providers["55555555-5555-5555-5555-555555555555"] = p

	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"city":"Lekki"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["city"] != "Lekki" {
		t.Fatalf("city = %v, want Lekki", data["city"])
	}
	if data["full_name"] != "Ada Lovelace" {
		t.Fatalf("full_name changed unexpectedly = %v", data["full_name"])
	}
}

func TestHandlerPatchMeEmptyBodyReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "body")
}

func TestHandlerPatchMeInvalidEmailReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"email":"not-valid"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "email")
}

func TestHandlerPatchMePublishesProfileUpdatedEvent(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())
	tokens := authusecases.NewTokenUsecase([]byte("profile-test-secret"), 15*time.Minute, 30*24*time.Hour)
	RegisterRoutes(router, tokens, NewHandler(service))
	token := mustAccessToken(t, tokens)

	body := `{"city":"Lekki"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if len(publisher.profileUpdated) != 1 {
		t.Fatalf("published %d events, want 1", len(publisher.profileUpdated))
	}
	if publisher.profileUpdated[0].Event != TopicProfileUpdated {
		t.Fatalf("event = %s, want %s", publisher.profileUpdated[0].Event, TopicProfileUpdated)
	}
}

func TestHandlerPatchMeReturnsFullUpdatedProfile(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	body := `{"city":"Lekki"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	for _, field := range []string{"provider_id", "phone", "has_emergency_contact", "has_guarantor", "onboarding_complete", "is_active"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("response missing field %s: %s", field, w.Body.String())
		}
	}
}

// ── 2G handler tests ──────────────────────────────────────────────────────────

func TestHandlerSetEmergencyContactValidReturns200(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Adaeze Okafor","phone":"+2348098765432","relationship":"spouse"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["full_name"] != "Adaeze Okafor" {
		t.Fatalf("full_name = %v, want Adaeze Okafor", data["full_name"])
	}
}

func TestHandlerSetEmergencyContactInvalidPhoneReturns400(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Adaeze Okafor","phone":"08098765432","relationship":"spouse"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "phone")
}

func TestHandlerSetEmergencyContactMissingFullNameReturns400(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"phone":"+2348098765432","relationship":"spouse"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "full_name")
}

func TestHandlerSetEmergencyContactMissingRelationshipReturns400(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Adaeze Okafor","phone":"+2348098765432"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "relationship")
}

func TestHandlerSetEmergencyContactCalledTwiceReplacesContact(t *testing.T) {
	repo := newFakeProfileRepository()
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	doPost := func(name string) *httptest.ResponseRecorder {
		body := `{"full_name":"` + name + `","phone":"+2348098765432","relationship":"spouse"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w1 := doPost("Grace Hopper")
	if w1.Code != http.StatusOK {
		t.Fatalf("first POST status = %d; body = %s", w1.Code, w1.Body.String())
	}
	w2 := doPost("Katherine Johnson")
	if w2.Code != http.StatusOK {
		t.Fatalf("second POST status = %d; body = %s", w2.Code, w2.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["full_name"] != "Katherine Johnson" {
		t.Fatalf("full_name = %v, want Katherine Johnson", data["full_name"])
	}
	// Confirm only one row exists (upsert semantics)
	count := 0
	for range repo.contacts {
		count++
	}
	if count != 1 {
		t.Fatalf("expected 1 emergency contact row, got %d", count)
	}
}

func TestHandlerGetEmergencyContactReturns200WhenExists(t *testing.T) {
	repo := newFakeProfileRepository()
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	// Create contact first
	createBody := `{"full_name":"Grace Hopper","phone":"+2348012345678","relationship":"Sister"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/provider/emergency-contact", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/emergency-contact", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetEmergencyContactReturns404WhenNotSet(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/emergency-contact", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

// ── 2H handler tests ──────────────────────────────────────────────────────────

func TestHandlerSetGuarantorValidReturns200(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Chukwudi Okafor","phone":"+2347011223344"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/guarantor", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["full_name"] != "Chukwudi Okafor" {
		t.Fatalf("full_name = %v, want Chukwudi Okafor", data["full_name"])
	}
}

func TestHandlerSetGuarantorInvalidPhoneReturns400(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"full_name":"Chukwudi Okafor","phone":"07011223344"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/guarantor", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "phone")
}

func TestHandlerSetGuarantorMissingFullNameReturns400(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	body := `{"phone":"+2347011223344"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/guarantor", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertValidationFieldInBody(t, w.Body.Bytes(), "full_name")
}

func TestHandlerSetGuarantorCalledTwiceReplacesGuarantor(t *testing.T) {
	repo := newFakeProfileRepository()
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	doPost := func(name string) *httptest.ResponseRecorder {
		body := `{"full_name":"` + name + `","phone":"+2347011223344"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/guarantor", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := doPost("Alan Turing"); w.Code != http.StatusOK {
		t.Fatalf("first POST status = %d; body = %s", w.Code, w.Body.String())
	}
	w2 := doPost("Mary Jackson")
	if w2.Code != http.StatusOK {
		t.Fatalf("second POST status = %d; body = %s", w2.Code, w2.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["full_name"] != "Mary Jackson" {
		t.Fatalf("full_name = %v, want Mary Jackson", data["full_name"])
	}
	count := 0
	for range repo.guarantors {
		count++
	}
	if count != 1 {
		t.Fatalf("expected 1 guarantor row, got %d", count)
	}
}

func TestHandlerGetGuarantorReturns200WhenExists(t *testing.T) {
	repo := newFakeProfileRepository()
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	createBody := `{"full_name":"Alan Turing","phone":"+2348099999999"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/provider/guarantor", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(httptest.NewRecorder(), createReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/guarantor", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetGuarantorReturns404WhenNotSet(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/guarantor", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

// ── 2I handler tests ──────────────────────────────────────────────────────────

func TestHandlerGetStatsReturnsCorrectValues(t *testing.T) {
	repo := newFakeProfileRepository()
	provider, _ := repo.EnsureProvider(context.Background(), "55555555-5555-5555-5555-555555555555", "+2348000000001")
	provider.TotalTrips = 42
	provider.AvgRating = 4.85
	provider.VerificationStatus = StatusVerified
	provider.IsActive = true
	repo.providers[provider.ID] = provider
	repo.ratingCounts["55555555-5555-5555-5555-555555555555"] = 38

	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["total_trips"].(float64) != 42 {
		t.Fatalf("total_trips = %v, want 42", data["total_trips"])
	}
	if data["avg_rating"].(float64) != 4.85 {
		t.Fatalf("avg_rating = %v, want 4.85", data["avg_rating"])
	}
	if data["ratings_count"].(float64) != 38 {
		t.Fatalf("ratings_count = %v, want 38", data["ratings_count"])
	}
	if data["completion_rate"].(float64) != 1.0 {
		t.Fatalf("completion_rate = %v, want 1.0", data["completion_rate"])
	}
}

func TestHandlerGetStatsZeroValuesForNewProvider(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("55555555-5555-5555-5555-555555555555", "+2348000000001")
	router, tokens := buildProfileTestRouter(repo)
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["total_trips"].(float64) != 0 {
		t.Fatalf("total_trips = %v, want 0", data["total_trips"])
	}
	if data["completion_rate"].(float64) != 0 {
		t.Fatalf("completion_rate = %v, want 0", data["completion_rate"])
	}
}

func TestHandlerGetStatsReturns404WhenProviderMissing(t *testing.T) {
	router, tokens := buildProfileTestRouter(newFakeProfileRepository())
	token := mustAccessToken(t, tokens)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func TestSettingsAndAvatarRoutesRequireJWT(t *testing.T) {
	router, _ := buildProfileTestRouter(newFakeProfileRepository())
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/provider/settings"},
		{method: http.MethodPatch, path: "/api/v1/provider/settings", body: `{"push_enabled":false}`},
		{method: http.MethodPost, path: "/api/v1/provider/me/avatar"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401; body = %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func buildProfileTestRouter(repo *fakeProfileRepository) (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	tokens := authusecases.NewTokenUsecase([]byte("profile-test-secret"), 15*time.Minute, 30*24*time.Hour)
	RegisterRoutes(router, tokens, NewHandler(NewService(repo)))
	return router, tokens
}

func mustAccessToken(t *testing.T, tokens *authusecases.TokenUsecase) string {
	t.Helper()
	token, _, err := tokens.GenerateAccessToken("55555555-5555-5555-5555-555555555555", "+2348000000001", "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	return token
}

func assertErrorCodeInBody(t *testing.T, raw []byte, code apperrors.Code) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errorObj, _ := resp["error"].(map[string]any)
	if errorObj["code"] != string(code) {
		t.Fatalf("code = %v, want %s; body = %s", errorObj["code"], code, raw)
	}
}

func assertValidationFieldInBody(t *testing.T, raw []byte, field string) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errorObj, _ := resp["error"].(map[string]any)
	fields, _ := errorObj["fields"].([]any)
	for _, item := range fields {
		violation, _ := item.(map[string]any)
		if violation["field"] == field {
			return
		}
	}
	t.Fatalf("fields = %v, want %s; body = %s", fields, field, raw)
}
