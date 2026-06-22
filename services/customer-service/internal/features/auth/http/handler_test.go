package authhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authmodels "cosmicforge/logistics/services/customer-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

func TestCustomerAuthHTTPFlow(t *testing.T) {
	router := newHTTPTestRouter()

	tokens := signInHTTP(t, router)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/customer/me", nil)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestStartAuthHTTPRejectsInvalidPhone(t *testing.T) {
	router := newHTTPTestRouter()

	postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"phone": "123",
	}, http.StatusUnprocessableEntity)
}

func TestVerifyAuthHTTPRejectsWrongOTP(t *testing.T) {
	router := newHTTPTestRouter()
	startBody := postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"phone": "08012345678",
	}, http.StatusCreated)
	challengeID := startBody["data"].(map[string]interface{})["challenge_id"].(string)

	postJSON(t, router, "/api/v1/customer/auth/verify", map[string]interface{}{
		"phone":        "08012345678",
		"otp":          "000000",
		"challenge_id": challengeID,
	}, http.StatusUnauthorized)
}

func TestCustomerEmailAuthHTTPFlow(t *testing.T) {
	router := newHTTPTestRouter()
	startBody := postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"email": "Ada@Example.COM",
	}, http.StatusCreated)
	debugOTP := startBody["data"].(map[string]interface{})["debug_otp"].(string)
	challengeID := startBody["data"].(map[string]interface{})["challenge_id"].(string)

	verifyBody := postJSON(t, router, "/api/v1/customer/auth/verify", map[string]interface{}{
		"email":        "ada@example.com",
		"otp":          debugOTP,
		"challenge_id": challengeID,
	}, http.StatusOK)
	customer := verifyBody["data"].(map[string]interface{})["customer"].(map[string]interface{})
	if customer["email"] != "ada@example.com" {
		t.Fatalf("expected normalized email customer, got %+v", customer)
	}
}

func TestStartAuthHTTPRejectsBothIdentifiers(t *testing.T) {
	router := newHTTPTestRouter()

	postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"phone": "08012345678",
		"email": "ada@example.com",
	}, http.StatusUnprocessableEntity)
}

func TestStartAuthHTTPRateLimitsOTPRequests(t *testing.T) {
	router := newHTTPTestRouter()
	for i := 0; i < 5; i++ {
		postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
			"phone": "08012345678",
		}, http.StatusCreated)
	}

	postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"phone": "08012345678",
	}, http.StatusTooManyRequests)
}

func TestCustomerAuthHTTPRefreshAndLogout(t *testing.T) {
	router := newHTTPTestRouter()
	tokens := signInHTTP(t, router)

	refreshBody := postJSON(t, router, "/api/v1/customer/auth/refresh", map[string]interface{}{
		"refresh_token": tokens.RefreshToken,
	}, http.StatusOK)
	refreshedRefreshToken := refreshBody["data"].(map[string]interface{})["refresh_token"].(string)
	if refreshedRefreshToken == tokens.RefreshToken {
		t.Fatal("expected refresh token rotation")
	}

	postJSON(t, router, "/api/v1/customer/auth/logout", map[string]interface{}{
		"refresh_token": refreshedRefreshToken,
	}, http.StatusOK)

	postJSON(t, router, "/api/v1/customer/auth/refresh", map[string]interface{}{
		"refresh_token": refreshedRefreshToken,
	}, http.StatusUnauthorized)
}

type httpTokens struct {
	AccessToken  string
	RefreshToken string
}

func newHTTPTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := newHTTPTestService()
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())
	RegisterCustomerRoutes(router.Group("/api/v1/customer"), service)
	return router
}

func signInHTTP(t *testing.T, router *gin.Engine) httpTokens {
	t.Helper()
	startBody := postJSON(t, router, "/api/v1/customer/auth/start", map[string]interface{}{
		"phone": "08012345678",
	}, http.StatusCreated)
	debugOTP := startBody["data"].(map[string]interface{})["debug_otp"].(string)
	challengeID := startBody["data"].(map[string]interface{})["challenge_id"].(string)

	verifyBody := postJSON(t, router, "/api/v1/customer/auth/verify", map[string]interface{}{
		"phone":        "+2348012345678",
		"otp":          debugOTP,
		"challenge_id": challengeID,
	}, http.StatusOK)
	data := verifyBody["data"].(map[string]interface{})
	return httpTokens{
		AccessToken:  data["access_token"].(string),
		RefreshToken: data["refresh_token"].(string),
	}
}

func postJSON(t *testing.T, router *gin.Engine, path string, body map[string]interface{}, expectedStatus int) map[string]interface{} {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != expectedStatus {
		t.Fatalf("POST %s status = %d body=%s", path, response.Code, response.Body.String())
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v body=%s", err, response.Body.String())
	}
	return parsed
}

