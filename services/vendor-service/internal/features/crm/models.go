package crm

import (
	"strings"
	"time"
)

// SubjectType is the kind of customer subject a relationship is with.
type SubjectType string

const (
	// SubjectCustomer is a canonical registered customer, referenced by an
	// opaque identifier owned by customer-service.
	SubjectCustomer SubjectType = "CUSTOMER"
	// SubjectGuest is a guest checkout subject produced by an authoritative
	// commerce flow. It may later be linked to a CUSTOMER by the identity
	// authority — never by a vendor staff assertion.
	SubjectGuest SubjectType = "GUEST"
	// SubjectExternal is a relationship imported by a trusted external or
	// partner authority. No ordinary vendor route can mint one.
	SubjectExternal SubjectType = "EXTERNAL"
)

// ProfileStatus is the vendor-facing lifecycle of the relationship. It is an
// operator convenience and carries no privacy meaning: archiving is not a
// lawful response to an erasure request and can never mask a redaction.
type ProfileStatus string

const (
	StatusActive   ProfileStatus = "ACTIVE"
	StatusArchived ProfileStatus = "ARCHIVED"
)

// PrivacyState is the authoritative privacy outcome projected onto the
// relationship.
type PrivacyState string

const (
	// PrivacyActive permits ordinary CRM processing.
	PrivacyActive PrivacyState = "ACTIVE"
	// PrivacyRestricted keeps the relationship present but limits processing.
	// It is not an implicit redaction.
	PrivacyRestricted PrivacyState = "RESTRICTED"
	// PrivacyRedacted blocks ordinary use entirely: not searchable, not
	// targetable, not mutable, and not recreatable by a late event.
	PrivacyRedacted PrivacyState = "REDACTED"
	// PrivacyMerged means the profile is no longer an independent target and
	// points at a surviving profile.
	PrivacyMerged PrivacyState = "MERGED"
)

// MergeState tracks a profile's position in an identity merge.
type MergeState string

const (
	MergeNone        MergeState = "NONE"
	MergeLinkPending MergeState = "LINK_PENDING"
	MergeSource      MergeState = "MERGED_SOURCE"
	MergeTarget      MergeState = "MERGED_TARGET"
)

// ProjectionState describes how much a projection can be trusted. The
// distinction between NOT_AVAILABLE and a real zero is load-bearing: a
// relationship with no observed commercial data must never be rendered as a
// customer who spent nothing.
type ProjectionState string

const (
	ProjectionNotAvailable ProjectionState = "NOT_AVAILABLE"
	ProjectionCurrent      ProjectionState = "CURRENT"
	ProjectionStale        ProjectionState = "STALE"
	ProjectionRebuilding   ProjectionState = "REBUILDING"
	ProjectionError        ProjectionState = "ERROR"
)

// ConsentDecision is the projected decision from the consent authority.
// UNKNOWN is a first-class value, distinct from both ALLOW and DENY.
type ConsentDecision string

const (
	ConsentAllow   ConsentDecision = "ALLOW"
	ConsentDeny    ConsentDecision = "DENY"
	ConsentUnknown ConsentDecision = "UNKNOWN"
)

// ConsentPurpose is the authoritative processing purpose. Section 8A needs
// marketing eligibility only; a purpose is never widened beyond the scope the
// authority actually granted.
type ConsentPurpose string

const ConsentPurposeMarketing ConsentPurpose = "MARKETING"

// ConsentChannel is the delivery channel a decision is scoped to. Consent and
// delivery capability are separate concerns: the platform has no SMS sender
// today, yet an SMS consent decision remains representable and must be
// honoured if a sender ever appears.
type ConsentChannel string

const (
	ChannelEmail ConsentChannel = "EMAIL"
	ChannelPush  ConsentChannel = "PUSH"
	ChannelInApp ConsentChannel = "IN_APP"
	ChannelSMS   ConsentChannel = "SMS"
)

// MarketingChannels is the fixed set Customer 360 reports eligibility for.
var MarketingChannels = []ConsentChannel{ChannelEmail, ChannelPush, ChannelInApp, ChannelSMS}

// PrivacyAction is an instruction from the privacy authority.
type PrivacyAction string

const (
	PrivacyActionRestrict          PrivacyAction = "RESTRICT"
	PrivacyActionRedact            PrivacyAction = "REDACT"
	PrivacyActionMarketingSuppress PrivacyAction = "MARKETING_SUPPRESS"
	PrivacyActionIdentityMerge     PrivacyAction = "IDENTITY_MERGE"
)

