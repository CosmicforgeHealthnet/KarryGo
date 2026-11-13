package models

import (
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Sales leads (Sections 14, 43.3, 59.6)
// ---------------------------------------------------------------------------
//
// A lead is a prospect the organization captured before qualifying it. It is
// tenant-private and is never a KarryGo marketplace customer identity: the
// only thing it can become is a company CRM customer (created or mapped) and,
// optionally, a sales opportunity against that customer.

// Lead sources.
const (
	LeadSourceWebsite      = "WEBSITE"
	LeadSourceReferral     = "REFERRAL"
	LeadSourceEvent        = "EVENT"
	LeadSourceInboundCall  = "INBOUND_CALL"
	LeadSourceInboundEmail = "INBOUND_EMAIL"
	LeadSourceOutbound     = "OUTBOUND"
	LeadSourcePartner      = "PARTNER"
	LeadSourceImport       = "IMPORT"
	LeadSourceOther        = "OTHER"
)

var validLeadSources = map[string]bool{
	LeadSourceWebsite: true, LeadSourceReferral: true, LeadSourceEvent: true,
	LeadSourceInboundCall: true, LeadSourceInboundEmail: true, LeadSourceOutbound: true,
	LeadSourcePartner: true, LeadSourceImport: true, LeadSourceOther: true,
}

// ValidLeadSource reports whether the lead source is recognised.
func ValidLeadSource(value string) bool { return validLeadSources[value] }

// Lead statuses.
const (
	LeadNew          = "NEW"
	LeadContacted    = "CONTACTED"
	LeadQualified    = "QUALIFIED"
	LeadDisqualified = "DISQUALIFIED"
	LeadConverted    = "CONVERTED"
)

// leadTransitions is the qualification matrix reachable through the status
// command. CONVERTED is deliberately absent: it is reachable only through the
// conversion command, which is the one place that creates the customer and
// opportunity atomically with the status change.
var leadTransitions = map[string]map[string]bool{
	LeadNew:          {LeadContacted: true, LeadQualified: true, LeadDisqualified: true},
	LeadContacted:    {LeadQualified: true, LeadDisqualified: true},
	LeadQualified:    {LeadContacted: true, LeadDisqualified: true},
	LeadDisqualified: {},
	LeadConverted:    {},
}

// ValidLeadStatus reports whether the status is recognised.
func ValidLeadStatus(value string) bool {
	_, ok := leadTransitions[value]
	return ok
}

// CanTransitionLead reports whether a qualification status change is allowed.
func CanTransitionLead(from, to string) bool { return leadTransitions[from][to] }

// TerminalLead reports whether the lead admits no further change.
func TerminalLead(status string) bool { return status == LeadDisqualified || status == LeadConverted }

// LeadConvertible reports whether a lead may be converted. A disqualified
// lead was judged not to be business; converting it would silently reverse
// that judgement.
func LeadConvertible(status string) bool {
	return status == LeadNew || status == LeadContacted || status == LeadQualified
}

// ValidateLeadScore enforces the 0..100 score range.
func ValidateLeadScore(score *int) error {
	if score != nil && (*score < 0 || *score > 100) {
		return errors.New("crm: lead score must be between 0 and 100")
	}
	return nil
}

// Lead is an organization-owned prospect.
type Lead struct {
	ID                       string
	OrganizationID           string
	BranchID                 string
	Source                   string
	CompanyName              string
	ContactSnapshot          map[string]any
	Status                   string
	OwnerUserID              string
	Score                    *int
	NextAction               string
	NextActionAt             *time.Time
	Notes                    string
	DisqualificationReason   string
	ConvertedCustomerID      string
	ConvertedOpportunityID   string
	ConvertedAt              *time.Time
	ConvertedBy              string
	ConversionIdempotencyKey string
	CreateIdempotencyKey     string
	CreateFingerprint        string
	Version                  int64
	CreatedBy                string
	UpdatedBy                string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// Lead history change types.
const (
	LeadChangeCreated       = "CREATED"
	LeadChangeUpdated       = "UPDATED"
	LeadChangeAssigned      = "ASSIGNED"
	LeadChangeStatusChanged = "STATUS_CHANGED"
	LeadChangeConverted     = "CONVERTED"
)

// LeadHistory is one immutable lead change.
type LeadHistory struct {
	ID             string
	OrganizationID string
	LeadID         string
	ChangeType     string
	PreviousStatus string
	NewStatus      string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	OccurredAt     time.Time
}
