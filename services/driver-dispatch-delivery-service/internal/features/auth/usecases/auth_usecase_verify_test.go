package authusecases

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
)

// ── Additional fakes needed for Verify ───────────────────────────────────────

type fakeIdentityRepository struct {
	identities map[string]authmodels.Identity // keyed by phone
}

func newFakeIdentityRepository() *fakeIdentityRepository {
	return &fakeIdentityRepository{identities: make(map[string]authmodels.Identity)}
}

func (f *fakeIdentityRepository) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	id, ok := f.identities[phone]
	return id, ok, nil
}

func (f *fakeIdentityRepository) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	for _, identity := range f.identities {
		if identity.Email != nil && *identity.Email == email {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}

func (f *fakeIdentityRepository) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	for _, identity := range f.identities {
		if identity.ID == id {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}

func (f *fakeIdentityRepository) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	if existing, ok := f.identities[phone]; ok {
		existing.UpdatedAt = time.Now()
		f.identities[phone] = existing
		return existing, nil
	}
	id := authmodels.Identity{
		ID:          "identity-" + phone,
		PhoneNumber: phone,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.identities[phone] = id
	return id, nil
}

func (f *fakeIdentityRepository) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	if _, exists := f.identities[phone]; exists {
		return authmodels.Identity{}, apperrors.Conflict("An account with this phone number or email already exists.", nil)
	}
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	id := authmodels.Identity{
		ID:          "identity-" + phone,
		PhoneNumber: phone,
		Email:       emailPtr,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	f.identities[phone] = id
	return id, nil
}

func (f *fakeIdentityRepository) UpdatePhone(_ context.Context, identityID, oldPhone, newPhone string) error {
	identity, ok := f.identities[oldPhone]
	if !ok || identity.ID != identityID {
		return apperrors.Conflict("Phone number did not match current record.", nil)
	}
	delete(f.identities, oldPhone)
	identity.PhoneNumber = newPhone
	identity.UpdatedAt = time.Now()
	f.identities[newPhone] = identity
	return nil
}

func (f *fakeIdentityRepository) UpdateEmail(_ context.Context, identityID, email string) error {
	for phone, identity := range f.identities {
		if identity.ID == identityID {
			identity.Email = &email
			identity.UpdatedAt = time.Now()
			f.identities[phone] = identity
			return nil
		}
	}
	return apperrors.NotFound("Identity not found.", nil)
}

// suspendedIdentityRepo always returns a suspended identity.
type suspendedIdentityRepo struct{}

func (r *suspendedIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{ID: "id-1", PhoneNumber: phone, Status: authmodels.StatusSuspended}, true, nil
}
func (r *suspendedIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{}, false, nil
}
func (r *suspendedIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{ID: id, PhoneNumber: "+2348012345678", Status: authmodels.StatusSuspended}, true, nil
}
func (r *suspendedIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "id-1", PhoneNumber: phone, Status: authmodels.StatusSuspended}, nil
}
func (r *suspendedIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "id-1", PhoneNumber: phone, Status: authmodels.StatusSuspended}, nil
}
func (r *suspendedIdentityRepo) UpdatePhone(_ context.Context, _, _, _ string) error  { return nil }
func (r *suspendedIdentityRepo) UpdateEmail(_ context.Context, _, _ string) error     { return nil }

// deletedIdentityRepo always returns a deleted identity.
type deletedIdentityRepo struct{}

func (r *deletedIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{ID: "id-2", PhoneNumber: phone, Status: authmodels.StatusDeleted}, true, nil
}
func (r *deletedIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{}, false, nil
}
func (r *deletedIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{ID: id, PhoneNumber: "+2348012345678", Status: authmodels.StatusDeleted}, true, nil
}
func (r *deletedIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "id-2", PhoneNumber: phone, Status: authmodels.StatusDeleted}, nil
}
func (r *deletedIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "id-2", PhoneNumber: phone, Status: authmodels.StatusDeleted}, nil
}
func (r *deletedIdentityRepo) UpdatePhone(_ context.Context, _, _, _ string) error { return nil }
func (r *deletedIdentityRepo) UpdateEmail(_ context.Context, _, _ string) error   { return nil }

