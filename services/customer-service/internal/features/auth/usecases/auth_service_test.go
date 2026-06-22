package authusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	authmodels "cosmicforge/logistics/services/customer-service/internal/features/auth/models"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func TestStartAndVerifyAuth(t *testing.T) {
	ctx := context.Background()
	customers := newFakeCustomerRepository()
	sessions := newFakeSessionRepository()
	challenges := newFakeChallengeStore()
	service := newTestService(customers, sessions, challenges)

	start, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"})
	if err != nil {
		t.Fatalf("StartAuth() error = %v", err)
	}
	if start.ChallengeID == "" || start.DebugOTP == "" {
		t.Fatalf("expected challenge id and debug otp, got %+v", start)
	}

	result, err := service.VerifyAuth(ctx, VerifyAuthInput{
		Phone:       "+2348012345678",
		OTP:         start.DebugOTP,
		ChallengeID: start.ChallengeID,
		UserAgent:   "test-agent",
		IPAddress:   "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("VerifyAuth() error = %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatalf("expected tokens, got %+v", result)
	}
	if result.Customer.Phone != "+2348012345678" {
		t.Fatalf("unexpected customer phone: %+v", result.Customer)
	}
	if len(sessions.sessions) != 1 {
		t.Fatalf("expected one session, got %d", len(sessions.sessions))
	}
}

func TestVerifyAuthRejectsWrongOTP(t *testing.T) {
	ctx := context.Background()
	challenges := newFakeChallengeStore()
	service := newTestService(newFakeCustomerRepository(), newFakeSessionRepository(), challenges)

	start, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"})
	if err != nil {
		t.Fatalf("StartAuth() error = %v", err)
	}

	_, err = service.VerifyAuth(ctx, VerifyAuthInput{
		Phone:       "08012345678",
		OTP:         "000000",
		ChallengeID: start.ChallengeID,
	})
	if err == nil {
		t.Fatal("expected wrong otp to fail")
	}
	stored, ok, err := challenges.Get(ctx, authmodels.AuthIdentifier{Type: authmodels.IdentifierTypePhone, Value: "+2348012345678"})
	if err != nil || !ok {
		t.Fatalf("expected stored challenge, ok=%v err=%v", ok, err)
	}
	if stored.Attempts != 1 {
		t.Fatalf("expected one failed attempt, got %d", stored.Attempts)
	}
}

func TestStartAndVerifyEmailAuth(t *testing.T) {
	ctx := context.Background()
	customers := newFakeCustomerRepository()
	service := newTestService(customers, newFakeSessionRepository(), newFakeChallengeStore())

	start, err := service.StartAuth(ctx, StartAuthInput{Email: "Ada@Example.COM"})
	if err != nil {
		t.Fatalf("StartAuth() error = %v", err)
	}

	result, err := service.VerifyAuth(ctx, VerifyAuthInput{
		Email:       "ada@example.com",
		OTP:         start.DebugOTP,
		ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth() error = %v", err)
	}
	if result.Customer.Email != "ada@example.com" {
		t.Fatalf("unexpected customer email: %+v", result.Customer)
	}
	if _, ok := customers.byEmail["ada@example.com"]; !ok {
		t.Fatal("expected customer to be stored by normalized email")
	}
}

func TestStartAuthRejectsInvalidIdentifierInput(t *testing.T) {
	ctx := context.Background()
	service := newTestService(newFakeCustomerRepository(), newFakeSessionRepository(), newFakeChallengeStore())

	for name, input := range map[string]StartAuthInput{
		"missing": {},
		"both":    {Phone: "08012345678", Email: "ada@example.com"},
		"email":   {Email: "not-an-email"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.StartAuth(ctx, input)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeValidationFailed {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestStartAuthRateLimitSeparatesPhoneAndEmail(t *testing.T) {
	ctx := context.Background()
	service := newTestService(newFakeCustomerRepository(), newFakeSessionRepository(), newFakeChallengeStore())

	for i := 0; i < 5; i++ {
		if _, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"}); err != nil {
			t.Fatalf("phone StartAuth(%d) error = %v", i, err)
		}
	}
	if _, err := service.StartAuth(ctx, StartAuthInput{Email: "ada@example.com"}); err != nil {
		t.Fatalf("expected email request to have separate rate limit, got %v", err)
	}
}

func TestStartAuthRateLimit(t *testing.T) {
	ctx := context.Background()
	service := newTestService(newFakeCustomerRepository(), newFakeSessionRepository(), newFakeChallengeStore())

	for i := 0; i < 5; i++ {
		if _, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"}); err != nil {
			t.Fatalf("StartAuth(%d) error = %v", i, err)
		}
	}

	_, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"})
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeRateLimited {
		t.Fatalf("expected rate_limited error, got %v", err)
	}
}

func TestRefreshRotatesSession(t *testing.T) {
	ctx := context.Background()
	sessions := newFakeSessionRepository()
	service := newTestService(newFakeCustomerRepository(), sessions, newFakeChallengeStore())

	start, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"})
	if err != nil {
		t.Fatalf("StartAuth() error = %v", err)
	}
	verified, err := service.VerifyAuth(ctx, VerifyAuthInput{
		Phone:       "08012345678",
		OTP:         start.DebugOTP,
		ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth() error = %v", err)
	}

	refreshed, err := service.Refresh(ctx, RefreshInput{RefreshToken: verified.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == verified.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}
	if len(sessions.revoked) != 1 {
		t.Fatalf("expected one revoked session, got %d", len(sessions.revoked))
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	ctx := context.Background()
	sessions := newFakeSessionRepository()
	service := newTestService(newFakeCustomerRepository(), sessions, newFakeChallengeStore())

	start, err := service.StartAuth(ctx, StartAuthInput{Phone: "08012345678"})
	if err != nil {
		t.Fatalf("StartAuth() error = %v", err)
	}
	verified, err := service.VerifyAuth(ctx, VerifyAuthInput{
		Phone:       "08012345678",
		OTP:         start.DebugOTP,
		ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth() error = %v", err)
	}

	if err := service.Logout(ctx, verified.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if len(sessions.revoked) != 1 {
		t.Fatalf("expected one revoked session, got %d", len(sessions.revoked))
	}

	err = service.Logout(ctx, verified.RefreshToken)
	if err == nil {
		t.Fatal("expected revoked session logout to fail")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func newTestService(customers *fakeCustomerRepository, sessions *fakeSessionRepository, challenges *fakeChallengeStore) *AuthService {
	service := NewAuthService(Options{
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
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.accessSigner.WithClock(func() time.Time { return now })
	return service
}

type fakeCustomerRepository struct {
	byPhone map[string]profilemodels.Customer
	byEmail map[string]profilemodels.Customer
	byID    map[string]profilemodels.Customer
}

func newFakeCustomerRepository() *fakeCustomerRepository {
	return &fakeCustomerRepository{
		byPhone: map[string]profilemodels.Customer{},
		byEmail: map[string]profilemodels.Customer{},
		byID:    map[string]profilemodels.Customer{},
	}
}

func (r *fakeCustomerRepository) UpsertByPhone(ctx context.Context, phone string) (profilemodels.Customer, error) {
	if existing, ok := r.byPhone[phone]; ok {
		return existing, nil
	}

	created := profilemodels.Customer{
		ID:               "customer-1",
		Phone:            phone,
		OnboardingStatus: profilemodels.OnboardingProfileNeeded,
		Status:           profilemodels.StatusActive,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	r.byPhone[phone] = created
	r.byID[created.ID] = created
	return created, nil
}

func (r *fakeCustomerRepository) UpsertByEmail(ctx context.Context, email string) (profilemodels.Customer, error) {
	if existing, ok := r.byEmail[email]; ok {
		return existing, nil
	}

	created := profilemodels.Customer{
		ID:               "customer-1",
		Email:            email,
		OnboardingStatus: profilemodels.OnboardingProfileNeeded,
		Status:           profilemodels.StatusActive,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	r.byEmail[email] = created
	r.byID[created.ID] = created
	return created, nil
}

func (r *fakeCustomerRepository) GetByID(ctx context.Context, id string) (profilemodels.Customer, error) {
	return r.byID[id], nil
}

func (r *fakeCustomerRepository) UpdateProfilePhoto(ctx context.Context, id, assetID, photoURL string) (profilemodels.Customer, error) {
	customer, ok := r.byID[id]
	if !ok {
		return profilemodels.Customer{}, nil
	}
	customer.ProfilePhotoURL = &photoURL
	customer.ProfilePhotoAssetID = &assetID
	r.byID[id] = customer
	return customer, nil
}

func (r *fakeCustomerRepository) UpdateProfile(ctx context.Context, id, firstName, lastName string) (profilemodels.Customer, error) {
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

func (r *fakeCustomerRepository) GetEmergencyContacts(ctx context.Context, customerID string) ([]profilemodels.EmergencyContact, error) {
	return nil, nil
}

func (r *fakeCustomerRepository) AddEmergencyContact(ctx context.Context, customerID, name, phone, relationship string) (profilemodels.EmergencyContact, error) {
	return profilemodels.EmergencyContact{}, nil
}

func (r *fakeCustomerRepository) DeleteEmergencyContact(ctx context.Context, id, customerID string) error {
	return nil
}

type fakeSessionRepository struct {
	sessions map[string]authmodels.RefreshSession
	revoked  map[string]bool
}

func newFakeSessionRepository() *fakeSessionRepository {
	return &fakeSessionRepository{
		sessions: map[string]authmodels.RefreshSession{},
		revoked:  map[string]bool{},
	}
}

func (r *fakeSessionRepository) Create(ctx context.Context, value authmodels.RefreshSession) error {
	r.sessions[value.ID] = value
	return nil
}

func (r *fakeSessionRepository) GetByID(ctx context.Context, id string) (authmodels.RefreshSession, error) {
	value := r.sessions[id]
	if r.revoked[id] {
		now := time.Now()
		value.RevokedAt = &now
	}
	return value, nil
}

func (r *fakeSessionRepository) Revoke(ctx context.Context, id string) error {
	r.revoked[id] = true
	return nil
}

type fakeChallengeStore struct {
	challenges map[string]authmodels.OTPChallenge
	requests   map[string]int
}

func newFakeChallengeStore() *fakeChallengeStore {
	return &fakeChallengeStore{
		challenges: map[string]authmodels.OTPChallenge{},
		requests:   map[string]int{},
	}
}

func (s *fakeChallengeStore) Save(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration, rateWindow time.Duration, maxRequests int) error {
	key := challenge.IdentifierKey()
	s.requests[key]++
	if s.requests[key] > maxRequests {
		return apperrors.RateLimited("Too many attempts. Please try again shortly.", nil)
	}
	s.challenges[key] = challenge
	return nil
}

func (s *fakeChallengeStore) Get(ctx context.Context, identifier authmodels.AuthIdentifier) (authmodels.OTPChallenge, bool, error) {
	challenge, ok := s.challenges[identifier.Key()]
	return challenge, ok, nil
}

func (s *fakeChallengeStore) RecordFailedAttempt(ctx context.Context, challenge authmodels.OTPChallenge, ttl time.Duration) error {
	challenge.Attempts++
	s.challenges[challenge.IdentifierKey()] = challenge
	return nil
}

func (s *fakeChallengeStore) Delete(ctx context.Context, identifier authmodels.AuthIdentifier) error {
	delete(s.challenges, identifier.Key())
	return nil
}

func TestVerifyChallenge(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	challenge := authmodels.OTPChallenge{
		ID:              "challenge",
		IdentifierType:  authmodels.IdentifierTypePhone,
		IdentifierValue: "+2348012345678",
		OTPHash:         sharedauth.HashOTP([]byte("secret"), "challenge", authmodels.AuthIdentifier{Type: authmodels.IdentifierTypePhone, Value: "+2348012345678"}.Key(), "123456"),
		ExpiresAt:       now.Add(time.Minute),
	}

	if err := authmodels.VerifyOTPChallenge([]byte("secret"), challenge, "challenge", "123456", 5, now); err != nil {
		t.Fatalf("VerifyOTPChallenge() error = %v", err)
	}
}

func TestVerifyChallengeRejectsExpiredOTP(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	challenge := authmodels.OTPChallenge{
		ID:              "challenge",
		IdentifierType:  authmodels.IdentifierTypePhone,
		IdentifierValue: "+2348012345678",
		OTPHash:         sharedauth.HashOTP([]byte("secret"), "challenge", authmodels.AuthIdentifier{Type: authmodels.IdentifierTypePhone, Value: "+2348012345678"}.Key(), "123456"),
		ExpiresAt:       now.Add(-time.Minute),
	}

	err := authmodels.VerifyOTPChallenge([]byte("secret"), challenge, "challenge", "123456", 5, now)
	if err == nil {
		t.Fatal("expected expired challenge to fail")
	}

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Code != apperrors.CodeUnauthorized {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}