// PrivacyProcessingStatus is the lifecycle of one privacy instruction.
type PrivacyProcessingStatus string

const (
	PrivacyPending    PrivacyProcessingStatus = "PENDING"
	PrivacyProcessing PrivacyProcessingStatus = "PROCESSING"
	PrivacyApplied    PrivacyProcessingStatus = "APPLIED"
	PrivacyFailed     PrivacyProcessingStatus = "FAILED"
	PrivacyDeadLetter PrivacyProcessingStatus = "DEAD_LETTER"
)

// LinkType is the kind of authoritative identity link.
type LinkType string

const (
	LinkGuestToCustomer    LinkType = "GUEST_TO_CUSTOMER"
	LinkCustomerMerge      LinkType = "CUSTOMER_MERGE"
	LinkExternalToCustomer LinkType = "EXTERNAL_TO_CUSTOMER"
)

// OrderFactState is the authoritative order state a CRM fact reflects.
//
// Counting rules, stated once and applied everywhere:
//
//   - COMPLETED contributes to completed_order_count, gross and net. A partial
//     refund arrives as a COMPLETED fact whose refunded amount has grown, so
//     net falls without the order being counted twice.
//   - REFUNDED means the order was reversed in full. The fact is retained as
//     evidence but contributes to no aggregate.
//   - CANCELLED contributes to no aggregate.
//   - UPDATED is a non-terminal observation and contributes to no aggregate.
type OrderFactState string

const (
	OrderCompleted OrderFactState = "COMPLETED"
	OrderCancelled OrderFactState = "CANCELLED"
	OrderRefunded  OrderFactState = "REFUNDED"
	OrderUpdated   OrderFactState = "UPDATED"
)

// CountsTowardCommercialTotals reports whether a fact in this state feeds the
// commercial aggregates.
func (s OrderFactState) CountsTowardCommercialTotals() bool { return s == OrderCompleted }

