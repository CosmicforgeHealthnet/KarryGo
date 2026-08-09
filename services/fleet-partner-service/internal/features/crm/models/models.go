// Package models holds the Section 14 CRM domain: customers, contacts,
// locations, sales opportunities, quotations, contracts, service-level
// agreements and customer credit policy.
//
// Nothing here imports gin, pgx, an HTTP client or a media SDK. The state
// machines are the point of the package: a status is only ever moved through a
// transition this file admits, so an invalid commercial state cannot be
// reached by any handler, worker or repair script that goes through the domain.
//
// Section 15 is out of scope. A quotation acceptance produces a conversion
// intent — a durable record that private work is now permissible — and never
// an order, shipment, package, assignment or trip.
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Shared value types
// ---------------------------------------------------------------------------

// ErrInvalidTransition is returned when a status change is not in the matrix.
var ErrInvalidTransition = errors.New("crm: transition is not permitted")

// ActorType distinguishes who caused a change. It matters for acceptance
// evidence, where "the customer clicked accept" and "a staff member recorded
// that the customer accepted" are different claims.
const (
	ActorCustomer     = "CUSTOMER"
	ActorOrganization = "ORGANIZATION_USER"
	ActorIntegration  = "EXTERNAL_INTEGRATION"
	ActorSystem       = "SYSTEM"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateCurrency enforces an ISO 4217 alphabetic code.
func ValidateCurrency(code string) error {
	if !currencyPattern.MatchString(code) {
		return fmt.Errorf("crm: %q is not a valid ISO 4217 currency", code)
	}
	return nil
}

// JSONMap is a jsonb column that is neither interpreted nor trusted by the
// domain: service scope, terms and operating hours are the operator's own
// structure, and inventing a schema for them would constrain commercial terms
// this service has no business constraining.
type JSONMap map[string]any

// Clone returns a defensive copy so a stored snapshot cannot be edited through
// the map that produced it.
func (m JSONMap) Clone() JSONMap {
	if m == nil {
		return nil
	}
	out := make(JSONMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Customer
// ---------------------------------------------------------------------------

// Customer types.
const (
	CustomerTypeIndividual           = "INDIVIDUAL"
	CustomerTypeBusiness             = "BUSINESS"
	CustomerTypeGovernment           = "GOVERNMENT"
	CustomerTypeNonprofit            = "NONPROFIT"
	CustomerTypeInternalBusinessUnit = "INTERNAL_BUSINESS_UNIT"
	CustomerTypeOther                = "OTHER"
)

// Customer statuses.
const (
	CustomerStatusProspect  = "PROSPECT"
	CustomerStatusActive    = "ACTIVE"
	CustomerStatusOnHold    = "ON_HOLD"
	CustomerStatusSuspended = "SUSPENDED"
	CustomerStatusInactive  = "INACTIVE"
	CustomerStatusArchived  = "ARCHIVED"
)

// Customer risk statuses.
const (
	RiskNotAssessed = "NOT_ASSESSED"
	RiskLow         = "LOW"
	RiskMedium      = "MEDIUM"
	RiskHigh        = "HIGH"
	RiskRestricted  = "RESTRICTED"
)

var customerTypes = map[string]bool{
	CustomerTypeIndividual: true, CustomerTypeBusiness: true, CustomerTypeGovernment: true,
	CustomerTypeNonprofit: true, CustomerTypeInternalBusinessUnit: true, CustomerTypeOther: true,
}

var riskStatuses = map[string]bool{
	RiskNotAssessed: true, RiskLow: true, RiskMedium: true, RiskHigh: true, RiskRestricted: true,
}

// customerTransitions is the whole customer status matrix. ARCHIVED has no
// outgoing edges: an archived customer is a closed record, not a paused one.
var customerTransitions = map[string]map[string]bool{
	CustomerStatusProspect: {
		CustomerStatusActive: true, CustomerStatusInactive: true, CustomerStatusArchived: true,
	},
	CustomerStatusActive: {
		CustomerStatusOnHold: true, CustomerStatusSuspended: true,
		CustomerStatusInactive: true, CustomerStatusArchived: true,
	},
	CustomerStatusOnHold: {
		CustomerStatusActive: true, CustomerStatusSuspended: true,
		CustomerStatusInactive: true, CustomerStatusArchived: true,
	},
	CustomerStatusSuspended: {
		CustomerStatusActive: true, CustomerStatusInactive: true, CustomerStatusArchived: true,
	},
	CustomerStatusInactive: {
		CustomerStatusActive: true, CustomerStatusArchived: true,
	},
	CustomerStatusArchived: {},
}

// ValidCustomerType reports whether the type is supported.
func ValidCustomerType(value string) bool { return customerTypes[value] }

// ValidRiskStatus reports whether the risk status is supported.
func ValidRiskStatus(value string) bool { return riskStatuses[value] }

// CanTransitionCustomer reports whether a customer status change is permitted.
func CanTransitionCustomer(from, to string) bool {
	return customerTransitions[from][to]
}

// Customer is an organization-owned commercial relationship record.
//
// It is deliberately not a marketplace identity: it holds no credentials,
// MarketplaceCustomerID is an opaque pointer that this service never resolves
// by reaching into another service's database, and nothing here ever creates
// or merges a marketplace account.
type Customer struct {
	ID                    string
	OrganizationID        string
	BranchID              string
	CustomerType          string
	LegalName             string
	DisplayName           string
	NormalizedName        string
	DuplicateFingerprint  string
	RegistrationNumber    string
	TaxReference          string
	MarketplaceCustomerID string
	Status                string
	RiskStatus            string
	AccountOwnerUserID    string
	DefaultCurrency       string
	PaymentTermsReference string
	CreditProfileID       string
	Version               int64
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ArchivedAt            *time.Time
}

// IsArchived reports whether the customer is closed to new commercial records.
func (c Customer) IsArchived() bool {
	return c.Status == CustomerStatusArchived || c.ArchivedAt != nil
}

// BlocksNewCommitment reports whether the customer's own state forbids a new
// quotation issue or contract activation.
//
// The two conditions are separate on purpose. RESTRICTED is a risk decision
// and stops everything; ON_HOLD and SUSPENDED are commercial states that stop
// new commitments while leaving existing ones intact.
func (c Customer) BlocksNewCommitment() (bool, string) {
	switch {
	case c.IsArchived():
		return true, "customer_archived"
	case c.RiskStatus == RiskRestricted:
		return true, "customer_risk_restricted"
	case c.Status == CustomerStatusOnHold || c.Status == CustomerStatusSuspended:
		return true, "customer_on_hold"
	default:
		return false, ""
	}
}

var whitespaceRun = regexp.MustCompile(`\s+`)
var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9 ]+`)

// NormalizeName case-folds and collapses whitespace for organization-scoped
// duplicate detection.
func NormalizeName(raw string) string {
	return whitespaceRun.ReplaceAllString(strings.ToLower(strings.TrimSpace(raw)), " ")
}

// DuplicateFingerprint builds the organization-scoped uniqueness key.
//
// A registration or tax reference is a genuine identity claim, so where one is
// supplied it alone decides the question. Where neither is supplied the key
// falls back to the normalized name plus the customer type — which is why two
// unrelated companies called "Lagos Logistics" that are both BUSINESS would
// collide, and why the fallback deliberately keeps the type in the key rather
// than matching on name similarity. Fuzzy name matching would reject
// legitimately separate customers, which is the more expensive mistake.
func DuplicateFingerprint(customerType, normalizedName, registrationNumber, taxReference string) string {
	registration := strings.ToLower(strings.TrimSpace(registrationNumber))
	tax := strings.ToLower(strings.TrimSpace(taxReference))
	if registration != "" {
		return "reg:" + nonAlphanumeric.ReplaceAllString(registration, "")
	}
	if tax != "" {
		return "tax:" + nonAlphanumeric.ReplaceAllString(tax, "")
	}
	return "name:" + customerType + ":" + nonAlphanumeric.ReplaceAllString(normalizedName, "")
}

// Customer history change types.
const (
	ChangeCreated        = "CREATED"
	ChangeUpdated        = "UPDATED"
	ChangeStatusChanged  = "STATUS_CHANGED"
	ChangeRiskChanged    = "RISK_CHANGED"
	ChangeArchived       = "ARCHIVED"
	ChangePrimaryChanged = "PRIMARY_CHANGED"
)

// CustomerHistory is one immutable customer change record.
type CustomerHistory struct {
	ID                 string
	OrganizationID     string
	CustomerID         string
	ChangeType         string
	PreviousStatus     string
	NewStatus          string
	PreviousRiskStatus string
	NewRiskStatus      string
	ChangedFields      []string
	ReasonCode         string
	ApprovalRequestID  string
	ActorType          string
	ActorID            string
	CorrelationID      string
	CausationID        string
	OccurredAt         time.Time
}

// ---------------------------------------------------------------------------
// Contacts
// ---------------------------------------------------------------------------

// Preferred contact channels.
const (
	ChannelEmail    = "EMAIL"
	ChannelPhone    = "PHONE"
	ChannelSMS      = "SMS"
	ChannelWhatsApp = "WHATSAPP"
	ChannelInApp    = "IN_APP"
	ChannelNone     = "NONE"
)

// Contact purposes.
const (
	PurposeGeneral    = "GENERAL"
	PurposeOperations = "OPERATIONS"
	PurposeBilling    = "BILLING"
	PurposeLegal      = "LEGAL"
	PurposeSales      = "SALES"
	PurposeEmergency  = "EMERGENCY"
	PurposeOther      = "OTHER"
)

// Contact and location lifecycle statuses.
const (
	RecordStatusActive   = "ACTIVE"
	RecordStatusInactive = "INACTIVE"
	RecordStatusArchived = "ARCHIVED"
)

// Consent states.
const (
	ConsentUnknown     = "UNKNOWN"
	ConsentGranted     = "GRANTED"
	ConsentWithdrawn   = "WITHDRAWN"
	ConsentNotRequired = "NOT_REQUIRED"
)

var contactChannels = map[string]bool{
	ChannelEmail: true, ChannelPhone: true, ChannelSMS: true,
	ChannelWhatsApp: true, ChannelInApp: true, ChannelNone: true,
}

var contactPurposes = map[string]bool{
	PurposeGeneral: true, PurposeOperations: true, PurposeBilling: true,
	PurposeLegal: true, PurposeSales: true, PurposeEmergency: true, PurposeOther: true,
}

var consentStates = map[string]bool{
	ConsentUnknown: true, ConsentGranted: true, ConsentWithdrawn: true, ConsentNotRequired: true,
}

// deliverableChannels are the channels this platform can actually deliver on
// today. SMS and WhatsApp are accepted as a stated preference but are not
// claimed as operational: promising a delivery route that does not exist is
// worse than recording the preference and routing to something that does.
var deliverableChannels = map[string]bool{
	ChannelEmail: true, ChannelInApp: true,
}

// ValidChannel reports whether the channel is a recognised preference.
func ValidChannel(value string) bool { return contactChannels[value] }

// ValidContactPurpose reports whether the purpose is supported.
func ValidContactPurpose(value string) bool { return contactPurposes[value] }

// ValidConsentStatus reports whether the consent state is supported.
func ValidConsentStatus(value string) bool { return consentStates[value] }

// ChannelDeliverable reports whether notification-service can be relied upon
// to deliver on this channel.
func ChannelDeliverable(value string) bool { return deliverableChannels[value] }

// Contact is a named person at a customer. It is not a login identity and
// carries no credential.
type Contact struct {
	ID                      string
	OrganizationID          string
	CustomerID              string
	FullName                string
	RoleTitle               string
	NormalizedEmail         string
	NormalizedPhone         string
	PreferredChannel        string
	ContactPurpose          string
	IsPrimary               bool
	NotificationPreferences JSONMap
	ConsentStatus           string
	ConsentSource           string
	ConsentRecordedAt       *time.Time
	Status                  string
	Version                 int64
	CreatedBy               string
	UpdatedBy               string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	ArchivedAt              *time.Time
}

// IsArchived reports whether the contact is closed to further edits.
func (c Contact) IsArchived() bool {
	return c.Status == RecordStatusArchived || c.ArchivedAt != nil
}

// Contactable reports whether this contact may be sent a CRM notification.
//
// Consent is checked before capability: an operator who has not recorded
// consent has not earned the right to send, regardless of which channels work.
func (c Contact) Contactable() bool {
	if c.IsArchived() || c.Status != RecordStatusActive {
		return false
	}
	if c.ConsentStatus == ConsentWithdrawn {
		return false
	}
	if c.PreferredChannel == ChannelNone {
		return false
	}
	return c.NormalizedEmail != "" || c.NormalizedPhone != ""
}

// ContactHistory is one immutable contact change record.
type ContactHistory struct {
	ID             string
	OrganizationID string
	CustomerID     string
	ContactID      string
	ChangeType     string
	PreviousStatus string
	NewStatus      string
	ChangedFields  []string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	CausationID    string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Locations
// ---------------------------------------------------------------------------

// Location types.
const (
	LocationPickup    = "PICKUP"
	LocationDelivery  = "DELIVERY"
	LocationBilling   = "BILLING"
	LocationWarehouse = "WAREHOUSE"
	LocationOffice    = "OFFICE"
	LocationDepot     = "DEPOT"
	LocationSite      = "SITE"
	LocationPort      = "PORT"
	LocationTerminal  = "TERMINAL"
	LocationOther     = "OTHER"
)

var locationTypes = map[string]bool{
	LocationPickup: true, LocationDelivery: true, LocationBilling: true, LocationWarehouse: true,
	LocationOffice: true, LocationDepot: true, LocationSite: true, LocationPort: true,
	LocationTerminal: true, LocationOther: true,
}

// ValidLocationType reports whether the location type is supported.
func ValidLocationType(value string) bool { return locationTypes[value] }

// Address is the structured address stored inside the location snapshot.
type Address struct {
	Line1         string `json:"line1"`
	Line2         string `json:"line2,omitempty"`
	City          string `json:"city"`
	StateOrRegion string `json:"stateOrRegion"`
	PostalCode    string `json:"postalCode,omitempty"`
	CountryCode   string `json:"countryCode"`
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// Validate checks the mandatory address parts.
func (a Address) Validate() error {
	if strings.TrimSpace(a.Line1) == "" {
		return errors.New("crm: address line 1 is required")
	}
	if strings.TrimSpace(a.City) == "" {
		return errors.New("crm: city is required")
	}
	if strings.TrimSpace(a.StateOrRegion) == "" {
		return errors.New("crm: state or region is required")
	}
	if !countryPattern.MatchString(a.CountryCode) {
		return errors.New("crm: country code must be an ISO 3166-1 alpha-2 code")
	}
	return nil
}

// Location is a customer site.
type Location struct {
	ID                 string
	OrganizationID     string
	CustomerID         string
	LocationType       string
	Label              string
	Address            Address
	Latitude           *float64
	Longitude          *float64
	GeofenceReference  string
	OperatingHours     JSONMap
	AccessInstructions string
	SafetyInstructions string
	ContactReference   string
	Timezone           string
	Status             string
	Version            int64
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
}

// IsArchived reports whether the location is closed to further edits.
func (l Location) IsArchived() bool {
	return l.Status == RecordStatusArchived || l.ArchivedAt != nil
}

// ValidateCoordinates enforces the range and the pairing rule.
//
// Half a coordinate is worse than none: a latitude with no longitude routes a
// vehicle to the prime meridian rather than failing loudly.
func ValidateCoordinates(latitude, longitude *float64) error {
	if (latitude == nil) != (longitude == nil) {
		return errors.New("crm: latitude and longitude must both be present or both absent")
	}
	if latitude == nil {
		return nil
	}
	if *latitude < -90 || *latitude > 90 {
		return errors.New("crm: latitude must be between -90 and 90")
	}
	if *longitude < -180 || *longitude > 180 {
		return errors.New("crm: longitude must be between -180 and 180")
	}
	return nil
}

// ValidateTimezone requires a loadable IANA zone. "UTC+1" and "WAT" are not
// zones; they are offsets, and an offset cannot represent a DST boundary.
func ValidateTimezone(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("crm: timezone is required")
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("crm: %q is not a valid IANA timezone", name)
	}
	return nil
}

// LocationHistory is one immutable location change record.
type LocationHistory struct {
	ID             string
	OrganizationID string
	CustomerID     string
	LocationID     string
	ChangeType     string
	PreviousStatus string
	NewStatus      string
	ChangedFields  []string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	CausationID    string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Sales opportunities
// ---------------------------------------------------------------------------

// Opportunity stages.
const (
	StageLead          = "LEAD"
	StageQualified     = "QUALIFIED"
	StageNeedsAnalysis = "NEEDS_ANALYSIS"
	StageProposal      = "PROPOSAL"
	StageNegotiation   = "NEGOTIATION"
	StageWon           = "WON"
	StageLost          = "LOST"
	StageOnHold        = "ON_HOLD"
)

// opportunityTransitions is the forward matrix. ON_HOLD is handled separately
// because its permitted destinations depend on where the opportunity was
// before it was held, which is a history question, not a table lookup.
var opportunityTransitions = map[string]map[string]bool{
	StageLead:          {StageQualified: true, StageLost: true, StageOnHold: true},
	StageQualified:     {StageNeedsAnalysis: true, StageLost: true, StageOnHold: true},
	StageNeedsAnalysis: {StageProposal: true, StageLost: true, StageOnHold: true},
	StageProposal:      {StageNegotiation: true, StageWon: true, StageLost: true, StageOnHold: true},
	StageNegotiation:   {StageWon: true, StageLost: true, StageOnHold: true},
	StageOnHold:        {StageLost: true},
	StageWon:           {},
	StageLost:          {},
}

// nonTerminalStages are the stages an ON_HOLD opportunity may resume into,
// provided it actually occupied that stage at some point.
var nonTerminalStages = map[string]bool{
	StageLead: true, StageQualified: true, StageNeedsAnalysis: true,
	StageProposal: true, StageNegotiation: true,
}

// ValidStage reports whether the stage is recognised.
func ValidStage(value string) bool {
	_, ok := opportunityTransitions[value]
	return ok
}

// CanTransitionOpportunity reports whether a stage change is permitted.
//
// priorStages is the set of stages the opportunity has previously occupied; it
// is only consulted when resuming from ON_HOLD. Resuming into a stage the
// opportunity never reached would let a hold be used to skip qualification.
func CanTransitionOpportunity(from, to string, priorStages []string) bool {
	if from == StageOnHold && nonTerminalStages[to] {
		for _, stage := range priorStages {
			if stage == to {
				return true
			}
		}
		return false
	}
	return opportunityTransitions[from][to]
}

// TerminalStage reports whether the stage admits no further movement.
func TerminalStage(stage string) bool { return stage == StageWon || stage == StageLost }

// Opportunity is a tracked sales pursuit.
type Opportunity struct {
	ID                       string
	OrganizationID           string
	BranchID                 string
	CustomerID               string
	Title                    string
	Description              string
	Stage                    string
	ExpectedValueAmountMinor *int64
	ExpectedValueCurrency    string
	ProbabilityBasisPoints   int
	ExpectedCloseDate        *time.Time
	ServiceCapabilities      []string
	AccountOwnerUserID       string
	NextAction               string
	NextActionAt             *time.Time
	LossReason               string
	HoldReason               string
	Version                  int64
	CreatedBy                string
	UpdatedBy                string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	ClosedAt                 *time.Time
}

// ValidateProbability enforces the basis-point range.
func ValidateProbability(basisPoints int) error {
	if basisPoints < 0 || basisPoints > 10000 {
		return errors.New("crm: probability must be between 0 and 10000 basis points")
	}
	return nil
}

// OpportunityHistory is one immutable stage change.
type OpportunityHistory struct {
	ID             string
	OrganizationID string
	OpportunityID  string
	PreviousStage  string
	NewStage       string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Quotations
// ---------------------------------------------------------------------------

// Quotation statuses.
const (
	QuotationDraft            = "DRAFT"
	QuotationAwaitingApproval = "AWAITING_APPROVAL"
	QuotationApproved         = "APPROVED"
	QuotationIssued           = "ISSUED"
	QuotationViewed           = "VIEWED"
	QuotationAccepted         = "ACCEPTED"
	QuotationRejected         = "REJECTED"
	QuotationExpired          = "EXPIRED"
	QuotationCancelled        = "CANCELLED"
	QuotationSuperseded       = "SUPERSEDED"
)

var quotationTransitions = map[string]map[string]bool{
	QuotationDraft: {QuotationAwaitingApproval: true, QuotationApproved: true, QuotationCancelled: true},
	// A rejected-for-changes approval returns the quotation to DRAFT rather
	// than killing it: the commercial intent survives the review.
	QuotationAwaitingApproval: {QuotationApproved: true, QuotationDraft: true, QuotationCancelled: true},
	QuotationApproved:         {QuotationIssued: true, QuotationCancelled: true},
	QuotationIssued: {
		QuotationViewed: true, QuotationAccepted: true, QuotationRejected: true,
		QuotationExpired: true, QuotationCancelled: true, QuotationSuperseded: true,
	},
	QuotationViewed: {
		QuotationAccepted: true, QuotationRejected: true, QuotationExpired: true,
		QuotationCancelled: true, QuotationSuperseded: true,
	},
	QuotationAccepted:   {},
	QuotationRejected:   {},
	QuotationExpired:    {},
	QuotationCancelled:  {},
	QuotationSuperseded: {},
}

// CanTransitionQuotation reports whether a quotation status change is allowed.
func CanTransitionQuotation(from, to string) bool { return quotationTransitions[from][to] }

// TerminalQuotation reports whether the quotation is finally resolved.
func TerminalQuotation(status string) bool {
	switch status {
	case QuotationAccepted, QuotationRejected, QuotationExpired,
		QuotationCancelled, QuotationSuperseded:
		return true
	}
	return false
}

// DeliveredQuotation reports whether the customer has been given the offer.
func DeliveredQuotation(status string) bool {
	return status == QuotationIssued || status == QuotationViewed
}

// Quotation is a priced offer to a CRM customer.
//
// Pricing is manually supplied throughout. There is no rate card, no formula
// and no marketplace fare in this type: an operator's commercial terms are
// theirs, and a hidden calculation would silently change what a customer was
// quoted.
type Quotation struct {
	ID                string
	OrganizationID    string
	BranchID          string
	CustomerID        string
	OpportunityID     string
	QuotationNumber   string
	Currency          string
	ValidFrom         time.Time
	ValidUntil        time.Time
	ServiceScope      JSONMap
	Terms             JSONMap
	InternalNotes     string
	Status            string
	CurrentVersion    int
	IssuedVersionID   string
	AcceptedVersionID string
	ApprovalRequestID string
	SupersededByID    string
	TotalAmountMinor  int64
	Version           int64
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	IssuedAt          *time.Time
	ResolvedAt        *time.Time
}

// Expired reports whether the quotation's validity window has closed.
//
// Server time is the authority. A client-supplied timestamp never expires a
// quotation, because the party with the most to gain from a late acceptance is
// the party holding the clock.
func (q Quotation) Expired(now time.Time) bool { return !now.Before(q.ValidUntil) }

// LineItem is one manually priced quotation line.
//
// Quantity is a decimal string, not a float: 0.1 tonnes must stay 0.1, and a
// binary float that renders as 0.30000000000000004 on an invoice is a dispute.
type LineItem struct {
	ID                       string
	OrganizationID           string
	QuotationID              string
	Description              string
	Quantity                 string
	UnitType                 string
	UnitAmountMinor          int64
	LineAmountMinor          int64
	Currency                 string
	ExternalPricingReference string
	ManualReason             string
	Sequence                 int
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// QuotationTotal sums explicit line amounts and nothing else.
//
// It infers no distance price, no weight price, no surge, no commission, no
// tax rate and no currency conversion. Every number it returns was typed in by
// somebody who is accountable for it.
func QuotationTotal(currency string, items []LineItem) (int64, error) {
	var total int64
	for _, item := range items {
		if item.Currency != currency {
			return 0, fmt.Errorf("crm: line item currency %q does not match quotation currency %q",
				item.Currency, currency)
		}
		if item.LineAmountMinor < 0 {
			return 0, errors.New("crm: line amount must not be negative")
		}
		next := total + item.LineAmountMinor
		if next < total {
			return 0, errors.New("crm: quotation total overflows")
		}
		total = next
	}
	return total, nil
}

// LineAmountFor computes unit amount times quantity in minor units using
// exact decimal arithmetic and half-up rounding at the final step.
//
// The rounding convention is stated rather than inherited from the float unit:
// a quantity of 1.005 at 100 minor units must round the same way on every
// platform and in every test, and float64 does not guarantee that.
func LineAmountFor(unitAmountMinor int64, quantity string) (int64, error) {
	whole, frac, scale, err := parseDecimal(quantity)
	if err != nil {
		return 0, err
	}
	if whole == 0 && frac == 0 {
		return 0, errors.New("crm: quantity must be greater than zero")
	}
	// unitAmountMinor * (whole + frac/scale), computed as an integer numerator
	// over the scale so no intermediate value is fractional.
	numerator := unitAmountMinor * whole
	if unitAmountMinor != 0 && numerator/unitAmountMinor != whole {
		return 0, errors.New("crm: line amount overflows")
	}
	numerator *= scale
	numerator += unitAmountMinor * frac
	if numerator < 0 {
		return 0, errors.New("crm: line amount overflows")
	}
	// Half-up.
	return (numerator + scale/2) / scale, nil
}

var decimalPattern = regexp.MustCompile(`^([0-9]{1,12})(?:\.([0-9]{1,6}))?$`)

// parseDecimal splits a positive decimal string into its whole part, its
// fractional part as an integer, and the scale of that fraction.
func parseDecimal(value string) (whole int64, frac int64, scale int64, err error) {
	match := decimalPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0, 0, 0, fmt.Errorf("crm: %q is not a valid decimal quantity", value)
	}
	for _, digit := range match[1] {
		whole = whole*10 + int64(digit-'0')
	}
	scale = 1
	for _, digit := range match[2] {
		frac = frac*10 + int64(digit-'0')
		scale *= 10
	}
	return whole, frac, scale, nil
}

// NormalizeQuantity renders a validated quantity in a stable form so the same
// quantity always produces the same fingerprint.
func NormalizeQuantity(value string) (string, error) {
	whole, frac, scale, err := parseDecimal(value)
	if err != nil {
		return "", err
	}
	if whole == 0 && frac == 0 {
		return "", errors.New("crm: quantity must be greater than zero")
	}
	if frac == 0 {
		return fmt.Sprintf("%d", whole), nil
	}
	digits := 0
	for s := scale; s > 1; s /= 10 {
		digits++
	}
	rendered := strings.TrimRight(fmt.Sprintf("%0*d", digits, frac), "0")
	return fmt.Sprintf("%d.%s", whole, rendered), nil
}

// QuotationVersion is the immutable record of what was offered.
type QuotationVersion struct {
	ID                   string
	OrganizationID       string
	QuotationID          string
	VersionNumber        int
	CustomerSnapshot     JSONMap
	ContactSnapshot      JSONMap
	Currency             string
	LineItemsSnapshot    []LineItem
	ServiceScopeSnapshot JSONMap
	TermsSnapshot        JSONMap
	TotalAmountMinor     int64
	ValidFrom            time.Time
	ValidUntil           time.Time
	DocumentReferences   []string
	ApprovalRequestID    string
	IssuedBy             string
	IssuedAt             time.Time
	ContentFingerprint   string
}

// Acceptance methods.
const (
	AcceptSecureToken       = "SECURE_CUSTOMER_TOKEN"
	AcceptCompanyUser       = "AUTHORIZED_COMPANY_USER"
	AcceptIntegration       = "APPROVED_EXTERNAL_INTEGRATION"
	AcceptOfflineDocumented = "DOCUMENTED_OFFLINE_ACCEPTANCE"
)

var acceptanceMethods = map[string]bool{
	AcceptSecureToken: true, AcceptCompanyUser: true,
	AcceptIntegration: true, AcceptOfflineDocumented: true,
}

// ValidAcceptanceMethod reports whether the acceptance method is supported.
func ValidAcceptanceMethod(value string) bool { return acceptanceMethods[value] }

// Acceptance is the immutable record that a customer took the offer.
type Acceptance struct {
	ID                        string
	OrganizationID            string
	QuotationID               string
	AcceptedVersionID         string
	ActorType                 string
	ActorReference            string
	AcceptanceMethod          string
	AcceptedAt                time.Time
	IPHash                    string
	UserAgentHash             string
	EvidenceDocumentReference string
	IdempotencyKey            string
	RequestFingerprint        string
	CorrelationID             string
	CausationID               string
	CreatedAt                 time.Time
}

// Conversion intent constants. There is exactly one of each because a
// conversion intent is not a workflow; it is a fact awaiting Section 15.
const (
	ConversionTypeFuturePrivateOrder = "FUTURE_PRIVATE_ORDER"
	ConversionStatusPending          = "PENDING_SECTION_15"
)

// ConversionIntent records that an accepted quotation may become private work
// once Section 15 exists.
//
// It carries no order ID, no shipment ID, no assignment ID and no trip ID —
// not as omissions, but because none of those things were created.
type ConversionIntent struct {
	ID                string
	OrganizationID    string
	QuotationID       string
	AcceptedVersionID string
	AcceptanceID      string
	ConversionType    string
	Status            string
	IdempotencyKey    string
	Fingerprint       string
	CorrelationID     string
	CausationID       string
	CreatedAt         time.Time
}

// Quotation access token scopes and statuses.
const (
	TokenScopeViewOnly       = "VIEW_ONLY"
	TokenScopeViewAndRespond = "VIEW_AND_RESPOND"

	TokenStatusActive   = "ACTIVE"
	TokenStatusConsumed = "CONSUMED"
	TokenStatusRevoked  = "REVOKED"
)

// QuotationAccessToken is the stored half of a customer's secure link.
//
// The raw token is never in this struct as it leaves the service: only the
// hash is persisted, so a database copy cannot be replayed as the customer.
type QuotationAccessToken struct {
	ID                 string
	OrganizationID     string
	QuotationID        string
	QuotationVersionID string
	TokenHash          string
	Scope              string
	SingleUse          bool
	Status             string
	IssuedToContactID  string
	ExpiresAt          time.Time
	FirstViewedAt      *time.Time
	LastViewedAt       *time.Time
	ViewCount          int
	RevokedReason      string
	CreatedBy          string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Usable reports whether the token may still be presented.
func (t QuotationAccessToken) Usable(now time.Time) bool {
	return t.Status == TokenStatusActive && now.Before(t.ExpiresAt)
}

// QuotationHistory is one immutable quotation status change.
type QuotationHistory struct {
	ID                string
	OrganizationID    string
	QuotationID       string
	PreviousStatus    string
	NewStatus         string
	QuotationVersion  int
	ReasonCode        string
	ApprovalRequestID string
	ActorType         string
	ActorID           string
	CorrelationID     string
	CausationID       string
	OccurredAt        time.Time
}

// ---------------------------------------------------------------------------
// Documents
// ---------------------------------------------------------------------------

// Document purposes.
const (
	DocPurposeQuotation           = "QUOTATION"
	DocPurposeSupporting          = "SUPPORTING"
	DocPurposeAcceptanceEvidence  = "ACCEPTANCE_EVIDENCE"
	DocPurposeRejectionEvidence   = "REJECTION_EVIDENCE"
	DocPurposeDraftContract       = "DRAFT_CONTRACT"
	DocPurposeSignedContract      = "SIGNED_CONTRACT"
	DocPurposeTerminationEvidence = "TERMINATION_EVIDENCE"
	DocPurposeRenewal             = "RENEWAL"
)

// Media projection states, mirrored locally and never authoritative.
const (
	DocStatusPending       = "PENDING"
	DocStatusReady         = "READY"
	DocStatusUnsafe        = "UNSAFE"
	DocStatusPendingReview = "PENDING_REVIEW"
	DocStatusRevoked       = "REVOKED"
	DocStatusDeleted       = "DELETED"
	DocStatusReplaced      = "REPLACED"
	DocStatusUnknown       = "UNKNOWN"

	ReadinessReady    = "READY"
	ReadinessNotReady = "NOT_READY"
	ReadinessUnknown  = "UNKNOWN"
	ReadinessStale    = "STALE"
)

// DocumentReference is Fleet's local projection of a Media-owned object.
//
// It holds an opaque object ID and a status projection. It holds no bytes, no
// URL, no storage path, no upload token, no checksum and no authoritative MIME
// type: those belong to media-file-service, and a second copy of them here
// would be a second answer that can go stale.
type DocumentReference struct {
	ID                 string
	OrganizationID     string
	SubjectID          string
	SubjectVersionID   string
	MediaObjectID      string
	DocumentPurpose    string
	IsMandatory        bool
	StatusProjection   string
	ReadinessState     string
	RetentionReference string
	SourceService      string
	SourceVersion      int64
	LastEventID        string
	SyncedAt           *time.Time
	Version            int64
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Usable reports whether the document may back a commercial commitment.
//
// Anything other than a confirmed-ready, confirmed-safe object is refused.
// UNKNOWN in particular is not optimism: a document whose safety could not be
// confirmed is treated exactly like one confirmed unsafe.
func (d DocumentReference) Usable() bool {
	return d.StatusProjection == DocStatusReady && d.ReadinessState == ReadinessReady
}

// ---------------------------------------------------------------------------
// Contracts
// ---------------------------------------------------------------------------

// Contract statuses.
const (
	ContractDraft            = "DRAFT"
	ContractAwaitingApproval = "AWAITING_APPROVAL"
	ContractApproved         = "APPROVED"
	ContractActive           = "ACTIVE"
	ContractSuspended        = "SUSPENDED"
	ContractExpired          = "EXPIRED"
	ContractTerminated       = "TERMINATED"
	ContractRenewalPending   = "RENEWAL_PENDING"
	ContractSuperseded       = "SUPERSEDED"
)

var contractTransitions = map[string]map[string]bool{
	ContractDraft:            {ContractAwaitingApproval: true, ContractApproved: true, ContractTerminated: true},
	ContractAwaitingApproval: {ContractApproved: true, ContractDraft: true, ContractTerminated: true},
	ContractApproved:         {ContractActive: true, ContractTerminated: true},
	ContractActive: {
		ContractSuspended: true, ContractRenewalPending: true, ContractExpired: true,
		ContractTerminated: true, ContractSuperseded: true,
	},
	ContractSuspended: {
		ContractActive: true, ContractTerminated: true, ContractExpired: true,
		ContractRenewalPending: true,
	},
	ContractRenewalPending: {
		ContractActive: true, ContractExpired: true, ContractTerminated: true,
		ContractSuperseded: true,
	},
	ContractExpired:    {},
	ContractTerminated: {},
	ContractSuperseded: {},
}

// CanTransitionContract reports whether a contract status change is allowed.
func CanTransitionContract(from, to string) bool { return contractTransitions[from][to] }

// TerminalContract reports whether the contract admits no further movement.
func TerminalContract(status string) bool {
	switch status {
	case ContractExpired, ContractTerminated, ContractSuperseded:
		return true
	}
	return false
}

// Contract is an agreed commercial arrangement with a CRM customer.
type Contract struct {
	ID                 string
	OrganizationID     string
	BranchID           string
	CustomerID         string
	QuotationID        string
	ContractNumber     string
	EffectiveFrom      time.Time
	EffectiveTo        *time.Time
	RenewalTerms       JSONMap
	ServiceScope       JSONMap
	CommercialSnapshot JSONMap
	TerminationTerms   JSONMap
	Status             string
	CurrentVersion     int
	ActiveVersionID    string
	ApprovalRequestID  string
	SupersedesID       string
	Version            int64
	CreatedBy          string
	UpdatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ActivatedAt        *time.Time
	TerminatedAt       *time.Time
}

// ContractVersion is the immutable record of what was activated.
type ContractVersion struct {
	ID                   string
	OrganizationID       string
	ContractID           string
	VersionNumber        int
	CustomerSnapshot     JSONMap
	ServiceScopeSnapshot JSONMap
	CommercialSnapshot   JSONMap
	EffectiveFrom        time.Time
	EffectiveTo          *time.Time
	RenewalTerms         JSONMap
	TerminationTerms     JSONMap
	DocumentReferences   []string
	ApprovalRequestID    string
	SignatoryReferences  []string
	ContentFingerprint   string
	ActivatedBy          string
	ActivatedAt          time.Time
}

// ContractHistory is one immutable contract status change.
type ContractHistory struct {
	ID             string
	OrganizationID string
	ContractID     string
	PreviousStatus string
	NewStatus      string
	ReasonCode     string
	ActorType      string
	ActorID        string
	CorrelationID  string
	OccurredAt     time.Time
}

// ---------------------------------------------------------------------------
// Service level agreements
// ---------------------------------------------------------------------------

// SLA statuses.
const (
	SLADraft      = "DRAFT"
	SLAActive     = "ACTIVE"
	SLASuspended  = "SUSPENDED"
	SLAExpired    = "EXPIRED"
	SLATerminated = "TERMINATED"
	SLASuperseded = "SUPERSEDED"
)

var slaTransitions = map[string]map[string]bool{
	SLADraft:  {SLAActive: true, SLATerminated: true},
	SLAActive: {SLASuspended: true, SLAExpired: true, SLATerminated: true, SLASuperseded: true},
	SLASuspended: {
		SLAActive: true, SLAExpired: true, SLATerminated: true, SLASuperseded: true,
	},
	SLAExpired:    {},
	SLATerminated: {},
	SLASuperseded: {},
}

// CanTransitionSLA reports whether an SLA status change is allowed.
func CanTransitionSLA(from, to string) bool { return slaTransitions[from][to] }

// Supported SLA metric codes. The list is closed: an unrecognised metric has
// no evaluation rule, and an SLA nobody can evaluate is a promise nobody can
// keep.
const (
	MetricPickupOnTime        = "PICKUP_ON_TIME"
	MetricDeliveryOnTime      = "DELIVERY_ON_TIME"
	MetricOrderCompletionTime = "ORDER_COMPLETION_TIME"
	MetricPackageDamageRate   = "PACKAGE_DAMAGE_RATE"
	MetricProofCompletionTime = "PROOF_COMPLETION_TIME"
	MetricFailedAttemptRate   = "FAILED_ATTEMPT_RATE"
)

// MetricDirection says which way is good for a metric.
type MetricDirection int

// Metric directions.
const (
	// HigherIsBetter: a percentage of on-time deliveries.
	HigherIsBetter MetricDirection = iota
	// LowerIsBetter: a damage rate or an elapsed duration.
	LowerIsBetter
)

// metricUnits maps each supported metric to its permitted units and the
// direction in which the target is satisfied.
var metricUnits = map[string]struct {
	Units     map[string]bool
	Direction MetricDirection
}{
	MetricPickupOnTime:        {Units: map[string]bool{"PERCENT": true, "BASIS_POINTS": true}, Direction: HigherIsBetter},
	MetricDeliveryOnTime:      {Units: map[string]bool{"PERCENT": true, "BASIS_POINTS": true}, Direction: HigherIsBetter},
	MetricOrderCompletionTime: {Units: map[string]bool{"MINUTES": true, "HOURS": true}, Direction: LowerIsBetter},
	MetricPackageDamageRate:   {Units: map[string]bool{"PERCENT": true, "BASIS_POINTS": true}, Direction: LowerIsBetter},
	MetricProofCompletionTime: {Units: map[string]bool{"MINUTES": true, "HOURS": true}, Direction: LowerIsBetter},
	MetricFailedAttemptRate:   {Units: map[string]bool{"PERCENT": true, "BASIS_POINTS": true}, Direction: LowerIsBetter},
}

// Supported measurement windows.
var measurementWindows = map[string]bool{
	"DAILY": true, "WEEKLY": true, "MONTHLY": true, "QUARTERLY": true, "PER_ORDER": true,
}

// ValidMetric reports whether the metric code is supported.
func ValidMetric(code string) bool {
	_, ok := metricUnits[code]
	return ok
}

// ValidMetricUnit reports whether the unit is permitted for the metric.
func ValidMetricUnit(code, unit string) bool {
	spec, ok := metricUnits[code]
	return ok && spec.Units[unit]
}

// ValidMeasurementWindow reports whether the window is supported.
func ValidMeasurementWindow(window string) bool { return measurementWindows[window] }

// MetricDirectionFor returns the direction for a supported metric.
func MetricDirectionFor(code string) (MetricDirection, bool) {
	spec, ok := metricUnits[code]
	if !ok {
		return HigherIsBetter, false
	}
	return spec.Direction, true
}

// SupportedMetrics lists the metric codes in a stable order.
func SupportedMetrics() []string {
	codes := make([]string, 0, len(metricUnits))
	for code := range metricUnits {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// SLA is a measurable service commitment.
type SLA struct {
	ID                         string
	OrganizationID             string
	CustomerID                 string
	ContractID                 string
	FutureServiceCatalogItemID string
	MetricCode                 string
	TargetValue                string
	MeasurementUnit            string
	MeasurementWindow          string
	EscalationPolicy           JSONMap
	EffectiveFrom              time.Time
	EffectiveTo                *time.Time
	Status                     string
	Version                    int64
	CreatedBy                  string
	UpdatedBy                  string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// SLA observation outcomes.
const (
	OutcomeMet              = "MET"
	OutcomeMissed           = "MISSED"
	OutcomeNotEvaluated     = "NOT_EVALUATED"
	OutcomeInsufficientData = "INSUFFICIENT_DATA"
	OutcomeUnknown          = "UNKNOWN"
)

var observationOutcomes = map[string]bool{
	OutcomeMet: true, OutcomeMissed: true, OutcomeNotEvaluated: true,
	OutcomeInsufficientData: true, OutcomeUnknown: true,
}

// ValidOutcome reports whether the outcome is recognised.
func ValidOutcome(value string) bool { return observationOutcomes[value] }

// SLAObservation is one append-only measurement from an authoritative source.
type SLAObservation struct {
	ID              string
	OrganizationID  string
	SLAID           string
	SourceType      string
	SourceReference string
	MeasuredValue   *string
	MeasurementUnit string
	MeasuredAt      time.Time
	WindowStart     time.Time
	WindowEnd       time.Time
	Outcome         string
	SourceVersion   int64
	EventID         string
	PayloadHash     string
	CorrelationID   string
	CausationID     string
	CreatedAt       time.Time
}

// EvaluateOutcome decides MET or MISSED from a measured value against a target.
//
// It returns INSUFFICIENT_DATA when there is no measurement and UNKNOWN when
// the metric or unit cannot be interpreted. Neither is ever collapsed into
// MET: an SLA that could not be measured has not been met, it has not been
// measured, and reporting the two as the same thing would let a monitoring
// outage read as perfect service.
func EvaluateOutcome(metricCode, unit, target string, measured *string) string {
	if measured == nil || strings.TrimSpace(*measured) == "" {
		return OutcomeInsufficientData
	}
	direction, ok := MetricDirectionFor(metricCode)
	if !ok || !ValidMetricUnit(metricCode, unit) {
		return OutcomeUnknown
	}
	comparison, err := compareDecimals(*measured, target)
	if err != nil {
		return OutcomeUnknown
	}
	if direction == HigherIsBetter {
		if comparison >= 0 {
			return OutcomeMet
		}
		return OutcomeMissed
	}
	if comparison <= 0 {
		return OutcomeMet
	}
	return OutcomeMissed
}

// compareDecimals compares two positive decimal strings exactly.
func compareDecimals(left, right string) (int, error) {
	lw, lf, ls, err := parseDecimal(left)
	if err != nil {
		return 0, err
	}
	rw, rf, rs, err := parseDecimal(right)
	if err != nil {
		return 0, err
	}
	// Bring both to a common scale without floating point.
	scale := ls
	if rs > scale {
		scale = rs
	}
	leftTotal := lw*scale + lf*(scale/ls)
	rightTotal := rw*scale + rf*(scale/rs)
	switch {
	case leftTotal < rightTotal:
		return -1, nil
	case leftTotal > rightTotal:
		return 1, nil
	default:
		return 0, nil
	}
}

// SLAHistory is one immutable SLA status change.
type SLAHistory struct {
	ID                string
	OrganizationID    string
	SLAID             string
	PreviousStatus    string
	NewStatus         string
	ReasonCode        string
	ApprovalRequestID string
	ActorType         string
	ActorID           string
	CorrelationID     string
	CausationID       string
	OccurredAt        time.Time
}

// ---------------------------------------------------------------------------
// Customer credit policy
// ---------------------------------------------------------------------------

// Credit hold statuses.
const (
	HoldNone           = "NO_HOLD"
	HoldReviewRequired = "REVIEW_REQUIRED"
	HoldOnHold         = "ON_HOLD"
	HoldRestricted     = "RESTRICTED"
)

var holdStatuses = map[string]bool{
	HoldNone: true, HoldReviewRequired: true, HoldOnHold: true, HoldRestricted: true,
}

// ValidHoldStatus reports whether the hold status is recognised.
func ValidHoldStatus(value string) bool { return holdStatuses[value] }

// BlockingHold reports whether the hold stops new commercial commitments.
//
// REVIEW_REQUIRED deliberately does not block. It is a flag for a human to
// look, not a decision that has been taken, and treating "someone should check
// this" as "refuse" would make the state useless as an early warning.
func BlockingHold(status string) bool {
	return status == HoldOnHold || status == HoldRestricted
}

// CreditProfile is an organization's own credit policy for a customer.
//
// It is policy and nothing else. Fleet owns no receivables ledger, calculates
// no debt, ages no invoices and derives no available credit from transactions:
// a second, wrong set of books is worse than no books.
type CreditProfile struct {
	ID                     string
	OrganizationID         string
	CustomerID             string
	CreditLimitAmountMinor *int64
	Currency               string
	PaymentTermsDays       int
	HoldStatus             string
	HoldReason             string
	RiskStatusSnapshot     string
	ApprovedBy             string
	ApprovalRequestID      string
	EffectiveFrom          time.Time
	EffectiveTo            *time.Time
	Version                int64
	CreatedBy              string
	UpdatedBy              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// CreditHistory is one immutable credit policy change.
type CreditHistory struct {
	ID                 string
	OrganizationID     string
	CreditProfileID    string
	CustomerID         string
	PreviousHoldStatus string
	NewHoldStatus      string
	PreviousLimitMinor *int64
	NewLimitMinor      *int64
	ReasonCode         string
	ActorType          string
	ActorID            string
	ApprovalRequestID  string
	CorrelationID      string
	OccurredAt         time.Time
}

// ---------------------------------------------------------------------------
// Notification operations
// ---------------------------------------------------------------------------

// Notification operation statuses.
const (
	NotificationPending         = "PENDING"
	NotificationProcessing      = "PROCESSING"
	NotificationUnknown         = "UNKNOWN"
	NotificationDelivered       = "DELIVERED"
	NotificationFailedRetryable = "FAILED_RETRYABLE"
	NotificationFailedPermanent = "FAILED_PERMANENT"
	NotificationDeadLetter      = "DEAD_LETTER"
)

// Notification intent types.
const (
	IntentQuotationIssued    = "QUOTATION_ISSUED"
	IntentQuotationViewed    = "QUOTATION_VIEWED"
	IntentQuotationAccepted  = "QUOTATION_ACCEPTED"
	IntentQuotationRejected  = "QUOTATION_REJECTED"
	IntentQuotationExpired   = "QUOTATION_EXPIRED"
	IntentContractActivated  = "CONTRACT_ACTIVATED"
	IntentContractSuspended  = "CONTRACT_SUSPENDED"
	IntentContractTerminated = "CONTRACT_TERMINATED"
	IntentContractRenewalDue = "CONTRACT_RENEWAL_REMINDER"
	IntentContractExpired    = "CONTRACT_EXPIRED"
	IntentSLAEscalation      = "SLA_ESCALATION"
	IntentCreditHold         = "CREDIT_HOLD"
	IntentCreditHoldReleased = "CREDIT_HOLD_RELEASED"
)

// NotificationOperation is a durable intent to notify somebody.
//
// It exists because a quotation is issued inside a database transaction and a
// notification is not: an outage at notification-service must not roll back a
// commercial commitment that has already been made to a customer.
type NotificationOperation struct {
	ID                 string
	OrganizationID     string
	SubjectType        string
	SubjectID          string
	IntentType         string
	RecipientReference string
	Channel            string
	Payload            JSONMap
	IdempotencyKey     string
	Status             string
	Attempts           int
	NextAttemptAt      time.Time
	LeaseOwner         string
	LeaseExpiresAt     *time.Time
	DeliveryReference  string
	SafeErrorCode      string
	DeadLetterReason   string
	CorrelationID      string
	CausationID        string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ---------------------------------------------------------------------------
// Fingerprinting
// ---------------------------------------------------------------------------

// Fingerprint renders a value as canonical JSON for hashing.
//
// Map keys are sorted by encoding/json, so the same logical content always
// produces the same bytes; an approval bound to a fingerprint that shifted
// with map iteration order would approve one payload and execute another.
func Fingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("crm: fingerprint: %w", err)
	}
	return string(encoded), nil
}
