package securehandover

import (
	"errors"
	"strings"
	"time"
)

type Stage string

const (
	StagePickup           Stage = "PICKUP"
	StageDelivery         Stage = "DELIVERY"
	StageProviderTransfer Stage = "PROVIDER_TRANSFER"
	StageReturn           Stage = "RETURN"
)

type Status string

const (
	StatusActive      Status = "ACTIVE"
	StatusConsumed    Status = "CONSUMED"
	StatusInvalidated Status = "INVALIDATED"
	StatusExpired     Status = "EXPIRED"
	StatusLocked      Status = "LOCKED"
)

type Method string

const (
	MethodOTP Method = "OTP"
	MethodQR  Method = "QR"
)

type Policy struct {
	Enabled                   bool
	PickupEvidenceEnabled     bool
	DeliveryEvidenceEnabled   bool
	QREnabled                 bool
	TTL                       time.Duration
	CaptureTTL                time.Duration
	ResendCooldown            time.Duration
	MaxAttempts               int
	MaxResends                int
	PolicyVersion             string
	EmailAllowed              bool
	InAppAllowed              bool
	PushAllowed               bool
	SMSAllowed                bool
	ReconciliationLease       time.Duration
	ReconciliationMaxAttempts int
	ReconciliationBackoffBase time.Duration
	ReconciliationBackoffMax  time.Duration
}

func (p Policy) normalized() Policy {
	if p.TTL <= 0 {
		p.TTL = 15 * time.Minute
	}
	if p.CaptureTTL <= 0 {
		p.CaptureTTL = 10 * time.Minute
	}
	if p.ResendCooldown <= 0 {
		p.ResendCooldown = 45 * time.Second
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 5
	}
	if p.MaxResends <= 0 {
		p.MaxResends = 5
	}
	if strings.TrimSpace(p.PolicyVersion) == "" {
		p.PolicyVersion = "dispatch_secure_handover_v1"
	}
	if p.ReconciliationLease <= 0 {
		p.ReconciliationLease = time.Minute
	}
	if p.ReconciliationMaxAttempts <= 0 {
		p.ReconciliationMaxAttempts = 10
	}
	if p.ReconciliationBackoffBase <= 0 {
		p.ReconciliationBackoffBase = 5 * time.Second
	}
	if p.ReconciliationBackoffMax < p.ReconciliationBackoffBase {
		p.ReconciliationBackoffMax = 15 * time.Minute
	}
	return p
}

type Context struct {
	DeliveryGroupID string
	MissionID       string
	MissionRef      string
	StopID          string
	StopRef         string
	TripID          string
	BookingID       string
	ManifestID      string
	ManifestRef     string
	ManifestVersion int64
	ProviderID      string
	CustomerID      string
	SourceType      string
	SourceRef       string
	SourceVersion   int64
	ReceiverRef     string
	ReceiverEmail   string
	ReceiverPhone   string
}

func (c Context) ContextKey(stage Stage) string {
	return strings.Join([]string{c.MissionRef, c.StopRef, string(stage), c.ManifestRef, integer(c.ManifestVersion)}, ":")
}