type fakeSessionRepository struct {
	sessions map[string]authmodels.Session // keyed by ID
}

func newFakeSessionRepository() *fakeSessionRepository {
	return &fakeSessionRepository{sessions: make(map[string]authmodels.Session)}
}
func (f *fakeSessionRepository) Create(ctx context.Context, s authmodels.Session) (authmodels.Session, error) {
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	f.sessions[s.ID] = s
	return s, nil
}
func (f *fakeSessionRepository) FindByRefreshTokenHash(ctx context.Context, hash string) (authmodels.Session, bool, error) {
	for _, s := range f.sessions {
		if s.RefreshTokenHash == hash && s.RevokedAt == nil {
			return s, true, nil
		}
	}
	return authmodels.Session{}, false, nil
}
func (f *fakeSessionRepository) GetByID(ctx context.Context, id string) (authmodels.Session, bool, error) {
	s, ok := f.sessions[id]
	return s, ok, nil
}
func (f *fakeSessionRepository) RotateRefreshToken(ctx context.Context, id string, hash string) error {
	session, ok := f.sessions[id]
	if !ok || session.RevokedAt != nil {
		return authrepositories.ErrSessionNotFound
	}
	session.RefreshTokenHash = hash
	session.UpdatedAt = time.Now()
	f.sessions[id] = session
	return nil
}
func (f *fakeSessionRepository) Revoke(ctx context.Context, id string) error {
	session, ok := f.sessions[id]
	if !ok || session.RevokedAt != nil {
		return authrepositories.ErrSessionNotFound
	}
	now := time.Now()
	session.RevokedAt = &now
	session.UpdatedAt = now
	f.sessions[id] = session
	return nil
}
func (f *fakeSessionRepository) RevokeAllByDispatchRiderID(ctx context.Context, dispatchRiderID string) (int64, error) {
	var count int64
	for id, s := range f.sessions {
		if s.DispatchRiderID == dispatchRiderID && s.RevokedAt == nil {
			now := time.Now()
			s.RevokedAt = &now
			s.UpdatedAt = now
			f.sessions[id] = s
			count++
		}
	}
	return count, nil
}

// capturePublisher captures both OTP and session events.
type capturePublisher struct {
	otpEvents       []authclients.OTPRequestedEvent
	sessionEvents   []authclients.SessionCreatedEvent
	loggedOutEvents []authclients.LoggedOutEvent
	phoneEvents     []authclients.PhoneChangedEvent
}

func (p *capturePublisher) PublishOTPRequested(ctx context.Context, e authclients.OTPRequestedEvent) error {
	p.otpEvents = append(p.otpEvents, e)
	return nil
}
func (p *capturePublisher) PublishSessionCreated(ctx context.Context, e authclients.SessionCreatedEvent) error {
	p.sessionEvents = append(p.sessionEvents, e)
	return nil
}
func (p *capturePublisher) PublishLoggedOut(ctx context.Context, e authclients.LoggedOutEvent) error {
	p.loggedOutEvents = append(p.loggedOutEvents, e)
	return nil
}
func (p *capturePublisher) PublishPhoneChanged(_ context.Context, event authclients.PhoneChangedEvent) error {
	p.phoneEvents = append(p.phoneEvents, event)
	return nil
}

type countingOTPRepository struct {
	*fakeOTPRepository
	latestCalls int
}

