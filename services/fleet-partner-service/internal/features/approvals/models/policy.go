package models

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// Policy origins.
const (
	// OriginTemplate is a policy row created on first use from the platform
	// eligibility policy. It is not edited through the approval-policy API:
	// the eligibility policy publication route owns its values.
	OriginTemplate = "TEMPLATE"
	// OriginOrganization is a policy an organization authored itself.
	OriginOrganization = "ORGANIZATION"
)

// Policy statuses and scopes.
const (
	PolicyStatusActive   = "ACTIVE"
	PolicyStatusArchived = "ARCHIVED"

	ScopeOrganization = "ORGANIZATION"
	ScopeBranch       = "BRANCH"
)

// Limits on an organization-authored policy.
const (
	MaxPolicySteps        = 5
	MaxStepQuorum         = 20
	MinPolicyExpirySecond = 300
	MaxPolicyExpirySecond = 30 * 24 * 60 * 60
	DefaultPolicyExpiry   = 7 * 24 * 60 * 60
)

// StepSpec is one stage of an approval sequence: how many approvals it needs
// and which roles may give them. An empty role list inherits the platform
// policy's eligible roles.
type StepSpec struct {
	Quorum          int      `json:"quorum"`
	EligibleRoleIDs []string `json:"eligibleRoleIds"`
}

// Policy is one approval policy row.
type Policy struct {
	ID             string
	OrganizationID string
	Code           string
	Name           string
	Description    string
	ResourceType   string
	Action         string
	ScopeType      string
	ScopeID        *string

	// ThresholdAmountMinor and ThresholdCurrency make the policy apply only
	// to requests whose amount, in that currency, is at or above the
	// threshold. Both nil/empty means the policy applies regardless of amount.
	ThresholdAmountMinor *int64
	ThresholdCurrency    string

	Steps               []StepSpec
	RequesterMayApprove bool
	SeparationOfDuties  bool
	ExpirySeconds       int

	Origin         string
	Status         string
	Version        int64
	CreatedBy      string
	UpdatedBy      string
	IdempotencyKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
}

// PolicyVersion is one immutable published version of a policy.
type PolicyVersion struct {
	ID           string
	PolicyID     string
	Version      int64
	Status       string
	Rules        map[string]any
	ChangedBy    string
	ChangeReason string
	CreatedAt    time.Time
}

// RulesSnapshot is the canonical, versioned description of what a policy
// version requires. It is what the version history stores.
func (p Policy) RulesSnapshot() map[string]any {
	steps := make([]map[string]any, 0, len(p.Steps))
	for _, step := range p.Steps {
		roles := step.EligibleRoleIDs
		if roles == nil {
			roles = []string{}
		}
		steps = append(steps, map[string]any{"quorum": step.Quorum, "eligibleRoleIds": roles})
	}
	snapshot := map[string]any{
		"code": p.Code, "name": p.Name, "description": p.Description,
		"resourceType": p.ResourceType, "action": p.Action,
		"scopeType": p.ScopeType, "steps": steps,
		"requesterMayApprove": p.RequesterMayApprove,
		"separationOfDuties":  p.SeparationOfDuties,
		"expirySeconds":       p.ExpirySeconds,
		"status":              p.Status,
	}
	if p.ScopeID != nil {
		snapshot["scopeId"] = *p.ScopeID
	}
	if p.ThresholdAmountMinor != nil {
		snapshot["thresholdAmountMinor"] = *p.ThresholdAmountMinor
		snapshot["thresholdCurrency"] = p.ThresholdCurrency
	}
	return snapshot
}

// Amount is a money amount in integer minor units with an ISO currency.
type Amount struct {
	Minor    int64
	Currency string
}

// ErrAmountInvalid means the payload named an amount that is not a whole,
// non-negative number of minor units, or an unusable currency.
var ErrAmountInvalid = errors.New("approvals: amount must be whole minor units with an ISO currency")

// AmountFromPayload reads amountMinor and currency from a controlled payload.
//
// The amount is taken from the payload, not from a separate field, because
// the payload is what the fingerprint covers: a caller cannot state one
// amount to pick a lenient policy and execute another.
//
// It returns (nil, false, nil) when the payload names no amount,
// (nil, true, nil) when it names an amount but no currency, and an error when
// what it names is not a valid amount.
func AmountFromPayload(payload map[string]any) (*Amount, bool, error) {
	raw, ok := payload["amountMinor"]
	if !ok || raw == nil {
		return nil, false, nil
	}
	var minor int64
	switch value := raw.(type) {
	case int:
		minor = int64(value)
	case int32:
		minor = int64(value)
	case int64:
		minor = value
	case float64:
		if value != math.Trunc(value) || value > math.MaxInt64 || value < math.MinInt64 {
			return nil, true, ErrAmountInvalid
		}
		minor = int64(value)
	default:
		return nil, true, ErrAmountInvalid
	}
	if minor < 0 {
		return nil, true, ErrAmountInvalid
	}
	currency, _ := payload["currency"].(string)
	currency = strings.TrimSpace(currency)
	if currency == "" {
		return nil, true, nil
	}
	if !ValidCurrency(currency) {
		return nil, true, ErrAmountInvalid
	}
	return &Amount{Minor: minor, Currency: currency}, true, nil
}

