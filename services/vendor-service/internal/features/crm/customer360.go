package crm

import (
	"context"
	"time"
)

// ModuleAvailability tells a client that a CRM module exists as a concept but
// is not yet operational, so an empty list is never mistaken for "this
// customer has none".
const (
	ModuleNotImplemented = "NOT_IMPLEMENTED"
	ModuleAvailable      = "AVAILABLE"
)

// ProfileView is the privacy-safe projection of a relationship.
//
// It carries no canonical customer contact data of any kind. There is no email,
// phone, address or date-of-birth field to populate, because CRM never stores
// one — a client that needs display identity must ask the customer authority.
type ProfileView struct {
	ProfileRef      string        `json:"profile_ref"`
	SubjectType     SubjectType   `json:"subject_type"`
	DisplayLabel    *string       `json:"display_label"`
	Status          ProfileStatus `json:"status"`
	PrivacyState    PrivacyState  `json:"privacy_state"`
	MergeState      MergeState    `json:"merge_state"`
	AcquisitionType *string       `json:"acquisition_source_type,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	Version         int64         `json:"version"`
}

// CommercialSummary is the commercial block, always led by its state so a
// caller must confront "do we actually know this" before reading a number.
type CommercialSummary struct {
	State ProjectionState `json:"state"`

	Currency               *string    `json:"currency,omitempty"`
	CompletedOrderCount    *int       `json:"completed_order_count,omitempty"`
	GrossSpendMinor        *int64     `json:"gross_spend_minor,omitempty"`
	NetSpendMinor          *int64     `json:"net_spend_minor,omitempty"`
	AverageOrderValueMinor *int64     `json:"average_order_value_minor,omitempty"`
	FirstCompletedOrderAt  *time.Time `json:"first_completed_order_at,omitempty"`
	LastCompletedOrderAt   *time.Time `json:"last_completed_order_at,omitempty"`
	LastOrderRef           *string    `json:"last_order_ref,omitempty"`

	// RecencyDays and OrderFrequencyPerYear are derived only from observed
	// completed orders. With no commercial data they stay nil rather than
	// being inferred from when the profile row happened to be created.
	RecencyDays           *int     `json:"recency_days,omitempty"`
	OrderFrequencyPerYear *float64 `json:"order_frequency_per_year,omitempty"`

	ObservedAt        *time.Time `json:"observed_at,omitempty"`
	ProjectionVersion *int64     `json:"projection_version,omitempty"`
	Stale             bool       `json:"stale"`

	// Scope states whether these numbers cover the whole business or only the
	// branches the caller may see, so a branch manager can never mistake their
	// slice for the business total.
	Scope string `json:"scope"`
}

// BranchSummary is one branch's slice of the relationship.
type BranchSummary struct {
	BranchRef           string          `json:"branch_ref"`
	CompletedOrderCount int             `json:"completed_order_count"`
	GrossSpendMinor     int64           `json:"gross_spend_minor"`
	NetSpendMinor       int64           `json:"net_spend_minor"`
	Currency            *string         `json:"currency,omitempty"`
	FirstRelationshipAt *time.Time      `json:"first_relationship_at,omitempty"`
	LastRelationshipAt  *time.Time      `json:"last_relationship_at,omitempty"`
	LastCapabilityType  *string         `json:"last_capability_type,omitempty"`
	ProjectionState     ProjectionState `json:"projection_state"`
}

// CapabilitySummary reports which capability the relationship last transacted
// through, and whether category/item affinity is available at all.
type CapabilitySummary struct {
	LastCapabilityType *string `json:"last_capability_type,omitempty"`
	// CategoryAffinity and ItemAffinity require an authoritative catalogue,
	// which does not exist yet. They report unavailability rather than an
	// empty list that would read as "no preferences".
	CategoryAffinity string `json:"category_affinity"`
	ItemAffinity     string `json:"item_affinity"`
}

// Customer360 is the composed relationship view.
type Customer360 struct {
	Profile      ProfileView         `json:"profile"`
	Commercial   CommercialSummary   `json:"commercial"`
	Branches     []BranchSummary     `json:"branches"`
	Capability   CapabilitySummary   `json:"capability"`
	Consent      ConsentSummary      `json:"consent"`
	Privacy      PrivacySummary      `json:"privacy"`
	IdentityLink IdentityLinkSummary `json:"identity_link"`
	// Modules states, per later CRM package, whether it is operational. This
	// is what keeps Customer 360 honest while 8B-8H remain unbuilt.
	Modules map[string]string `json:"modules"`
}

// ViewOf renders the safe list projection of a profile.
func ViewOf(profile Profile) ProfileView {
	return ProfileView{
		ProfileRef:      profile.ID,
		SubjectType:     profile.SubjectType,
		DisplayLabel:    profile.SafeDisplayLabel,
		Status:          profile.Status,
		PrivacyState:    profile.PrivacyState,
		MergeState:      profile.MergeState,
		AcquisitionType: profile.AcquisitionSourceType,
		CreatedAt:       profile.CreatedAt,
		Version:         profile.Version,
	}
}

// CommercialStaleAfter bounds how long a commercial projection is considered
// current once observed.
const CommercialStaleAfter = 7 * 24 * time.Hour

// Customer360For composes the relationship view for one profile.
//
// Composition is deliberately modular — each block comes from its own reader —
// so later packages can contribute their own without this function growing a
// dependency on every future CRM repository.
func (s *Service) Customer360For(ctx context.Context, actor Actor, scope Scope, profile Profile) (Customer360, error) {
	relations, err := s.branchRelations(ctx, s.db, actor.VendorID, profile.ID)
	if err != nil {
		return Customer360{}, err
	}
	// A branch-scoped caller sees only their own branches, and their totals
	// are recomputed from just those branches — never the business-wide
	// figure, which would leak other branches' trade.
	visible := relations
	if !scope.Global {
		visible = visible[:0:0]
		for _, relation := range relations {
			if containsString(scope.BranchIDs, relation.BranchID) {
				visible = append(visible, relation)
			}
		}
	}

	consent, err := s.ConsentSummaryFor(ctx, actor, profile)
	if err != nil {
		return Customer360{}, err
	}
	privacy, err := s.PrivacySummaryFor(ctx, actor, profile)
	if err != nil {
		return Customer360{}, err
	}
	links, err := s.IdentityLinkSummaryFor(ctx, actor, profile)
	if err != nil {
		return Customer360{}, err
	}

	view := Customer360{
		Profile:      ViewOf(profile),
		Commercial:   s.commercialSummary(profile, visible, scope),
		Branches:     branchSummaries(visible),
		Capability:   s.capabilitySummary(profile, scope, visible),
		Consent:      consent,
		Privacy:      privacy,
		IdentityLink: links,
		Modules: map[string]string{
			"tags":         ModuleNotImplemented,
			"segments":     ModuleNotImplemented,
			"interactions": ModuleNotImplemented,
			"tasks":        ModuleNotImplemented,
			"loyalty":      ModuleNotImplemented,
			"campaigns":    ModuleNotImplemented,
			"corporate":    ModuleNotImplemented,
			"intelligence": ModuleNotImplemented,
		},
	}
	return view, nil
}

func (s *Service) commercialSummary(profile Profile, visible []BranchRelation, scope Scope) CommercialSummary {
	summary := CommercialSummary{State: profile.CommercialProjectionState, Scope: "BUSINESS"}
	if !scope.Global {
		summary.Scope = "BRANCH"
	}
	if profile.CommercialProjectionState == ProjectionNotAvailable {
		// No authoritative commercial data has ever been observed. Every
		// figure stays absent: a nil count is "we do not know", a zero would
		// be a claim we cannot support.
		return summary
	}

	if scope.Global {
		summary.Currency = profile.CommercialCurrency
		summary.CompletedOrderCount = ptr(profile.CompletedOrderCount)
		summary.GrossSpendMinor = ptr(profile.GrossSpendMinor)
		summary.NetSpendMinor = ptr(profile.NetSpendMinor)
		summary.AverageOrderValueMinor = ptr(profile.AverageOrderValueMinor)
		summary.FirstCompletedOrderAt = profile.FirstCompletedOrderAt
		summary.LastCompletedOrderAt = profile.LastCompletedOrderAt
		summary.LastOrderRef = profile.LastOrderRef
	} else {
		var count int
		var gross, net int64
		var first, last *time.Time
		var currency *string
		for _, relation := range visible {
			count += relation.CompletedOrderCount
			gross += relation.GrossSpendMinor
			net += relation.NetSpendMinor
			if currency == nil {
				currency = relation.Currency
			}
			if relation.FirstRelationshipAt != nil && (first == nil || relation.FirstRelationshipAt.Before(*first)) {
				first = relation.FirstRelationshipAt
			}
			if relation.LastRelationshipAt != nil && (last == nil || relation.LastRelationshipAt.After(*last)) {
				last = relation.LastRelationshipAt
			}
		}
		summary.Currency = currency
		summary.CompletedOrderCount = ptr(count)
		summary.GrossSpendMinor = ptr(gross)
		summary.NetSpendMinor = ptr(net)
		summary.AverageOrderValueMinor = ptr(AverageOrderValue(net, count))
		summary.FirstCompletedOrderAt = first
		summary.LastCompletedOrderAt = last
		// last_order_ref is intentionally omitted for a branch-scoped caller:
		// the most recent order overall may belong to a branch they cannot see.
	}

	summary.ObservedAt = profile.LastCommercialSourceObservedAt
	summary.ProjectionVersion = profile.CommercialProjectionVersion
	if profile.LastCommercialSourceObservedAt != nil {
		summary.Stale = s.now().Sub(*profile.LastCommercialSourceObservedAt) > CommercialStaleAfter
	}
	summary.RecencyDays, summary.OrderFrequencyPerYear = recencyAndFrequency(
		summary.FirstCompletedOrderAt, summary.LastCompletedOrderAt, summary.CompletedOrderCount, s.now())
	return summary
}

// recencyAndFrequency derives simple RFM-style figures from observed orders
// only. Both stay nil when there is nothing to derive them from.
func recencyAndFrequency(first, last *time.Time, count *int, now time.Time) (*int, *float64) {
	if last == nil || count == nil || *count == 0 {
		return nil, nil
	}
	days := int(now.Sub(*last).Hours() / 24)
	if days < 0 {
		days = 0
	}
	if first == nil {
		return &days, nil
	}
	span := last.Sub(*first).Hours() / 24
	if span < 1 {
		// A single day of trading cannot support an annual rate.
		return &days, nil
	}
	frequency := float64(*count) / (span / 365.0)
	return &days, &frequency
}

func branchSummaries(relations []BranchRelation) []BranchSummary {
	out := make([]BranchSummary, 0, len(relations))
	for _, relation := range relations {
		out = append(out, BranchSummary{
			BranchRef:           relation.BranchID,
			CompletedOrderCount: relation.CompletedOrderCount,
			GrossSpendMinor:     relation.GrossSpendMinor,
			NetSpendMinor:       relation.NetSpendMinor,
			Currency:            relation.Currency,
			FirstRelationshipAt: relation.FirstRelationshipAt,
			LastRelationshipAt:  relation.LastRelationshipAt,
			LastCapabilityType:  relation.LastCapabilityType,
			ProjectionState:     relation.ProjectionState,
		})
	}
	return out
}

func (s *Service) capabilitySummary(profile Profile, scope Scope, visible []BranchRelation) CapabilitySummary {
	summary := CapabilitySummary{
		CategoryAffinity: ModuleNotImplemented,
		ItemAffinity:     ModuleNotImplemented,
	}
	if scope.Global {
		summary.LastCapabilityType = profile.LastCapabilityType
		return summary
	}
	// For a branch-scoped caller the capability must come from a branch they
	// can see, not from the business-wide last order.
	var latest *time.Time
	for _, relation := range visible {
		if relation.LastRelationshipAt == nil {
			continue
		}
		if latest == nil || relation.LastRelationshipAt.After(*latest) {
			latest = relation.LastRelationshipAt
			summary.LastCapabilityType = relation.LastCapabilityType
		}
	}
	return summary
}