func (r *countingOTPRepository) LatestByPhone(ctx context.Context, phone string) (authmodels.OTP, bool, error) {
	r.latestCalls++
	return r.fakeOTPRepository.LatestByPhone(ctx, phone)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// seedOTPForVerify plants a ready-to-use OTP in repo and returns the plain code.
func seedOTPForVerify(otpUC *OTPUsecase, repo *fakeOTPRepository, phone string) (string, error) {
	_, code, err := otpUC.Create(context.Background(), phone)
	return code, err
}

func newVerifyUsecase(
	otpRepo *fakeOTPRepository,
	identityRepo IdentityRepositoryIface,
	sessionRepo *fakeSessionRepository,
	pub *capturePublisher,
) *AuthUsecase {
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	return NewAuthUsecase(Options{
		OTPUsecase:         otpUC,
		Identities:         identityRepo,
		Sessions:           sessionRepo,
		Publisher:          pub,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		OTPDebug:           false,
	})
}

// IdentityRepositoryIface satisfies both the real and fake identity repos in tests.
type IdentityRepositoryIface interface {
	FindByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, bool, error)
	FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error)
	GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error)
	UpsertByPhone(ctx context.Context, phoneNumber string) (authmodels.Identity, error)
	CreateForSignup(ctx context.Context, phoneNumber, email string) (authmodels.Identity, error)
	UpdatePhone(ctx context.Context, identityID, oldPhone, newPhone string) error
	UpdateEmail(ctx context.Context, identityID, email string) error
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestVerifySuccess(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)

	// Plant a real OTP
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed OTP: %v", err)
	}

	result, err := svc.Verify(context.Background(), VerifyInput{
		PhoneNumber:   "+2348012345678",
		OTPCode:       code,
		CorrelationID: "corr-verify-001",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if result.ProviderID == "" {
		t.Error("ProviderID must not be empty")
	}
	if result.Role != authmodels.RoleDispatchProvider {
		t.Errorf("Role = %q, want %q", result.Role, authmodels.RoleDispatchProvider)
	}
	if result.AccessToken == "" {
		t.Error("AccessToken must not be empty")
	}
	if result.RefreshToken == "" {
		t.Error("RefreshToken must not be empty")
	}
	if result.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", result.TokenType)
	}
	if result.ExpiresInSeconds != 900 {
		t.Errorf("ExpiresInSeconds = %d, want 900", result.ExpiresInSeconds)
	}
	if stored := otpRepo.latest["+2348012345678"]; !stored.Verified {
		t.Fatal("OTP must be marked verified after successful verification")
	}
	if len(sessRepo.sessions) != 1 {
		t.Fatalf("sessions created = %d, want 1", len(sessRepo.sessions))
	}
}

func TestVerifyOTPNotInResult(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	result, err := svc.Verify(context.Background(), VerifyInput{
		PhoneNumber: "+2348012345678",
		OTPCode:     code,
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	// TokenResult must not have OTP-related fields
	_ = result.AccessToken  // must exist
	_ = result.RefreshToken // must exist
	// Role must be dispatch_provider, never otp
	if result.Role != authmodels.RoleDispatchProvider {
		t.Errorf("unexpected role: %q", result.Role)
	}
}

func TestVerifyInvalidPhone(t *testing.T) {
	svc := newVerifyUsecase(newFakeOTPRepository(), newFakeIdentityRepository(), newFakeSessionRepository(), &capturePublisher{})
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "08012345678", OTPCode: "123456"})
	if err == nil {
		t.Fatal("expected validation error for non-E.164 phone")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeValidationFailed {
		t.Errorf("error code = %v, want validation_failed", err)
	}
}

func TestVerifyEmptyPhone(t *testing.T) {
	svc := newVerifyUsecase(newFakeOTPRepository(), newFakeIdentityRepository(), newFakeSessionRepository(), &capturePublisher{})
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "", OTPCode: "123456"})
	if err == nil {
		t.Fatal("expected validation error for empty phone")
	}
}

func TestVerifyInvalidOTPFormat(t *testing.T) {
	svc := newVerifyUsecase(newFakeOTPRepository(), newFakeIdentityRepository(), newFakeSessionRepository(), &capturePublisher{})
	for _, bad := range []string{"12345", "1234567", "abc123", ""} {
		_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: bad})
		if err == nil {
			t.Errorf("OTPCode=%q: expected error, got nil", bad)
		}
	}
}

func TestVerifyInvalidOTPFormatDoesNotReadOTPRepository(t *testing.T) {
	otpRepo := &countingOTPRepository{fakeOTPRepository: newFakeOTPRepository()}
	svc := newVerifyUsecase(otpRepo.fakeOTPRepository, newFakeIdentityRepository(), newFakeSessionRepository(), &capturePublisher{})
	svc.otp.repository = otpRepo

	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "abc123"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if otpRepo.latestCalls != 0 {
		t.Fatalf("LatestByPhone calls = %d, want 0", otpRepo.latestCalls)
	}
}

