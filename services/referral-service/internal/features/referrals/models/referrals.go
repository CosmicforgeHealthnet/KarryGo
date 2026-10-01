package referralmodels

import (
	"time"

	"cosmicforge/logistics/shared/go/referralcontract"
)

// Person statuses.
const (
	PersonActive  = "active"
	PersonFrozen  = "frozen"
	PersonBlocked = "blocked"
)

// Person is one human in the programme.
type Person struct {
	ID            string
	Status        string
	StatusReason  string
	HomeActorType string
	HomeActorID   string
	// PowerUserSince is when the person reached the 10-referral milestone.
	PowerUserSince *time.Time
	CreatedAt      time.Time
}

// Actor is one of a person's accounts.
type Actor struct {
	ActorType          string
	ActorID            string
	PersonID           string
	RegisteredAt       time.Time
	IsNewAccount       bool
	AttributionOutcome string
	LocationState      string
}

// Attribution outcomes recorded on the account for support staff.
const (
	AttributionAttributed      = "attributed"       // a new referral was created
	AttributionAlreadyReferred = "already_referred" // the person was referred before; this account joins that referral
	AttributionNoCode          = "no_code"
	AttributionInvalidCode     = "invalid_code"
	AttributionSelfReferral    = "self_referral"
	AttributionExistingPerson  = "existing_person" // not new to the platform, so not referable
	AttributionReferrerBlocked = "referrer_blocked"
	AttributionBackfill        = "backfill" // announced by the one-time backfill; never referable
)

// Referral relationship statuses.
const (
	ReferralActive       = "active"
	ReferralUnderReview  = "under_review"
	ReferralSuspended    = "suspended"
	ReferralDisqualified = "disqualified"
)

// Referral is one "A referred B" relationship.
type Referral struct {
	ID               string
	ReferrerPersonID string
	ReferredPersonID string
	Code             string
	CodeSource       string
	AttributedAt     time.Time
	Status           string
	StatusReason     string
}

// Track models: which reward programme a track runs.
const (
	ModelCustomer = "customer"
	ModelProvider = "provider"
)

// Track statuses.
const (
	TrackPendingQualification = "pending_qualification"
	TrackQualified            = "qualified"
	TrackExpired              = "expired"
)

// Track is one referral's progress under one model.
type Track struct {
	ReferralID                string
	Model                     string
	Status                    string
	JobsRequired              int
	JobsCounted               int
	OnboardingCompletedAt     *time.Time
	QualifiedAt               *time.Time
	QualifyingSourceService   string
	QualifyingSourceReference string
	WindowStartsAt            *time.Time
	WindowEndsAt              *time.Time
}

// ModelForActor is the reward model an account type runs: the reward follows
// the kind of account the referred person opened.
func ModelForActor(actorType string) string {
	if referralcontract.IsProviderActor(actorType) {
		return ModelProvider
	}
	return ModelCustomer
}

// Fraud flag types and severities.
const (
	FlagSelfReferral      = "self_referral"
	FlagDuplicateIdentity = "duplicate_identity"
	FlagIdentityConflict  = "identity_conflict"
	FlagDuplicateDevice   = "duplicate_device"
	FlagDuplicateBank     = "duplicate_bank"
	FlagRepeatedReversals = "repeated_reversals"
	// FlagCollusiveJob: a job between a referred person and their own
	// referrer, or with themselves on both sides. It earns nothing.
	FlagCollusiveJob = "collusive_job"
	FlagManual       = "manual"

	SeverityInfo   = "info"
	SeverityReview = "review"
)

// Flag review outcomes.
const (
	FlagOpen    = "open"
	FlagCleared = "cleared"
	FlagUpheld  = "upheld"
)

// Flag is something raised for a human to review.
type Flag struct {
	ID             string         `json:"id"`
	PersonID       string         `json:"person_id,omitempty"`
	ReferralID     string         `json:"referral_id,omitempty"`
	FlagType       string         `json:"flag_type"`
	Severity       string         `json:"severity"`
	Evidence       map[string]any `json:"evidence"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy     string         `json:"resolved_by,omitempty"`
	ResolutionNote string         `json:"resolution_note,omitempty"`
}

// Milestones: the referrer's 5th and 10th qualified customer referrals.
const (
	Milestone5  = "qualified_5"
	Milestone10 = "qualified_10"
)

// MilestoneRule is one milestone and what it pays.
type MilestoneRule struct {
	Key        string
	Referrals  int
	RewardType string
	Balance    string
	AmountKobo int64
	// Badge is whether reaching it earns the "Power User" badge.
	Badge bool
}

// Milestones lists the customer-referral milestones in the order they are
// reached. The 5-referral voucher is spend-only credit; the 10-referral bonus
// is referral earnings.
func (r Rules) Milestones() []MilestoneRule {
	return []MilestoneRule{
		{Key: Milestone5, Referrals: 5, RewardType: RewardMilestoneFreeDelivery, Balance: BalancePromotional, AmountKobo: r.Milestone5CreditKobo},
		{Key: Milestone10, Referrals: 10, RewardType: RewardMilestoneBonus, Balance: BalanceEarnings, AmountKobo: r.Milestone10BonusKobo, Badge: true},
	}
}
