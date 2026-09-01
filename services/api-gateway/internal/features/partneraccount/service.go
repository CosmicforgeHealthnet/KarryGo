package partneraccount

import (
	"context"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

const (
	bcryptCost      = 12
	minPasswordLen  = 8
	defaultKeyLabel = "API Key"
	firstKeyLabel   = "Default Sandbox Key"
	keyWarning      = "Save this key now. It will not be shown again."
)

// Options configures a Service.
type Options struct {
	Accounts          AccountRepository
	Sessions          SessionRepository
	Keys              APIKeyStore
	OTPs              OTPChallengeRepository
	EmailSender       PartnerEmailSender
	OTPGenerator      func() (string, error)
	JWTSecret         string
	JWTTTL            time.Duration
	RefreshTTL        time.Duration
	OTPTTL            time.Duration
	OTPMaxAttempts    int
	OTPResendCooldown time.Duration
	Now               func() time.Time
}

// Service implements partner dashboard auth + key management + internal controls.
type Service struct {
	accounts          AccountRepository
	sessions          SessionRepository
	keys              APIKeyStore
	otps              OTPChallengeRepository
	emailSender       PartnerEmailSender
	otpGenerator      func() (string, error)
	jwtSecret         string
	jwtTTL            time.Duration
	refreshTTL        time.Duration
	otpTTL            time.Duration
	otpMaxAttempts    int
	otpResendCooldown time.Duration
	now               func() time.Time
}

// NewService builds a Service with sensible defaults.
func NewService(o Options) *Service {
	s := &Service{
		accounts:          o.Accounts,
		sessions:          o.Sessions,
		keys:              o.Keys,
		otps:              o.OTPs,
		emailSender:       o.EmailSender,
		otpGenerator:      o.OTPGenerator,
		jwtSecret:         o.JWTSecret,
		jwtTTL:            o.JWTTTL,
		refreshTTL:        o.RefreshTTL,
		otpTTL:            o.OTPTTL,
		otpMaxAttempts:    o.OTPMaxAttempts,
		otpResendCooldown: o.OTPResendCooldown,
		now:               o.Now,
	}
	if s.jwtTTL <= 0 {
		s.jwtTTL = 15 * time.Minute
	}
	if s.jwtTTL > 15*time.Minute {
		s.jwtTTL = 15 * time.Minute
	}
	if s.refreshTTL <= 0 {
		s.refreshTTL = 30 * 24 * time.Hour
	}
	if s.otpTTL <= 0 {
		s.otpTTL = 10 * time.Minute
	}
	if s.otpMaxAttempts <= 0 {
		s.otpMaxAttempts = 5
	}
	if s.otpResendCooldown <= 0 {
		s.otpResendCooldown = time.Minute
	}
	if s.otpGenerator == nil {
		s.otpGenerator = generateOTPCode
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// --- DTOs --------------------------------------------------------------------

type RegisterInput struct {
	BusinessName  string `json:"business_name"`
	BusinessEmail string `json:"business_email"`
	Password      string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignupEmailInput struct {
	BusinessEmail string `json:"business_email"`
}

type SignupVerifyInput struct {
	BusinessEmail string `json:"business_email"`
	OTPCode       string `json:"otp_code"`
}

// CreateKeyInput is the POST /keys body (the Generate API Key form).
// Expiration accepts either a short code ("30d", "90d", "1y") or the literal
// dropdown label the frontend currently sends ("30 days", "90 days", "1 year"),
// or "" / "No expiration" for a key that never expires. See parseExpiration.
type CreateKeyInput struct {
	Label       string   `json:"label"`
	Environment string   `json:"environment"`
	Description string   `json:"description"`
	Expiration  string   `json:"expiration"`
	Permissions []string `json:"permissions"`
}

// UpdateProfileInput is the PATCH /profile body (Settings > Profile edit form).
// Every field is a pointer so the handler can tell "not sent" (nil, leave
// unchanged) apart from "sent as empty string" (non-nil pointer to "", clears
// the field) - a plain string field could not make that distinction.
type UpdateProfileInput struct {
	BusinessName    *string `json:"business_name"`
	BusinessPhone   *string `json:"business_phone"`
	State           *string `json:"state"`
	City            *string `json:"city"`
	BusinessLogoURL *string `json:"business_logo_url"`
	ProfilePhotoURL *string `json:"profile_photo_url"`
}

// IssuedAPIKey is the one-time key creation payload (raw key shown once).
type IssuedAPIKey struct {
	ID          string     `json:"id,omitempty"`
	Key         string     `json:"key"`
	Prefix      string     `json:"prefix"`
	Environment string     `json:"environment,omitempty"`
	Description string     `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	Warning     string     `json:"warning"`
}

// RegisterResult is returned by Register.
type RegisterResult struct {
	RefreshToken     string `json:"-"`
	RefreshExpiresAt time.Time
	Account          Account
	Token            string
	ExpiresAt        time.Time
	APIKey           IssuedAPIKey
}

// SessionResult is returned by Login.
type SessionResult struct {
	RefreshToken     string `json:"-"`
	RefreshExpiresAt time.Time
	Account          Account
	Token            string
	ExpiresAt        time.Time
}

// --- request context --------------------------------------------------------

// RequestMeta carries request metadata to persist with a session.
type RequestMeta struct {
	IPAddress string
	UserAgent string
}

// --- errors ------------------------------------------------------------------

func apiErr(status int, code, message string) *delivery.APIError {
	return delivery.NewAPIError(status, code, message, nil)
}

func apiErrWithMeta(status int, code, message string, meta map[string]any) *delivery.APIError {
	return delivery.NewAPIErrorWithMeta(status, code, message, nil, meta)
}

func validationErr(field, message string) *delivery.APIError {
	return delivery.ValidationError([]delivery.FieldError{{Field: field, Message: message}})
}

// --- dashboard auth ----------------------------------------------------------

// Register creates a partner account, its first sandbox key, and a session.
//
// The first sandbox key is granted ALL permissions (partnerauth.AllPermissions),
// not whatever a form would have picked - there is no Generate API Key form at
// registration time, so it cannot be scoped down by user choice, and issuing it
// with zero permissions would make it unusable against every permission-gated
// route the moment signup finishes.
func (s *Service) Register(ctx context.Context, in RegisterInput, meta RequestMeta) (*RegisterResult, error) {
	name := strings.TrimSpace(in.BusinessName)
	email := normalizeEmail(in.BusinessEmail)
	if name == "" {
		return nil, validationErr("business_name", "Business name is required.")
	}
	if !validEmail(email) {
		return nil, validationErr("business_email", "A valid business email is required.")
	}
	if len(in.Password) < minPasswordLen {
		return nil, validationErr("password", "Password must be at least 8 characters.")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	account, err := s.accounts.CreateAccount(ctx, CreateAccountParams{
		BusinessName:  &name,
		BusinessEmail: email,
		PasswordHash:  ptrString(string(passwordHash)),
		Status:        StatusActive,
		Plan:          PlanSandbox,
	})
	if err == ErrEmailTaken {
		return nil, apiErr(http.StatusConflict, "email_taken", "An account with this email already exists.")
	}
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	// First sandbox API key (shown once). Full permissions - see doc comment above.
	issued, err := s.issueKey(ctx, account.ID, EnvironmentSandbox, firstKeyLabel,
		"Auto-generated sandbox key for getting started.", nil, partnerauth.AllPermissions())
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	credentials, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	return &RegisterResult{Account: account, Token: credentials.Token, ExpiresAt: credentials.ExpiresAt, RefreshToken: credentials.RefreshToken, RefreshExpiresAt: credentials.RefreshExpiresAt, APIKey: issued}, nil
}

// Login verifies credentials and issues a session. It never reveals whether the
// email exists.
func (s *Service) Login(ctx context.Context, in LoginInput, meta RequestMeta) (*SessionResult, error) {
	email := normalizeEmail(in.Email)
	if email == "" || in.Password == "" {
		return nil, apiErr(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
	}

	account, passwordHash, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if account == nil {
		return nil, apiErr(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
	}
	if account.Status != StatusActive {
		return nil, apiErr(http.StatusForbidden, "account_inactive", "This account is not active.")
	}
	if passwordHash == nil || bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(in.Password)) != nil {
		return nil, apiErr(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
	}

	credentials, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	credentials.Account = *account
	return credentials, nil
}

// Logout revokes a session by its token hash.
func (s *Service) Logout(ctx context.Context, tokenHash string) error {
	if err := s.sessions.Revoke(ctx, tokenHash); err != nil {
		return delivery.ErrInternal()
	}
	return nil
}

// Me returns the account for a partner id.
func (s *Service) Me(ctx context.Context, partnerID string) (*Account, error) {
	account, err := s.accounts.FindByID(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if account == nil {
		return nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}
	return account, nil
}

// UpdateProfile applies the Settings > Profile edit form's changes. Only fields
// present (non-nil) in the input are updated; omitted fields are left as-is.
func (s *Service) UpdateProfile(ctx context.Context, partnerID string, in UpdateProfileInput) (*Account, error) {
	if in.BusinessName != nil {
		trimmed := strings.TrimSpace(*in.BusinessName)
		if trimmed == "" {
			return nil, validationErr("business_name", "Business name cannot be empty.")
		}
		in.BusinessName = &trimmed
	}

	account, found, err := s.accounts.UpdateProfile(ctx, partnerID, UpdateProfileParams{
		BusinessName:    in.BusinessName,
		BusinessPhone:   in.BusinessPhone,
		State:           in.State,
		City:            in.City,
		BusinessLogoURL: in.BusinessLogoURL,
		ProfilePhotoURL: in.ProfilePhotoURL,
	})
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if !found {
		return nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}
	return account, nil
}

// DeleteAccount permanently deletes a partner account and all associated data.
func (s *Service) DeleteAccount(ctx context.Context, partnerID string) error {
	found, err := s.accounts.DeleteAccount(ctx, partnerID)
	if err != nil {
		return delivery.ErrInternal()
	}
	if !found {
		return apiErr(http.StatusNotFound, "partner_not_found", "Partner not found.")
	}
	return nil
}

// AuthenticateSession validates a raw session JWT against the DB. It returns the
// partner id, the token hash, and the loaded account. Used by RequirePartnerSession.
func (s *Service) AuthenticateSession(ctx context.Context, rawToken string) (partnerID, tokenHash string, account *Account, err error) {
	pid, perr := parseSessionJWT(s.jwtSecret, rawToken, s.now())
	if perr != nil {
		return "", "", nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}
	th := hashToken(rawToken)

	session, ferr := s.sessions.FindByTokenHash(ctx, th)
	if ferr != nil {
		return "", "", nil, delivery.ErrInternal()
	}
	if session == nil {
		return "", "", nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}
	if session.RevokedAt != nil {
		return "", "", nil, apiErr(http.StatusUnauthorized, "session_revoked", "This session has been revoked.")
	}
	if !session.ExpiresAt.After(s.now()) {
		return "", "", nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}

	acc, aerr := s.accounts.FindByID(ctx, pid)
	if aerr != nil {
		return "", "", nil, delivery.ErrInternal()
	}
	if acc == nil {
		return "", "", nil, apiErr(http.StatusUnauthorized, "invalid_token", "Session expired or invalid.")
	}
	if !isActive(acc.Status) {
		return "", "", nil, apiErr(http.StatusForbidden, "partner_account_inactive", "This partner account is not active.")
	}
	return pid, th, acc, nil
}

// --- API key management ------------------------------------------------------

// CreateKey issues a new API key for a partner. Live keys require a live plan.

func (s *Service) CreateKey(ctx context.Context, partnerID, plan string, in CreateKeyInput) (*IssuedAPIKey, error) {
	env := strings.TrimSpace(strings.ToLower(in.Environment))
	if env == "" {
		env = EnvironmentSandbox
	}
	if env != EnvironmentSandbox && env != EnvironmentLive {
		return nil, validationErr("environment", "Environment must be sandbox or live.")
	}
	if env == EnvironmentLive && plan != PlanLive {
		return nil, apiErr(http.StatusForbidden, "sandbox_only", "Live API keys require an activated live plan.")
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = defaultKeyLabel
	}

	for _, p := range in.Permissions {
		if !partnerauth.ValidPermission(p) {
			return nil, validationErr("permissions", "Unknown permission: "+p)
		}
	}

	expiresAt, err := parseExpiration(in.Expiration, s.now())
	if err != nil {
		return nil, validationErr("expiration", "Unrecognized expiration option.")
	}

	issued, err := s.issueKey(ctx, partnerID, env, label, strings.TrimSpace(in.Description), expiresAt, in.Permissions)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	return &issued, nil
}

// ListKeys returns a partner's keys (safe fields only).
func (s *Service) ListKeys(ctx context.Context, partnerID string) ([]KeyView, error) {
	views, err := s.keys.ListByPartner(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if views == nil {
		views = []KeyView{}
	}
	return views, nil
}

// RevokeKey revokes a partner's own key.
func (s *Service) RevokeKey(ctx context.Context, partnerID, keyID string) error {
	found, err := s.keys.RevokeByPartner(ctx, partnerID, keyID)
	if err != nil {
		return delivery.ErrInternal()
	}
	if !found {
		return apiErr(http.StatusNotFound, "not_found", "Key not found.")
	}
	return nil
}

// --- internal controls -------------------------------------------------------

// ActivateLive upgrades a partner to the live plan.
func (s *Service) ActivateLive(ctx context.Context, partnerID string) error {
	found, err := s.accounts.UpdatePlan(ctx, partnerID, PlanLive)
	if err != nil {
		return delivery.ErrInternal()
	}
	if !found {
		return apiErr(http.StatusNotFound, "partner_not_found", "Partner not found.")
	}
	return nil
}

// UpdateStatus sets a partner's account status (internal).
func (s *Service) UpdateStatus(ctx context.Context, partnerID, status string) error {
	status = strings.TrimSpace(strings.ToLower(status))
	switch status {
	case StatusActive, StatusSuspended, StatusDisabled:
	default:
		return validationErr("status", "Status must be one of: active, suspended, disabled.")
	}
	found, err := s.accounts.UpdateStatus(ctx, partnerID, status)
	if err != nil {
		return delivery.ErrInternal()
	}
	if !found {
		return apiErr(http.StatusNotFound, "partner_not_found", "Partner not found.")
	}
	return nil
}

// --- helpers -----------------------------------------------------------------

func (s *Service) issueKey(ctx context.Context, partnerID, env, label, description string, expiresAt *time.Time, permissions []string) (IssuedAPIKey, error) {
	raw, prefix, err := generateRawAPIKey(env)
	if err != nil {
		return IssuedAPIKey{}, err
	}
	keyHash, err := bcrypt.GenerateFromPassword([]byte(raw), bcryptCost)
	if err != nil {
		return IssuedAPIKey{}, err
	}
	if permissions == nil {
		permissions = []string{}
	}
	id, err := s.keys.CreateKey(ctx, CreateKeyParams{
		PartnerID:   partnerID,
		KeyHash:     string(keyHash),
		KeyPrefix:   prefix,
		Environment: env,
		Label:       label,
		Status:      "active",
		Description: description,
		ExpiresAt:   expiresAt,
		Permissions: permissions,
	})
	if err != nil {
		return IssuedAPIKey{}, err
	}
	return IssuedAPIKey{
		ID:          id,
		Key:         raw,
		Prefix:      prefix,
		Environment: env,
		Description: description,
		ExpiresAt:   expiresAt,
		Permissions: permissions,
		Warning:     keyWarning,
	}, nil
}

func (s *Service) issueSession(ctx context.Context, partnerID string, meta RequestMeta) (*SessionResult, error) {
	credentials, params, err := s.newCredentials(partnerID, "", meta)
	if err != nil {
		return nil, err
	}
	if err = s.sessions.CreateSession(ctx, params); err != nil {
		return nil, err
	}
	return credentials, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ptrString(value string) *string { return &value }

func validEmail(email string) bool {
	if email == "" {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

// parseExpiration turns the Generate API Key form's Expiration selection into
// an absolute *time.Time (nil = never expires). It accepts both a short code
// ("30d"/"90d"/"1y") and the literal option label the frontend currently sends
// ("30 days"/"90 days"/"1 year"/"No expiration") - the frontend was built
// against display strings as option values rather than machine-readable codes,
// so this stays permissive rather than breaking every key creation until the
// frontend is updated to send codes instead of labels.
func parseExpiration(input string, now time.Time) (*time.Time, error) {
	switch strings.TrimSpace(strings.ToLower(input)) {
	case "", "no expiration", "none":
		return nil, nil
	case "30 days", "30d":
		t := now.AddDate(0, 0, 30)
		return &t, nil
	case "90 days", "90d":
		t := now.AddDate(0, 0, 90)
		return &t, nil
	case "1 year", "1y":
		t := now.AddDate(1, 0, 0)
		return &t, nil
	default:
		return nil, errUnrecognizedExpiration
	}
}

var errUnrecognizedExpiration = errUnrecognizedExpirationErr{}

type errUnrecognizedExpirationErr struct{}

func (errUnrecognizedExpirationErr) Error() string { return "unrecognized expiration option" }