func TestVerifyNoActiveOTP(t *testing.T) {
	// No OTP in repo — should return otp_invalid
	svc := newVerifyUsecase(newFakeOTPRepository(), newFakeIdentityRepository(), newFakeSessionRepository(), &capturePublisher{})
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "123456"})
	if err == nil {
		t.Fatal("expected error for missing OTP")
	}
}

func TestVerifyWrongOTPIncrementsAttempts(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	_, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	_, err = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "000000"})
	if err == nil {
		t.Fatal("expected error for wrong OTP")
	}

	// Check attempts incremented
	stored := otpRepo.latest["+2348012345678"]
	if stored.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 after one wrong guess", stored.Attempts)
	}
	if stored.Verified {
		t.Fatal("wrong OTP must not mark OTP verified")
	}
	if len(idRepo.identities) != 0 {
		t.Fatalf("identities created = %d, want 0", len(idRepo.identities))
	}
	if len(sessRepo.sessions) != 0 {
		t.Fatalf("sessions created = %d, want 0", len(sessRepo.sessions))
	}
	if len(pub.sessionEvents) != 0 {
		t.Fatalf("session events = %d, want 0", len(pub.sessionEvents))
	}
}

func TestVerifyLockoutAfterThreeFailed(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	_, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, &capturePublisher{})

	// 3 wrong attempts should lock
	for i := 0; i < 3; i++ {
		svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "000000"}) //nolint
	}

	stored := otpRepo.latest["+2348012345678"]
	if stored.LockedUntil == nil {
		t.Error("LockedUntil must be set after 3 failed attempts")
	}
	if stored.LockedUntil != nil && stored.LockedUntil.Before(time.Now()) {
		t.Error("LockedUntil must be in the future")
	}
}

func TestVerifyLockedOTPReturnsRateLimited(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	_, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Lock the OTP manually
	lockTime := time.Now().Add(30 * time.Minute)
	otp := otpRepo.latest["+2348012345678"]
	otp.LockedUntil = &lockTime
	otp.Attempts = 3
	otpRepo.latest["+2348012345678"] = otp

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, &capturePublisher{})
	_, err = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "123456"})
	if err == nil {
		t.Fatal("expected error for locked OTP")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != apperrors.CodeRateLimited {
		t.Errorf("code = %q, want rate_limited", appErr.Code)
	}
}

func TestVerifyLockoutReturnsRateLimitedAndCorrectOTPDoesNotBypass(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, &capturePublisher{})
	for i := 0; i < 3; i++ {
		_, _ = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "000000"})
	}

	_, err = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "000000"})
	requireVerifyErrorCode(t, err, apperrors.CodeRateLimited)

	_, err = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code})
	requireVerifyErrorCode(t, err, apperrors.CodeRateLimited)
}

func TestVerifyExpiredOTP(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	_, err := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Expire the OTP manually
	otp := otpRepo.latest["+2348012345678"]
	otp.ExpiresAt = time.Now().Add(-1 * time.Minute) // already expired
	otpRepo.latest["+2348012345678"] = otp

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, &capturePublisher{})
	_, err = svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: "123456"})
	if err == nil {
		t.Fatal("expected error for expired OTP")
	}
	appErr, _ := err.(*apperrors.Error)
	if appErr.Code != "otp_expired" {
		t.Errorf("code = %q, want otp_expired", appErr.Code)
	}
	if stored := otpRepo.latest["+2348012345678"]; stored.Verified {
		t.Fatal("expired OTP must not be marked verified")
	}
	if len(sessRepo.sessions) != 0 {
		t.Fatalf("sessions created = %d, want 0", len(sessRepo.sessions))
	}
}

func TestVerifyAlreadyVerifiedOTPCannotReuse(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	// Mark it as already verified
	otp := otpRepo.latest["+2348012345678"]
	otp.Verified = true
	otpRepo.latest["+2348012345678"] = otp

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, &capturePublisher{})
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code})
	if err == nil {
		t.Fatal("expected error for already-verified OTP")
	}
	if len(sessRepo.sessions) != 0 {
		t.Fatalf("sessions created = %d, want 0", len(sessRepo.sessions))
	}
}