// Profile is the vendor-local customer relationship.
type Profile struct {
	ID         string
	BusinessID string

	SubjectType SubjectType
	CustomerRef *string
	GuestRef    *string
	ExternalRef *string

	SafeDisplayLabel *string

	Status       ProfileStatus
	PrivacyState PrivacyState
	MergeState   MergeState

	AcquisitionSourceType *string
	AcquisitionSourceRef  *string

	FirstCompletedOrderAt  *time.Time
	LastCompletedOrderAt   *time.Time
	CompletedOrderCount    int
	GrossSpendMinor        int64
	NetSpendMinor          int64
	AverageOrderValueMinor int64
	CommercialCurrency     *string

	LastOrderRef       *string
	LastBranchRef      *string
	PreferredBranchRef *string
	LastCapabilityType *string

	CommercialProjectionState      ProjectionState
	CommercialProjectionVersion    *int64
	LastCommercialSourceObservedAt *time.Time

	ConsentProjectionVersion *int64
	ConsentObservedAt        *time.Time

	RedactedAt          *time.Time
	MergedIntoProfileID *string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

// PrimarySubjectRef returns the reference that identifies this subject for its
// type, which is the only reference an authority event may resolve it by.
func (p Profile) PrimarySubjectRef() string {
	switch p.SubjectType {
	case SubjectCustomer:
		return deref(p.CustomerRef)
	case SubjectGuest:
		return deref(p.GuestRef)
	case SubjectExternal:
		return deref(p.ExternalRef)
	}
	return ""
}

// IsRedacted reports whether ordinary CRM use is blocked.
func (p Profile) IsRedacted() bool { return p.PrivacyState == PrivacyRedacted }

// IsMergedSource reports whether this profile has been superseded by a merge
// and may no longer receive independent mutations.
func (p Profile) IsMergedSource() bool { return p.MergeState == MergeSource }

// AcceptsOrdinaryMutation reports whether a normal CRM command (archive,
// restore) may act on this profile. Redacted and merged-away profiles are
// structurally immutable through ordinary routes — that is what stops an
// archive/restore cycle from being used to launder a redaction.
func (p Profile) AcceptsOrdinaryMutation() bool {
	return !p.IsRedacted() && !p.IsMergedSource()
}

// ValidateSubjectReferences enforces the "exactly one primary reference for
// this subject type" invariant in Go, mirroring the SQL CHECK constraint. A
// GUEST may additionally carry a customer reference once an authoritative link
// has been applied; nothing else may carry a foreign reference.
func ValidateSubjectReferences(subjectType SubjectType, customerRef, guestRef, externalRef *string) bool {
	has := func(v *string) bool { return v != nil && strings.TrimSpace(*v) != "" }
	switch subjectType {
	case SubjectCustomer:
		return has(customerRef) && !has(guestRef) && !has(externalRef)
	case SubjectGuest:
		return has(guestRef) && !has(externalRef)
	case SubjectExternal:
		return has(externalRef) && !has(guestRef)
	}
	return false
}

// IsValidSubjectType reports whether a wire value is a supported subject type.
func IsValidSubjectType(v string) bool {
	switch SubjectType(v) {
	case SubjectCustomer, SubjectGuest, SubjectExternal:
		return true
	}
	return false
}

// IsValidConsentChannel reports whether a wire value is a supported channel.
func IsValidConsentChannel(v string) bool {
	switch ConsentChannel(v) {
	case ChannelEmail, ChannelPush, ChannelInApp, ChannelSMS:
		return true
	}
	return false
}

// IsValidPrivacyAction reports whether a wire value is a supported action.
func IsValidPrivacyAction(v string) bool {
	switch PrivacyAction(v) {
	case PrivacyActionRestrict, PrivacyActionRedact, PrivacyActionMarketingSuppress, PrivacyActionIdentityMerge:
		return true
	}
	return false
}

// IsValidOrderFactState reports whether a wire value is a supported state.
func IsValidOrderFactState(v string) bool {
	switch OrderFactState(v) {
	case OrderCompleted, OrderCancelled, OrderRefunded, OrderUpdated:
		return true
	}
	return false
}

// Marketing eligibility reason codes. These are returned to vendor staff so
// they understand why a customer cannot be targeted, and are deliberately
// coarse: they never disclose the authority's internal reasoning.
const (
	ReasonConsentAllowed      = "CONSENT_ALLOWED"
	ReasonConsentDenied       = "CONSENT_DENIED"
	ReasonConsentUnknown      = "CONSENT_UNKNOWN"
	ReasonConsentStale        = "CONSENT_STALE"
	ReasonConsentUnavailable  = "CONSENT_SOURCE_UNAVAILABLE"
	ReasonPrivacyRedacted     = "PRIVACY_REDACTED"
	ReasonPrivacyRestricted   = "PRIVACY_RESTRICTED"
	ReasonMarketingSuppressed = "PRIVACY_MARKETING_SUPPRESSED"
	ReasonProfileMerged       = "PROFILE_MERGED"
	ReasonProfileArchived     = "PROFILE_ARCHIVED"
)

// MarketingEligibilityInput is everything the eligibility rule needs. It is a
// plain value so the rule can be unit-tested exhaustively without a database.
type MarketingEligibilityInput struct {
	PrivacyState        PrivacyState
	MergeState          MergeState
	Status              ProfileStatus
	MarketingSuppressed bool
	Decision            ConsentDecision
	// ConsentStale is set when the projection is older than policy allows, or
	// when the consent integration is disabled so no projection can be
	// trusted. Either way the answer is "not eligible".
	ConsentStale bool
	// ConsentIntegrationAvailable is false when no consent authority is wired.
	ConsentIntegrationAvailable bool
}

// MarketingEligibility is the single place marketing targetability is decided.
//
// Precedence, highest first: redaction, merge, marketing suppression,
// restriction, archival, explicit denial, unknown/stale/unavailable consent,
// and only then an explicit allow. Every branch except the last returns false,
// which is what makes "fail closed" a property of the type rather than a
// convention callers must remember.
func MarketingEligibility(in MarketingEligibilityInput) (eligible bool, reason string) {
	switch {
	case in.PrivacyState == PrivacyRedacted:
		return false, ReasonPrivacyRedacted
	case in.PrivacyState == PrivacyMerged || in.MergeState == MergeSource:
		return false, ReasonProfileMerged
	case in.MarketingSuppressed:
		return false, ReasonMarketingSuppressed
	case in.PrivacyState == PrivacyRestricted:
		return false, ReasonPrivacyRestricted
	case in.Status == StatusArchived:
		return false, ReasonProfileArchived
	case !in.ConsentIntegrationAvailable:
		return false, ReasonConsentUnavailable
	case in.Decision == ConsentDeny:
		return false, ReasonConsentDenied
	case in.Decision == ConsentUnknown || in.Decision == "":
		return false, ReasonConsentUnknown
	case in.ConsentStale:
		return false, ReasonConsentStale
	case in.Decision == ConsentAllow:
		return true, ReasonConsentAllowed
	}
	// Unreachable for the closed decision set above; defaulting to ineligible
	// keeps an unexpected future value from silently becoming targetable.
	return false, ReasonConsentUnknown
}

// CommercialTotals is the aggregate derived from a set of order facts.
type CommercialTotals struct {
	CompletedOrderCount    int
	GrossSpendMinor        int64
	NetSpendMinor          int64
	AverageOrderValueMinor int64
	FirstCompletedOrderAt  *time.Time
	LastCompletedOrderAt   *time.Time
	LastOrderRef           *string
	LastBranchRef          *string
	LastCapabilityType     *string
	Currency               string
}

// OrderFact is one authoritative order reference projected into CRM.
type OrderFact struct {
	ID             string
	BusinessID     string
	ProfileID      string
	SourceService  string
	SourceOrderRef string
	SourceEventID  string
	SourceVersion  int64
	OrderState     OrderFactState
	CompletedAt    *time.Time
	Currency       string
	GrossMinor     int64
	NetMinor       int64
	RefundedMinor  int64
	BranchID       *string
	CapabilityType *string
	LastObservedAt time.Time
}

// ComputeCommercialTotals derives the profile-level aggregate from facts.
//
// Deriving totals from facts on every change, rather than incrementing running
// counters, is what makes refunds, replays, out-of-order delivery and merges
// all safe: the same fact set always produces the same answer, so a redelivered
// event cannot inflate a total and a merge cannot double-count an order both
// profiles saw.
func ComputeCommercialTotals(facts []OrderFact) CommercialTotals {
	var totals CommercialTotals
	for _, fact := range facts {
		if !fact.OrderState.CountsTowardCommercialTotals() {
			continue
		}
		totals.CompletedOrderCount++
		totals.GrossSpendMinor += fact.GrossMinor
		totals.NetSpendMinor += fact.NetMinor
		if totals.Currency == "" {
			totals.Currency = fact.Currency
		}
		if fact.CompletedAt == nil {
			continue
		}
		if totals.FirstCompletedOrderAt == nil || fact.CompletedAt.Before(*totals.FirstCompletedOrderAt) {
			at := *fact.CompletedAt
			totals.FirstCompletedOrderAt = &at
		}
		if totals.LastCompletedOrderAt == nil || fact.CompletedAt.After(*totals.LastCompletedOrderAt) {
			at := *fact.CompletedAt
			totals.LastCompletedOrderAt = &at
			ref := fact.SourceOrderRef
			totals.LastOrderRef = &ref
			totals.LastBranchRef = fact.BranchID
			totals.LastCapabilityType = fact.CapabilityType
		}
	}
	totals.AverageOrderValueMinor = AverageOrderValue(totals.NetSpendMinor, totals.CompletedOrderCount)
	return totals
}

// AverageOrderValue divides net completed spend by eligible completed order
// count using integer arithmetic — money never touches a float. The result
// truncates toward zero; with no eligible orders it is zero, which callers
// must present alongside a NOT_AVAILABLE projection state rather than as a
// real average.
func AverageOrderValue(netSpendMinor int64, completedOrderCount int) int64 {
	if completedOrderCount <= 0 {
		return 0
	}
	return netSpendMinor / int64(completedOrderCount)
}

// PreferredBranch picks the branch a customer relates to most strongly:
// highest completed-order count, then most recent relationship, then the
// lowest branch id as a stable tie-break. It is a deterministic rule, not a
// prediction — CRM does not infer preference from anything but observed
// completed orders.
func PreferredBranch(relations []BranchRelation) *string {
	var best *BranchRelation
	for i := range relations {
		candidate := &relations[i]
		if candidate.CompletedOrderCount == 0 {
			continue
		}
		if best == nil || betterBranch(candidate, best) {
			best = candidate
		}
	}
	if best == nil {
		return nil
	}
	id := best.BranchID
	return &id
}

func betterBranch(candidate, best *BranchRelation) bool {
	if candidate.CompletedOrderCount != best.CompletedOrderCount {
		return candidate.CompletedOrderCount > best.CompletedOrderCount
	}
	switch {
	case candidate.LastRelationshipAt != nil && best.LastRelationshipAt == nil:
		return true
	case candidate.LastRelationshipAt == nil && best.LastRelationshipAt != nil:
		return false
	case candidate.LastRelationshipAt != nil && best.LastRelationshipAt != nil &&
		!candidate.LastRelationshipAt.Equal(*best.LastRelationshipAt):
		return candidate.LastRelationshipAt.After(*best.LastRelationshipAt)
	}
	return candidate.BranchID < best.BranchID
}

// BranchRelation is the branch-scoped slice of a relationship.
type BranchRelation struct {
	ID                  string
	BusinessID          string
	ProfileID           string
	BranchID            string
	FirstRelationshipAt *time.Time
	LastRelationshipAt  *time.Time
	CompletedOrderCount int
	GrossSpendMinor     int64
	NetSpendMinor       int64
	Currency            *string
	LastOrderRef        *string
	LastCapabilityType  *string
	ProjectionState     ProjectionState
	SourceVersion       *int64
	Version             int64
}

// ConsentProjection is a local copy of one authoritative consent decision.
type ConsentProjection struct {
	ID            string
	BusinessID    string
	ProfileID     string
	Purpose       ConsentPurpose
	Channel       ConsentChannel
	Decision      ConsentDecision
	SourceService string
	SourceRef     *string
	SourceVersion int64
	ObservedAt    time.Time
	EffectiveAt   *time.Time
	ExpiresAt     *time.Time
	Version       int64
}

// Expired reports whether a time-boxed consent decision has lapsed. An expired
// ALLOW is not an ALLOW.
func (c ConsentProjection) Expired(now time.Time) bool {
	return c.ExpiresAt != nil && !now.Before(*c.ExpiresAt)
}

// EffectiveDecision collapses expiry into the decision itself, so callers
// cannot forget to check it.
func (c ConsentProjection) EffectiveDecision(now time.Time) ConsentDecision {
	if c.Expired(now) {
		return ConsentUnknown
	}
	return c.Decision
}

// PrivacyProjection is one authoritative privacy instruction.
type PrivacyProjection struct {
	ID                string
	BusinessID        string
	ProfileID         *string
	PrivacySubjectRef string
	Action            PrivacyAction
	SourceService     string
	SourceRef         *string
	SourceVersion     int64
	ObservedAt        time.Time
	ProcessedAt       *time.Time
	ProcessingStatus  PrivacyProcessingStatus
	RetentionPolicy   *string
	Attempts          int
	SafeReasonCode    *string
	Version           int64
}

// PrivacyStateFor maps an action to the profile privacy state it produces.
// MARKETING_SUPPRESS deliberately leaves the state alone: suppression removes
// targetability without pretending the relationship was restricted or erased.
func PrivacyStateFor(action PrivacyAction, current PrivacyState) PrivacyState {
	switch action {
	case PrivacyActionRedact:
		return PrivacyRedacted
	case PrivacyActionRestrict:
		// Never downgrade a redaction into a restriction.
		if current == PrivacyRedacted || current == PrivacyMerged {
			return current
		}
		return PrivacyRestricted
	}
	return current
}

// IdentityLink is one authoritative alias between two profiles.
type IdentityLink struct {
	ID                 string
	BusinessID         string
	SourceProfileID    string
	TargetProfileID    string
	LinkType           LinkType
	AuthorityService   string
	AuthorityReference string
	AuthorityVersion   int64
	EvidenceEventID    string
	Status             string
	CreatedAt          time.Time
	AppliedAt          *time.Time
}

// LinkTypeFor derives the link kind from the two subject types involved.
// An unsupported combination returns false rather than being coerced into the
// nearest type — an authority that sends something we do not model should be
// rejected loudly, not guessed at.
func LinkTypeFor(source, target SubjectType) (LinkType, bool) {
	switch {
	case source == SubjectGuest && target == SubjectCustomer:
		return LinkGuestToCustomer, true
	case source == SubjectCustomer && target == SubjectCustomer:
		return LinkCustomerMerge, true
	case source == SubjectExternal && target == SubjectCustomer:
		return LinkExternalToCustomer, true
	}
	return "", false
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ptr[T any](v T) *T { return &v }
