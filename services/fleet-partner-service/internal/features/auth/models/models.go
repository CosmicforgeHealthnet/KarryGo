// Package models holds the corporate authentication domain models.
//
// KarryGo corporate authentication is passwordless: there is no password
// field, hash, reset or temporary credential anywhere in this package. A
// corporate identity is an email + phone pair verified by an emailed OTP.
package models

import "time"

// Corporate identity statuses.
const (
	IdentityStatusPending   = "PENDING"
	IdentityStatusActive    = "ACTIVE"
	IdentityStatusSuspended = "SUSPENDED"
	IdentityStatusLocked    = "LOCKED"
	IdentityStatusArchived  = "ARCHIVED"
)

// OTP challenge purposes.
const (
	PurposeSignUpOrLogin         = "SIGN_UP_OR_LOGIN"
	PurposeStaffInvitation       = "STAFF_INVITATION"
	PurposeOrganizationSelection = "ORGANIZATION_SELECTION"
	PurposeStepUp                = "STEP_UP"
	PurposeContactChange         = "CONTACT_CHANGE"
	PurposeProviderInvitation    = "PROVIDER_INVITATION"
)

// OTP challenge statuses.
const (
	ChallengeStatusPending    = "PENDING"
	ChallengeStatusVerified   = "VERIFIED"
	ChallengeStatusConsumed   = "CONSUMED"
	ChallengeStatusExpired    = "EXPIRED"
	ChallengeStatusSuperseded = "SUPERSEDED"
	ChallengeStatusLocked     = "LOCKED"
)

// Delivery statuses shared by OTP and invitation delivery.
const (
	DeliveryStatusPending     = "PENDING"
	DeliveryStatusAccepted    = "ACCEPTED"
	DeliveryStatusFailed      = "FAILED"
	DeliveryStatusUnavailable = "UNAVAILABLE"
)

// Session statuses.
const (
	SessionStatusActive      = "ACTIVE"
	SessionStatusRevoked     = "REVOKED"
	SessionStatusExpired     = "EXPIRED"
	SessionStatusCompromised = "COMPROMISED"
)

// Identity is a corporate person who may belong to several organizations.
// It deliberately carries no credential material.
type Identity struct {
	ID                        string
	EmailNormalized           string
	EmailDisplay              string
	PhoneE164                 string
	PhoneDisplay              string
	Status                    string
	EmailVerifiedAt           *time.Time
	PhoneBoundAt              *time.Time
	LastAuthenticatedAt       *time.Time
	FailedAuthenticationCount int
	LockedUntil               *time.Time
	Version                   int64
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	ArchivedAt                *time.Time
}

// ContactVerified reports whether the emailed OTP was verified and the
// normalized phone was bound to this identity.
func (i Identity) ContactVerified() bool {
	return i.EmailVerifiedAt != nil && i.PhoneBoundAt != nil
}

// Usable reports whether the identity may authenticate right now.
func (i Identity) Usable(now time.Time) bool {
	if i.Status != IdentityStatusActive && i.Status != IdentityStatusPending {
		return false
	}
	if i.LockedUntil != nil && now.Before(*i.LockedUntil) {
		return false
	}
	return true
}

// Challenge is a durable OTP challenge. The raw code is never stored — only
// CodeDigest, a keyed HMAC of (challengeID, phone, code).
type Challenge struct {
	ID                    string
	IdentityID            *string
	EmailNormalized       string
	EmailHash             string
	PhoneE164             string
	PhoneHash             string
	Purpose               string
	Status                string
	CodeDigest            string
	ExpiresAt             time.Time
	ResendAvailableAt     time.Time
	FailedAttempts        int
	MaximumAttempts       int
	DeliveryAttempts      int
	DeliveryStatus        string
	NotificationReference *string
	SessionID             *string
	OrganizationID        *string
	ActionClass           *string
	InvitationID          *string
	RequestedIPHash       *string
	UserAgentHash         *string
	RequestID             string
	CorrelationID         string
	VerifiedAt            *time.Time
	ConsumedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Usable reports whether the challenge may still be verified.
func (c Challenge) Usable(now time.Time) bool {
	return c.Status == ChallengeStatusPending &&
		now.Before(c.ExpiresAt) &&
		c.FailedAttempts < c.MaximumAttempts
}

// Session is a durable corporate session with a rotating refresh token.
// Only the refresh token hash is persisted.
type Session struct {
	ID                 string
	IdentityID         string
	OrganizationUserID *string
	OrganizationID     *string
	SessionFamilyID    string
	RefreshTokenHash   string
	Status             string
	AccessIssuedAt     time.Time
	RefreshExpiresAt   time.Time
	LastSeenAt         time.Time
	IPHash             *string
	UserAgentHash      *string
	DeviceLabel        *string
	RoleVersion        int64
	RevokedAt          *time.Time
	RevocationReason   *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// OrganizationScoped reports whether the session carries organization scope.
func (s Session) OrganizationScoped() bool {
	return s.OrganizationID != nil && *s.OrganizationID != ""
}

// SecurityEvent is an append-only authentication security record. It stores
// hashes rather than raw contact values.
type SecurityEvent struct {
	ID             string
	EventType      string
	Severity       string
	IdentityID     *string
	OrganizationID *string
	EmailHash      *string
	PhoneHash      *string
	IPHash         *string
	UserAgentHash  *string
	Detail         map[string]any
	RequestID      string
	CorrelationID  string
	OccurredAt     time.Time
}

// Security event types.
const (
	SecurityEventIdentityPairConflict = "identity_pair_conflict"
	SecurityEventOTPAttemptExceeded   = "otp_attempt_limit_exceeded"
	SecurityEventRefreshReuse         = "refresh_token_reused"
	SecurityEventSuspendedAttempt     = "suspended_identity_attempt"
	SecurityEventRateLimited          = "authentication_rate_limited"
	SecurityEventOTPDeliveryFailed    = "otp_delivery_failed"
)

// Severities.
const (
	SeverityInfo     = "INFO"
	SeverityWarn     = "WARN"
	SeverityCritical = "CRITICAL"
)