func TestVerifySuspendedIdentityReturnsForbidden(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	svc := newVerifyUsecase(otpRepo, &suspendedIdentityRepo{}, sessRepo, pub)
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code})
	if err == nil {
		t.Fatal("expected forbidden for suspended identity")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeForbidden {
		t.Errorf("code = %v, want forbidden", err)
	}
	// No session must be created
	if len(sessRepo.sessions) != 0 {
		t.Error("session must not be created for suspended identity")
	}
}

func TestVerifyDeletedIdentityReturnsForbidden(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	svc := newVerifyUsecase(otpRepo, &deletedIdentityRepo{}, sessRepo, pub)
	_, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code})
	if err == nil {
		t.Fatal("expected forbidden for deleted identity")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeForbidden {
		t.Errorf("code = %v, want forbidden", err)
	}
	if len(sessRepo.sessions) != 0 {
		t.Error("session must not be created for deleted identity")
	}
}

func TestVerifySamePhoneSameProviderID(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)

	// First login
	code1, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	r1, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code1})
	if err != nil {
		t.Fatalf("first verify: %v", err)
	}

	// Second login — fresh OTP
	code2, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	r2, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code2})
	if err != nil {
		t.Fatalf("second verify: %v", err)
	}

	if r1.ProviderID != r2.ProviderID {
		t.Errorf("provider_id changed: %q → %q", r1.ProviderID, r2.ProviderID)
	}
}

func TestVerifyNewPhoneCreatesNewIdentity(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)

	code1, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")
	r1, _ := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code1})

	code2, _ := seedOTPForVerify(otpUC, otpRepo, "+2347012345678")
	r2, _ := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2347012345678", OTPCode: code2})

	if r1.ProviderID == r2.ProviderID {
		t.Error("different phones must produce different provider_ids")
	}
	if idRepo.identities["+2347012345678"].Status != authmodels.StatusActive {
		t.Fatalf("new identity status = %q, want active", idRepo.identities["+2347012345678"].Status)
	}
}

func TestVerifyRefreshTokenHashedInSession(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	result, err := svc.Verify(context.Background(), VerifyInput{PhoneNumber: "+2348012345678", OTPCode: code})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	// Session must store a hash, not the plain refresh token
	if len(sessRepo.sessions) == 0 {
		t.Fatal("no session was created")
	}
	for _, sess := range sessRepo.sessions {
		if sess.RefreshTokenHash == result.RefreshToken {
			t.Error("refresh_token_hash must not equal the plain refresh token")
		}
		if len(sess.RefreshTokenHash) == 0 {
			t.Error("refresh_token_hash must not be empty")
		}
	}
}

