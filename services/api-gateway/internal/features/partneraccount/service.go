package partneraccount

import (
	"context"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
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
	Accounts  AccountRepository
	Sessions  SessionRepository
	Keys      APIKeyStore
	JWTSecret string
	JWTTTL    time.Duration
	Now       func() time.Time
}

// Service implements partner dashboard auth + key management + internal controls.
type Service struct {
	accounts  AccountRepository
	sessions  SessionRepository
	keys      APIKeyStore
	jwtSecret string
	jwtTTL    time.Duration
	now       func() time.Time
}

// NewService builds a Service with sensible defaults.
func NewService(o Options) *Service {
	s := &Service{
		accounts:  o.Accounts,
		sessions:  o.Sessions,
		keys:      o.Keys,
		jwtSecret: o.JWTSecret,
		jwtTTL:    o.JWTTTL,
		now:       o.Now,
	}
	if s.jwtTTL <= 0 {
		s.jwtTTL = 24 * time.Hour
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

type CreateKeyInput struct {
	Label       string `json:"label"`
	Environment string `json:"environment"`
}

// IssuedAPIKey is the one-time key creation payload (raw key shown once).
type IssuedAPIKey struct {
	ID          string `json:"id,omitempty"`
	Key         string `json:"key"`
	Prefix      string `json:"prefix"`
	Environment string `json:"environment,omitempty"`
	Warning     string `json:"warning"`
}

// RegisterResult is returned by Register.
type RegisterResult struct {
	Account   Account
	Token     string
	ExpiresAt time.Time
	APIKey    IssuedAPIKey
}

// SessionResult is returned by Login.
type SessionResult struct {
	Account   Account
	Token     string
	ExpiresAt time.Time
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

func validationErr(field, message string) *delivery.APIError {
	return delivery.ValidationError([]delivery.FieldError{{Field: field, Message: message}})
}

// --- dashboard auth ----------------------------------------------------------

// Register creates a partner account, its first sandbox key, and a session.
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
		BusinessName:  name,
		BusinessEmail: email,
		PasswordHash:  string(passwordHash),
		Status:        StatusActive,
		Plan:          PlanSandbox,
	})
	if err == ErrEmailTaken {
		return nil, apiErr(http.StatusConflict, "email_taken", "An account with this email already exists.")
	}
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	// First sandbox API key (shown once).
	issued, err := s.issueKey(ctx, account.ID, EnvironmentSandbox, firstKeyLabel)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	token, exp, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	return &RegisterResult{Account: account, Token: token, ExpiresAt: exp, APIKey: issued}, nil
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
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(in.Password)) != nil {
		return nil, apiErr(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
	}

	token, exp, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	return &SessionResult{Account: *account, Token: token, ExpiresAt: exp}, nil
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
	issued, err := s.issueKey(ctx, partnerID, env, label)
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

func (s *Service) issueKey(ctx context.Context, partnerID, env, label string) (IssuedAPIKey, error) {
	raw, prefix, err := generateRawAPIKey(env)
	if err != nil {
		return IssuedAPIKey{}, err
	}
	keyHash, err := bcrypt.GenerateFromPassword([]byte(raw), bcryptCost)
	if err != nil {
		return IssuedAPIKey{}, err
	}
	id, err := s.keys.CreateKey(ctx, CreateKeyParams{
		PartnerID:   partnerID,
		KeyHash:     string(keyHash),
		KeyPrefix:   prefix,
		Environment: env,
		Label:       label,
		Status:      "active",
	})
	if err != nil {
		return IssuedAPIKey{}, err
	}
	return IssuedAPIKey{ID: id, Key: raw, Prefix: prefix, Environment: env, Warning: keyWarning}, nil
}

func (s *Service) issueSession(ctx context.Context, partnerID string, meta RequestMeta) (string, time.Time, error) {
	token, exp, err := signSessionJWT(s.jwtSecret, partnerID, s.jwtTTL, s.now())
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.sessions.CreateSession(ctx, CreateSessionParams{
		PartnerID: partnerID,
		TokenHash: hashToken(token),
		IPAddress: meta.IPAddress,
		UserAgent: meta.UserAgent,
		ExpiresAt: exp,
	}); err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if email == "" {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}
