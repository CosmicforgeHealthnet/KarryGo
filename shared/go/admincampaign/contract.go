// Package admincampaign is the HQ -> KarryGo marketing campaign contract
// (Sections 17 and 25). An Intent is an approved, immutable campaign revision;
// a Status is notification-service's truthful lifecycle and counts. The two
// are distinct: accepting an intent never claims anything was delivered.
package admincampaign

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"time"

	"github.com/google/uuid"
)

const (
	Contract = "karryam.campaign-intent.v1"
	Owner    = "notification-service"
	Product  = "karryam"
	// Paths on notification-service's private HMAC surface.
	IntentPath = "/internal/notification/v1/admin/campaigns"
	MaxBody    = 16 << 10
)

var (
	ErrInvalid  = errors.New("invalid campaign intent")
	ErrConflict = errors.New("campaign revision conflict")
)

var opaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`)
var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var clock = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Closed vocabularies. Emergency/safety broadcasts are NOT a marketing
// category and are refused on this contract (separate higher authority).
var Channels = []string{"in_app", "push", "email"}
var Categories = []string{"MARKETING", "PRODUCT_ANNOUNCEMENT"}
var Audiences = []string{"CUSTOMERS_OPTED_IN"}
var LegalBases = []string{"CONSENT"}

// Lifecycle states (Section 17B). DELIVERED is reachable only with provider
// receipts; without them a revision completes as SENT/COMPLETED with
// delivery_confirmation=UNAVAILABLE.
var States = []string{"ACCEPTED_BY_KARRYGO", "PLANNED", "QUEUED", "SENT", "DELIVERED", "COMPLETED", "REJECTED", "FAILED", "SUPPRESSED", "CANCELLED"}

type Approval struct {
	ApprovalID  string    `json:"approval_id"`
	ApproverRef string    `json:"approver_ref"`
	ApprovedAt  time.Time `json:"approved_at"`
}

type QuietHours struct {
	Start    string `json:"start"` // HH:MM local
	End      string `json:"end"`
	Timezone string `json:"timezone"` // IANA
}

// Intent is one immutable revision. Content is a reference to an existing
// notification-service template plus its hash; HQ never supplies HTML/script.
type Intent struct {
	ContractVersion          string      `json:"contract_version"`
	CampaignID               string      `json:"campaign_id"`
	Revision                 int64       `json:"revision"`
	IdempotencyKey           string      `json:"idempotency_key"`
	Product                  string      `json:"product"`
	RequestingDepartment     string      `json:"requesting_department"`
	RequestedBy              string      `json:"requested_by"`
	Approvals                []Approval  `json:"approvals"`
	Category                 string      `json:"category"`
	Markets                  []string    `json:"markets"`
	Zones                    []string    `json:"zones"`
	Audience                 string      `json:"audience"`
	AudienceDefinitionSHA256 string      `json:"audience_definition_sha256"`
	LegalBasis               string      `json:"legal_basis"`
	Channels                 []string    `json:"channels"`
	TemplateKey              string      `json:"template_key"`
	TemplateLocale           string      `json:"template_locale"`
	TemplateRevision         int64       `json:"template_revision"`
	ContentSHA256            string      `json:"content_sha256"`
	StartAt                  time.Time   `json:"start_at"`
	EndAt                    time.Time   `json:"end_at"`
	MaxPerRecipientPerDay    int64       `json:"max_per_recipient_per_day"`
	MaxMessages              int64       `json:"max_messages"`
	BudgetMinor              int64       `json:"budget_minor"`
	Currency                 string      `json:"currency"`
	Priority                 string      `json:"priority"`
	QuietHours               *QuietHours `json:"quiet_hours"`
	CorrelationID            string      `json:"correlation_id"`
}

func in(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Validate applies contract rules. Policy facts KarryGo does not own (legal
// age thresholds, budgets, quiet-hour defaults) are never filled in here.
func (i Intent) Validate(now time.Time) error {
	id, err := uuid.Parse(i.CampaignID)
	if i.ContractVersion != Contract || err != nil || id.String() != i.CampaignID || i.Revision < 1 || !opaque.MatchString(i.IdempotencyKey) || i.Product != Product || !opaque.MatchString(i.RequestingDepartment) || !opaque.MatchString(i.RequestedBy) {
		return ErrInvalid
	}
	if !in(i.Category, Categories) || !in(i.Audience, Audiences) || !in(i.LegalBasis, LegalBases) || !digest.MatchString(i.AudienceDefinitionSHA256) || !digest.MatchString(i.ContentSHA256) || !opaque.MatchString(i.TemplateKey) || !opaque.MatchString(i.TemplateLocale) || i.TemplateRevision < 1 {
		return ErrInvalid
	}
	if len(i.Markets) == 0 || len(i.Markets) > 16 || len(i.Zones) > 64 || len(i.Channels) == 0 || len(i.Channels) > 3 {
		return ErrInvalid
	}
	for _, m := range append(append([]string{}, i.Markets...), i.Zones...) {
		if !opaque.MatchString(m) {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, c := range i.Channels {
		if !in(c, Channels) || seen[c] {
			return ErrInvalid
		}
		seen[c] = true
	}
	if i.StartAt.IsZero() || !i.EndAt.After(i.StartAt) || i.EndAt.Before(now) || i.EndAt.Sub(i.StartAt) > 90*24*time.Hour {
		return ErrInvalid
	}
	if i.MaxPerRecipientPerDay < 1 || i.MaxPerRecipientPerDay > 10 || i.MaxMessages < 1 || i.MaxMessages > 10_000_000 || i.BudgetMinor < 0 || (i.BudgetMinor > 0) != (i.Currency != "") || (i.Currency != "" && !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(i.Currency)) || !in(i.Priority, []string{"low", "normal"}) {
		return ErrInvalid
	}
	if q := i.QuietHours; q != nil {
		if !clock.MatchString(q.Start) || !clock.MatchString(q.End) || q.Start == q.End {
			return ErrInvalid
		}
		if _, err := time.LoadLocation(q.Timezone); err != nil || q.Timezone == "" || q.Timezone == "Local" {
			return ErrInvalid
		}
	}
	if len(i.Approvals) == 0 || len(i.Approvals) > 4 {
		return ErrInvalid
	}
	approvers := map[string]bool{}
	for _, a := range i.Approvals {
		aid, err := uuid.Parse(a.ApprovalID)
		if err != nil || aid.String() != a.ApprovalID || !opaque.MatchString(a.ApproverRef) || a.ApproverRef == i.RequestedBy || approvers[a.ApproverRef] || a.ApprovedAt.IsZero() || a.ApprovedAt.After(now.Add(time.Minute)) {
			return ErrInvalid
		}
		approvers[a.ApproverRef] = true
	}
	if i.CorrelationID != "" && !opaque.MatchString(i.CorrelationID) {
		return ErrInvalid
	}
	return nil
}

// Digest is the canonical hash of the revision content (approvals excluded so
// an approval binds to it; idempotency key excluded so retries match).
func (i Intent) Digest() string {
	c := i
	c.Approvals = nil
	c.IdempotencyKey = ""
	c.CorrelationID = ""
	c.Markets = sortedCopy(c.Markets)
	c.Zones = sortedCopy(c.Zones)
	c.Channels = sortedCopy(c.Channels)
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortedCopy(v []string) []string {
	out := append([]string{}, v...)
	sort.Strings(out)
	return out
}

// Decode strictly parses an intent body.
func Decode(raw []byte) (Intent, error) {
	var i Intent
	if len(raw) == 0 || len(raw) > MaxBody {
		return i, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&i); err != nil {
		return i, ErrInvalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return i, ErrInvalid
	}
	return i, nil
}

// Status is the owner's truthful view of one revision.
type Status struct {
	Contract             string           `json:"contract_version"`
	CampaignID           string           `json:"campaign_id"`
	Revision             int64            `json:"revision"`
	IntentDigest         string           `json:"intent_digest"`
	State                string           `json:"state"`
	RejectionCode        string           `json:"rejection_code,omitempty"`
	AudienceCount        int64            `json:"audience_count"`
	EligibleCount        int64            `json:"eligible_count"`
	SuppressedCount      int64            `json:"suppressed_count"`
	SuppressionReasons   map[string]int64 `json:"suppression_reasons"`
	QueuedCount          int64            `json:"queued_count"`
	SentCount            int64            `json:"sent_count"`
	LoggedOnlyCount      int64            `json:"logged_only_count"`
	FailedCount          int64            `json:"failed_count"`
	DeliveredCount       *int64           `json:"delivered_count"`
	DeliveryConfirmation string           `json:"delivery_confirmation"`
	ConversionReporting  string           `json:"conversion_reporting"`
	AsOf                 time.Time        `json:"as_of"`
}

type Cancellation struct {
	ContractVersion string `json:"contract_version"`
	CampaignID      string `json:"campaign_id"`
	Revision        int64  `json:"revision"`
	CancellationKey string `json:"cancellation_key"`
	RequestedBy     string `json:"requested_by"`
	ReasonCode      string `json:"reason_code"`
}

func (c Cancellation) Validate() error {
	id, err := uuid.Parse(c.CampaignID)
	if c.ContractVersion != Contract || err != nil || id.String() != c.CampaignID || c.Revision < 1 || !opaque.MatchString(c.CancellationKey) || !opaque.MatchString(c.RequestedBy) || !regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`).MatchString(c.ReasonCode) {
		return ErrInvalid
	}
	return nil
}