func TestVerifySessionCreatedEventPublished(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, _ := seedOTPForVerify(otpUC, otpRepo, "+2348012345678")

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	result, err := svc.Verify(context.Background(), VerifyInput{
		PhoneNumber:   "+2348012345678",
		OTPCode:       code,
		CorrelationID: "corr-sess-001",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if len(pub.sessionEvents) != 1 {
		t.Fatalf("expected 1 session event, got %d", len(pub.sessionEvents))
	}
	ev := pub.sessionEvents[0]
	if ev.Event != authclients.TopicSessionCreated {
		t.Errorf("event = %q, want %q", ev.Event, authclients.TopicSessionCreated)
	}
	if ev.ProviderID != result.ProviderID {
		t.Errorf("event.ProviderID = %q, want %q", ev.ProviderID, result.ProviderID)
	}
	if ev.Role != authmodels.RoleDispatchProvider {
		t.Errorf("event.Role = %q, want dispatch_provider", ev.Role)
	}
	if ev.CorrelationID != "corr-sess-001" {
		t.Errorf("event.CorrelationID = %q, want corr-sess-001", ev.CorrelationID)
	}
	if ev.SessionID == "" {
		t.Error("event.SessionID must not be empty")
	}
	if ev.CreatedAt.IsZero() {
		t.Error("event.CreatedAt must be populated")
	}
}

func TestPhoneChangeStartCreatesOTPForNewPhone(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "provider-123",
		PhoneNumber: "+2348012345678",
		Status:      authmodels.StatusActive,
	}
	svc := newVerifyUsecase(otpRepo, idRepo, newFakeSessionRepository(), &capturePublisher{})

	result, err := svc.PhoneChangeStart(context.Background(), PhoneChangeStartInput{
		ProviderID: "provider-123",
		NewPhone:   "+2348098765432",
	})
	if err != nil {
		t.Fatalf("PhoneChangeStart() error = %v", err)
	}
	if result.ExpiresInSeconds == 0 {
		t.Fatal("ExpiresInSeconds must be populated")
	}
	if _, found, err := otpRepo.LatestByPhoneAndPurpose(context.Background(), "+2348098765432", "phone_change"); err != nil || !found {
		t.Fatalf("new-phone phone_change OTP found=%v err=%v", found, err)
	}
	if _, found, err := otpRepo.LatestByPhoneAndPurpose(context.Background(), "+2348012345678", "phone_change"); err != nil || found {
		t.Fatalf("current-phone phone_change OTP found=%v err=%v", found, err)
	}
}

func TestPhoneChangeStartRejectsDuplicatePhone(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	idRepo.identities["+2348012345678"] = authmodels.Identity{ID: "provider-123", PhoneNumber: "+2348012345678", Status: authmodels.StatusActive}
	idRepo.identities["+2348098765432"] = authmodels.Identity{ID: "provider-456", PhoneNumber: "+2348098765432", Status: authmodels.StatusActive}
	svc := newVerifyUsecase(otpRepo, idRepo, newFakeSessionRepository(), &capturePublisher{})

	_, err := svc.PhoneChangeStart(context.Background(), PhoneChangeStartInput{
		ProviderID: "provider-123",
		NewPhone:   "+2348098765432",
	})
	requireVerifyErrorCode(t, err, apperrors.CodeConflict)
}

func TestPhoneChangeVerifyUpdatesPhoneAndPublishesEvent(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	pub := &capturePublisher{}
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "provider-123",
		PhoneNumber: "+2348012345678",
		Status:      authmodels.StatusActive,
	}
	svc := newVerifyUsecase(otpRepo, idRepo, newFakeSessionRepository(), pub)
	newPhone := "+2348098765432"
	_, code, err := svc.otp.CreateForPhoneChange(context.Background(), newPhone)
	if err != nil {
		t.Fatalf("CreateForPhoneChange() error = %v", err)
	}

	result, err := svc.PhoneChangeVerify(context.Background(), PhoneChangeVerifyInput{
		ProviderID:    "provider-123",
		NewPhone:      newPhone,
		OTPCode:       code,
		CorrelationID: "corr-phone",
	})
	if err != nil {
		t.Fatalf("PhoneChangeVerify() error = %v", err)
	}
	if result.PhoneNumber != newPhone {
		t.Fatalf("PhoneNumber = %q, want %q", result.PhoneNumber, newPhone)
	}
	if _, found, _ := idRepo.FindByPhone(context.Background(), "+2348012345678"); found {
		t.Fatal("old phone should no longer resolve")
	}
	identity, found, _ := idRepo.FindByPhone(context.Background(), newPhone)
	if !found || identity.ID != "provider-123" {
		t.Fatalf("new phone identity found=%v identity=%+v", found, identity)
	}
	if len(pub.phoneEvents) != 1 {
		t.Fatalf("phone events = %d, want 1", len(pub.phoneEvents))
	}
	event := pub.phoneEvents[0]
	if event.Event != authclients.TopicPhoneChanged || event.OldPhone != "+2348012345678" || event.NewPhone != newPhone || event.CorrelationID != "corr-phone" {
		t.Fatalf("phone event mismatch: %+v", event)
	}
	if event.ChangedAt.IsZero() {
		t.Fatal("event.ChangedAt must be populated")
	}
}

func requireVerifyErrorCode(t *testing.T, err error, code apperrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", code)
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %q, want %q", appErr.Code, code)
	}
}