func newHTTPTestService() *authusecases.AuthService {
	customers := &httpFakeCustomerRepository{
		byPhone: map[string]profilemodels.Customer{},
		byEmail: map[string]profilemodels.Customer{},
		byID:    map[string]profilemodels.Customer{},
	}
	sessions := &httpFakeSessionRepository{sessions: map[string]authmodels.RefreshSession{}}
	challenges := &httpFakeChallengeStore{
		challenges: map[string]authmodels.OTPChallenge{},
		requests:   map[string]int{},
	}
	service := authusecases.NewAuthService(authusecases.Options{
		Customers:          customers,
		Sessions:           sessions,
		Challenges:         challenges,
		AccessTokenSecret:  []byte("access-secret"),
		RefreshTokenSecret: []byte("refresh-secret"),
		OTPSecret:          []byte("otp-secret"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    24 * time.Hour,
		OTPTTL:             5 * time.Minute,
		OTPRateWindow:      10 * time.Minute,
		OTPMaxRequests:     5,
		OTPMaxAttempts:     5,
		OTPDebug:           true,
	})
	return service
}

type httpFakeCustomerRepository struct {
	byPhone map[string]profilemodels.Customer
	byEmail map[string]profilemodels.Customer
	byID    map[string]profilemodels.Customer
}

func (r *httpFakeCustomerRepository) UpsertByPhone(ctx context.Context, phone string) (profilemodels.Customer, error) {
	if existing, ok := r.byPhone[phone]; ok {
		return existing, nil
	}
	created := profilemodels.Customer{
		ID:               "customer-http-1",
		Phone:            phone,
		OnboardingStatus: profilemodels.OnboardingProfileNeeded,
		Status:           profilemodels.StatusActive,
	}
	r.byPhone[phone] = created
	r.byID[created.ID] = created
	return created, nil
}

func (r *httpFakeCustomerRepository) UpsertByEmail(ctx context.Context, email string) (profilemodels.Customer, error) {
	if existing, ok := r.byEmail[email]; ok {
		return existing, nil
	}
	created := profilemodels.Customer{
		ID:               "customer-http-1",
		Email:            email,
		OnboardingStatus: profilemodels.OnboardingProfileNeeded,
		Status:           profilemodels.StatusActive,
	}
	r.byEmail[email] = created
	r.byID[created.ID] = created
	return created, nil
}

func (r *httpFakeCustomerRepository) GetByID(ctx context.Context, id string) (profilemodels.Customer, error) {
	return r.byID[id], nil
}

func (r *httpFakeCustomerRepository) UpdateProfilePhoto(ctx context.Context, id, assetID, photoURL string) (profilemodels.Customer, error) {
	customer, ok := r.byID[id]
	if !ok {
		return profilemodels.Customer{}, nil
	}
	customer.ProfilePhotoURL = &photoURL
	customer.ProfilePhotoAssetID = &assetID
	r.byID[id] = customer
	return customer, nil
}

func (r *httpFakeCustomerRepository) UpdateProfile(ctx context.Context, id, firstName, lastName string) (profilemodels.Customer, error) {
	customer, ok := r.byID[id]
	if !ok {
		return profilemodels.Customer{}, nil
	}
	if firstName != "" {
		customer.FirstName = &firstName
	}
	if lastName != "" {
		customer.LastName = &lastName
	}
	r.byID[id] = customer
	return customer, nil
}

func (r *httpFakeCustomerRepository) GetEmergencyContacts(ctx context.Context, customerID string) ([]profilemodels.EmergencyContact, error) {
	return nil, nil
}

func (r *httpFakeCustomerRepository) AddEmergencyContact(ctx context.Context, customerID, name, phone, relationship string) (profilemodels.EmergencyContact, error) {
	return profilemodels.EmergencyContact{}, nil
}

func (r *httpFakeCustomerRepository) DeleteEmergencyContact(ctx context.Context, id, customerID string) error {
	return nil
}

type httpFakeSessionRepository struct {
	sessions map[string]authmodels.RefreshSession
}

func (r *httpFakeSessionRepository) Create(ctx context.Context, value authmodels.RefreshSession) error {
	r.sessions[value.ID] = value
	return nil
}

func (r *httpFakeSessionRepository) GetByID(ctx context.Context, id string) (authmodels.RefreshSession, error) {
	return r.sessions[id], nil
}

func (r *httpFakeSessionRepository) Revoke(ctx context.Context, id string) error {
	value := r.sessions[id]
	now := time.Now()
	value.RevokedAt = &now
	r.sessions[id] = value
	return nil
}

type httpFakeChallengeStore struct {
	challenges map[string]authmodels.OTPChallenge
	requests   map[string]int
}

func (s *httpFakeChallengeStore) Save(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration, rateWindow time.Duration, maxRequests int) error {
	key := challenge.IdentifierKey()
	s.requests[key]++
	if s.requests[key] > maxRequests {
		return apperrors.RateLimited("Too many attempts. Please try again shortly.", nil)
	}
	s.challenges[key] = challenge
	return nil
}

func (s *httpFakeChallengeStore) Get(ctx context.Context, identifier authmodels.AuthIdentifier) (authmodels.OTPChallenge, bool, error) {
	challenge, ok := s.challenges[identifier.Key()]
	return challenge, ok, nil
}

func (s *httpFakeChallengeStore) RecordFailedAttempt(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration) error {
	challenge.Attempts++
	s.challenges[challenge.IdentifierKey()] = challenge
	return nil
}

func (s *httpFakeChallengeStore) Delete(ctx context.Context, identifier authmodels.AuthIdentifier) error {
	delete(s.challenges, identifier.Key())
	return nil
}