// ValidCurrency reports whether value is a three-letter upper-case ISO code.
func ValidCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// AppliesTo reports whether an ACTIVE organization policy governs a request
// raised in branchID for amount.
func (p Policy) AppliesTo(branchID string, amount *Amount) bool {
	if p.Status != PolicyStatusActive || p.Origin != OriginOrganization {
		return false
	}
	if p.ScopeType == ScopeBranch && (p.ScopeID == nil || *p.ScopeID != branchID) {
		return false
	}
	if p.ThresholdAmountMinor == nil {
		return true
	}
	return amount != nil && amount.Currency == p.ThresholdCurrency &&
		amount.Minor >= *p.ThresholdAmountMinor
}

// HasThreshold reports whether the policy is amount-conditional.
func (p Policy) HasThreshold() bool { return p.ThresholdAmountMinor != nil }

// InScope reports whether the policy's scope covers a request raised in
// branchID, ignoring any threshold.
func (p Policy) InScope(branchID string) bool {
	if p.Status != PolicyStatusActive || p.Origin != OriginOrganization {
		return false
	}
	return p.ScopeType != ScopeBranch || (p.ScopeID != nil && *p.ScopeID == branchID)
}

// SelectPolicy picks the one organization policy that governs a request.
//
// The most specific policy wins: a branch policy over an organization-wide
// one, then the highest threshold the amount reaches over a lower one or an
// unconditional one. Remaining ties resolve by code so the choice is
// deterministic. Nil means no organization policy applies and the platform
// policy governs alone.
func SelectPolicy(policies []Policy, branchID string, amount *Amount) *Policy {
	candidates := make([]Policy, 0, len(policies))
	for _, policy := range policies {
		if policy.AppliesTo(branchID, amount) {
			candidates = append(candidates, policy)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if (a.ScopeType == ScopeBranch) != (b.ScopeType == ScopeBranch) {
			return a.ScopeType == ScopeBranch
		}
		at, bt := int64(-1), int64(-1)
		if a.ThresholdAmountMinor != nil {
			at = *a.ThresholdAmountMinor
		}
		if b.ThresholdAmountMinor != nil {
			bt = *b.ThresholdAmountMinor
		}
		if at != bt {
			return at > bt
		}
		return a.Code < b.Code
	})
	selected := candidates[0]
	return &selected
}

// PlatformRules is what the platform eligibility policy requires of an
// approval. An organization policy can make these stricter, never weaker.
type PlatformRules struct {
	MinimumApprovals    int
	EligibleRoleIDs     []string
	SelfApprovalAllowed bool
	SeparationOfDuties  bool
	TTLSeconds          int
}

// Rules is the effective set of rules frozen onto a request.
type Rules struct {
	Steps               []StepSpec
	SelfApprovalAllowed bool
	SeparationOfDuties  bool
	TTLSeconds          int
}

// TotalApprovals is the number of approvals the whole sequence needs.
func (r Rules) TotalApprovals() int {
	total := 0
	for _, step := range r.Steps {
		total += step.Quorum
	}
	return total
}

// EffectiveRules combines an organization policy with the platform floor.
//
// Organization owners cannot bypass platform safety rules (spec 9.1), so each
// rule takes the stricter of the two: the first step needs at least the
// platform quorum, self-approval is allowed only if both allow it, separation
// of duties holds if either demands it, and the approval window is the
// shorter of the two. Steps that name no roles inherit the platform roles.
func EffectiveRules(policy *Policy, platform PlatformRules) Rules {
	minimum := platform.MinimumApprovals
	if minimum < 1 {
		minimum = 1
	}
	if policy == nil {
		return Rules{
			Steps:               []StepSpec{{Quorum: minimum, EligibleRoleIDs: platform.EligibleRoleIDs}},
			SelfApprovalAllowed: platform.SelfApprovalAllowed,
			SeparationOfDuties:  platform.SeparationOfDuties,
			TTLSeconds:          platform.TTLSeconds,
		}
	}
	steps := make([]StepSpec, 0, len(policy.Steps))
	for _, step := range policy.Steps {
		roles := step.EligibleRoleIDs
		if len(roles) == 0 {
			roles = platform.EligibleRoleIDs
		}
		steps = append(steps, StepSpec{Quorum: step.Quorum, EligibleRoleIDs: roles})
	}
	if len(steps) == 0 {
		steps = []StepSpec{{Quorum: minimum, EligibleRoleIDs: platform.EligibleRoleIDs}}
	}
	if steps[0].Quorum < minimum {
		steps[0].Quorum = minimum
	}
	ttl := policy.ExpirySeconds
	if platform.TTLSeconds > 0 && (ttl <= 0 || platform.TTLSeconds < ttl) {
		ttl = platform.TTLSeconds
	}
	return Rules{
		Steps:               steps,
		SelfApprovalAllowed: policy.RequesterMayApprove && platform.SelfApprovalAllowed,
		SeparationOfDuties:  policy.SeparationOfDuties || platform.SeparationOfDuties,
		TTLSeconds:          ttl,
	}
}