type Challenge struct {
	ID                       string     `json:"-"`
	ChallengeRef             string     `json:"challenge_ref"`
	ContextKey               string     `json:"-"`
	Version                  int64      `json:"version"`
	Stage                    Stage      `json:"stage"`
	MissionRef               string     `json:"mission_ref"`
	StopRef                  string     `json:"stop_ref"`
	TripID                   string     `json:"trip_id"`
	BookingID                string     `json:"booking_id,omitempty"`
	ManifestRef              string     `json:"manifest_ref"`
	ManifestVersion          int64      `json:"manifest_version"`
	AssignedProviderID       string     `json:"-"`
	IntendedRecipientRef     string     `json:"-"`
	RecipientType            string     `json:"recipient_type"`
	MaskedDestination        string     `json:"masked_destination,omitempty"`
	OTPHash                  string     `json:"-"`
	QRTokenHash              string     `json:"-"`
	ExpiresAt                time.Time  `json:"expires_at"`
	AttemptCount             int        `json:"attempt_count"`
	MaxAttempts              int        `json:"max_attempts"`
	ResendCount              int        `json:"resend_count"`
	CooldownUntil            *time.Time `json:"resend_cooldown_until,omitempty"`
	Status                   Status     `json:"status"`
	NotificationState        string     `json:"notification_state"`
	NotificationChannels     []string   `json:"notification_channels,omitempty"`
	NotificationAttemptCount int        `json:"-"`
	SourceVersion            int64      `json:"source_version"`
	OptimisticVersion        int64      `json:"optimistic_version"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type PendingNotification struct {
	ChallengeRef string
	AttemptCount int
}

type Credential struct {
	ChallengeRef    string    `json:"challenge_ref"`
	Version         int64     `json:"version"`
	Stage           Stage     `json:"stage"`
	OTP             string    `json:"otp,omitempty"`
	QRToken         string    `json:"qr_token,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
	ManifestRef     string    `json:"manifest_ref"`
	ManifestVersion int64     `json:"manifest_version"`
}

type Actor struct {
	Type      string
	Ref       string
	SourceRef string
}

type VerifyCommand struct {
	ChallengeRef          string `json:"challenge_ref"`
	Method                Method `json:"method"`
	Credential            string `json:"credential"`
	MissionRef            string `json:"mission_ref,omitempty"`
	StopRef               string `json:"stop_ref,omitempty"`
	ManifestRef           string `json:"manifest_ref"`
	ManifestVersion       int64  `json:"manifest_version"`
	ExpectedSourceVersion int64  `json:"expected_source_version,omitempty"`
	OperationRef          string `json:"-"`
}

func (c VerifyCommand) Validate() error {
	if strings.TrimSpace(c.ChallengeRef) == "" || strings.TrimSpace(c.Credential) == "" || strings.TrimSpace(c.OperationRef) == "" {
		return errors.New("challenge, credential and operation reference are required")
	}
	if c.Method != MethodOTP && c.Method != MethodQR {
		return errors.New("verification method must be OTP or QR")
	}
	if strings.TrimSpace(c.ManifestRef) == "" || c.ManifestVersion < 1 {
		return errors.New("exact manifest reference and version are required")
	}
	return nil
}

type ResendCommand struct {
	ChallengeRef string `json:"challenge_ref"`
	OperationRef string `json:"-"`
}

type CaptureSessionCommand struct {
	Stage           Stage  `json:"stage"`
	EvidenceType    string `json:"evidence_type"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	OperationRef    string `json:"-"`
}

type CaptureSession struct {
	Ref             string    `json:"capture_session_ref"`
	ChallengeRef    string    `json:"challenge_ref"`
	Stage           Stage     `json:"stage"`
	EvidenceType    string    `json:"evidence_type"`
	ManifestRef     string    `json:"manifest_ref"`
	ManifestVersion int64     `json:"manifest_version"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type ScanCommand struct {
	Stage           Stage  `json:"stage,omitempty"`
	ParcelCode      string `json:"parcel_code"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	OperationRef    string `json:"-"`
}

type EvidenceCommand struct {
	CaptureSessionRef string    `json:"capture_session_ref"`
	MediaRef          string    `json:"media_ref"`
	CapturedAt        time.Time `json:"captured_at"`
	Latitude          *float64  `json:"latitude,omitempty"`
	Longitude         *float64  `json:"longitude,omitempty"`
	ReasonCode        string    `json:"reason_code,omitempty"`
	SupersedesRef     string    `json:"supersedes_evidence_ref,omitempty"`
	OperationRef      string    `json:"-"`
}

type Evidence struct {
	EvidenceRef       string    `json:"evidence_ref"`
	MediaRef          string    `json:"media_ref"`
	Stage             Stage     `json:"stage"`
	EvidenceType      string    `json:"evidence_type"`
	ManifestRef       string    `json:"manifest_ref"`
	ManifestVersion   int64     `json:"manifest_version"`
	CapturedAt        time.Time `json:"captured_at"`
	CustodyTransition string    `json:"custody_transition,omitempty"`
}

func integer(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}
