// Package http exposes the Section 14 CRM routes.
//
// The response types in this file are the data-minimization boundary. Every one
// of them is built field by field from a domain model; none is a serialized
// model with fields removed afterwards. That direction matters: a model that
// grows a tax reference, an access instruction or an internal note starts
// leaking it the moment it is serialized wholesale, whereas a view that has no
// field for it cannot leak it however the model changes.
//
// Two staff-facing shapes exist for anything with PII. The standard view masks;
// the sensitive view is only ever built for a caller holding
// `crm.sensitive.read`.
package http

import (
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
)

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

// maskEmail keeps the domain and the first character of the local part.
//
// Enough to recognise an address you already know; not enough to contact
// somebody you do not.
func maskEmail(value string) string {
	if value == "" {
		return ""
	}
	at := strings.LastIndex(value, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := value[:at], value[at:]
	if len(local) <= 1 {
		return "*" + domain
	}
	return local[:1] + strings.Repeat("*", len(local)-1) + domain
}

// maskPhone keeps the last two digits.
func maskPhone(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 2 {
		return "**"
	}
	return strings.Repeat("*", len(value)-2) + value[len(value)-2:]
}

// maskReference masks a registration or tax reference to its last four
// characters.
func maskReference(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return strings.Repeat("*", len(value))
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

// MoneyView is the canonical money shape: integer minor units and an ISO 4217
// code. There is no float anywhere in a CRM response.
type MoneyView struct {
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

// CustomerView is the standard staff projection of a customer.
//
// RegistrationNumber and TaxReference are masked. They are identity documents,
// and ordinary CRM read is not a grant to see them.
type CustomerView struct {
	ID                     string `json:"id"`
	BranchID               string `json:"branchId,omitempty"`
	CustomerType           string `json:"customerType"`
	LegalName              string `json:"legalName"`
	DisplayName            string `json:"displayName"`
	RegistrationNumberMask string `json:"registrationNumberMasked,omitempty"`
	TaxReferenceMask       string `json:"taxReferenceMasked,omitempty"`
	MarketplaceCustomerID  string `json:"marketplaceCustomerId,omitempty"`
	Status                 string `json:"status"`
	RiskStatus             string `json:"riskStatus"`
	AccountOwnerUserID     string `json:"accountOwnerUserId,omitempty"`
	DefaultCurrency        string `json:"defaultCurrency"`
	PaymentTermsReference  string `json:"paymentTermsReference,omitempty"`
	CreditProfileID        string `json:"creditProfileId,omitempty"`
	Version                int64  `json:"version"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
	ArchivedAt             string `json:"archivedAt,omitempty"`
}

// SensitiveCustomerView adds the unmasked identity references.
//
// It is a distinct type rather than a flag on CustomerView so a handler that
// forgets the permission check cannot accidentally produce it: building this
// value requires deliberately calling the sensitive constructor.
type SensitiveCustomerView struct {
	CustomerView
	RegistrationNumber string `json:"registrationNumber,omitempty"`
	TaxReference       string `json:"taxReference,omitempty"`
}

// NewCustomerView builds the masked staff projection.
func NewCustomerView(customer models.Customer) CustomerView {
	return CustomerView{
		ID:                     customer.ID,
		BranchID:               customer.BranchID,
		CustomerType:           customer.CustomerType,
		LegalName:              customer.LegalName,
		DisplayName:            customer.DisplayName,
		RegistrationNumberMask: maskReference(customer.RegistrationNumber),
		TaxReferenceMask:       maskReference(customer.TaxReference),
		MarketplaceCustomerID:  customer.MarketplaceCustomerID,
		Status:                 customer.Status,
		RiskStatus:             customer.RiskStatus,
		AccountOwnerUserID:     customer.AccountOwnerUserID,
		DefaultCurrency:        customer.DefaultCurrency,
		PaymentTermsReference:  customer.PaymentTermsReference,
		CreditProfileID:        customer.CreditProfileID,
		Version:                customer.Version,
		CreatedAt:              formatTime(customer.CreatedAt),
		UpdatedAt:              formatTime(customer.UpdatedAt),
		ArchivedAt:             formatTimePtr(customer.ArchivedAt),
	}
}

// NewSensitiveCustomerView builds the unmasked projection.
func NewSensitiveCustomerView(customer models.Customer) SensitiveCustomerView {
	return SensitiveCustomerView{
		CustomerView:       NewCustomerView(customer),
		RegistrationNumber: customer.RegistrationNumber,
		TaxReference:       customer.TaxReference,
	}
}

// CustomerHistoryView is one immutable customer change.
type CustomerHistoryView struct {
	ID                 string   `json:"id"`
	ChangeType         string   `json:"changeType"`
	PreviousStatus     string   `json:"previousStatus,omitempty"`
	NewStatus          string   `json:"newStatus,omitempty"`
	PreviousRiskStatus string   `json:"previousRiskStatus,omitempty"`
	NewRiskStatus      string   `json:"newRiskStatus,omitempty"`
	ChangedFields      []string `json:"changedFields"`
	ReasonCode         string   `json:"reasonCode,omitempty"`
	ApprovalRequestID  string   `json:"approvalRequestId,omitempty"`
	ActorType          string   `json:"actorType"`
	ActorID            string   `json:"actorId"`
	OccurredAt         string   `json:"occurredAt"`
}

// NewCustomerHistoryView builds the history projection.
func NewCustomerHistoryView(entry models.CustomerHistory) CustomerHistoryView {
	fields := entry.ChangedFields
	if fields == nil {
		fields = []string{}
	}
	return CustomerHistoryView{
		ID: entry.ID, ChangeType: entry.ChangeType, PreviousStatus: entry.PreviousStatus,
		NewStatus: entry.NewStatus, PreviousRiskStatus: entry.PreviousRiskStatus,
		NewRiskStatus: entry.NewRiskStatus, ChangedFields: fields,
		ReasonCode: entry.ReasonCode, ApprovalRequestID: entry.ApprovalRequestID,
		ActorType: entry.ActorType, ActorID: entry.ActorID,
		OccurredAt: formatTime(entry.OccurredAt),
	}
}

// ---------------------------------------------------------------------------
// Contacts
// ---------------------------------------------------------------------------

// ContactView is the standard staff projection of a contact, with the email and
// phone masked.
type ContactView struct {
	ID                      string         `json:"id"`
	CustomerID              string         `json:"customerId"`
	FullName                string         `json:"fullName"`
	RoleTitle               string         `json:"roleTitle,omitempty"`
	EmailMasked             string         `json:"emailMasked,omitempty"`
	PhoneMasked             string         `json:"phoneMasked,omitempty"`
	PreferredChannel        string         `json:"preferredChannel"`
	ChannelDeliverable      bool           `json:"channelDeliverable"`
	ContactPurpose          string         `json:"contactPurpose"`
	IsPrimary               bool           `json:"isPrimary"`
	NotificationPreferences map[string]any `json:"notificationPreferences,omitempty"`
	ConsentStatus           string         `json:"consentStatus"`
	ConsentRecordedAt       string         `json:"consentRecordedAt,omitempty"`
	Status                  string         `json:"status"`
	Version                 int64          `json:"version"`
	CreatedAt               string         `json:"createdAt"`
	UpdatedAt               string         `json:"updatedAt"`
	ArchivedAt              string         `json:"archivedAt,omitempty"`
}

// SensitiveContactView adds the unmasked contact details and the consent
// source.
type SensitiveContactView struct {
	ContactView
	Email         string `json:"email,omitempty"`
	Phone         string `json:"phone,omitempty"`
	ConsentSource string `json:"consentSource,omitempty"`
}

// NewContactView builds the masked staff projection.
//
// ChannelDeliverable is reported rather than assumed: a contact who prefers
// WhatsApp is recorded as preferring it and shown as not deliverable, because
// claiming a delivery route the platform does not have is worse than saying so.
func NewContactView(contact models.Contact) ContactView {
	return ContactView{
		ID: contact.ID, CustomerID: contact.CustomerID, FullName: contact.FullName,
		RoleTitle:               contact.RoleTitle,
		EmailMasked:             maskEmail(contact.NormalizedEmail),
		PhoneMasked:             maskPhone(contact.NormalizedPhone),
		PreferredChannel:        contact.PreferredChannel,
		ChannelDeliverable:      models.ChannelDeliverable(contact.PreferredChannel),
		ContactPurpose:          contact.ContactPurpose,
		IsPrimary:               contact.IsPrimary,
		NotificationPreferences: contact.NotificationPreferences,
		ConsentStatus:           contact.ConsentStatus,
		ConsentRecordedAt:       formatTimePtr(contact.ConsentRecordedAt),
		Status:                  contact.Status,
		Version:                 contact.Version,
		CreatedAt:               formatTime(contact.CreatedAt),
		UpdatedAt:               formatTime(contact.UpdatedAt),
		ArchivedAt:              formatTimePtr(contact.ArchivedAt),
	}
}

// NewSensitiveContactView builds the unmasked projection.
func NewSensitiveContactView(contact models.Contact) SensitiveContactView {
	return SensitiveContactView{
		ContactView:   NewContactView(contact),
		Email:         contact.NormalizedEmail,
		Phone:         contact.NormalizedPhone,
		ConsentSource: contact.ConsentSource,
	}
}

// ---------------------------------------------------------------------------
// Locations
// ---------------------------------------------------------------------------

// LocationView is the standard staff projection of a customer site.
//
// The exact coordinates, the access instructions and the safety instructions
// are absent — not masked, absent. There is no field for them on this type.
// HasCoordinates tells a caller whether asking with the sensitive permission
// would return anything.
type LocationView struct {
	ID                string         `json:"id"`
	CustomerID        string         `json:"customerId"`
	LocationType      string         `json:"locationType"`
	Label             string         `json:"label"`
	City              string         `json:"city"`
	StateOrRegion     string         `json:"stateOrRegion"`
	CountryCode       string         `json:"countryCode"`
	Timezone          string         `json:"timezone"`
	OperatingHours    map[string]any `json:"operatingHours,omitempty"`
	ContactReference  string         `json:"contactReference,omitempty"`
	GeofenceReference string         `json:"geofenceReference,omitempty"`
	HasCoordinates    bool           `json:"hasCoordinates"`
	HasAccessNotes    bool           `json:"hasAccessInstructions"`
	HasSafetyNotes    bool           `json:"hasSafetyInstructions"`
	Status            string         `json:"status"`
	Version           int64          `json:"version"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
	ArchivedAt        string         `json:"archivedAt,omitempty"`
}

// SensitiveLocationView adds the street address, exact coordinates and the
// operational instructions.
type SensitiveLocationView struct {
	LocationView
	AddressLine1       string   `json:"addressLine1"`
	AddressLine2       string   `json:"addressLine2,omitempty"`
	PostalCode         string   `json:"postalCode,omitempty"`
	Latitude           *float64 `json:"latitude,omitempty"`
	Longitude          *float64 `json:"longitude,omitempty"`
	AccessInstructions string   `json:"accessInstructions,omitempty"`
	SafetyInstructions string   `json:"safetyInstructions,omitempty"`
}

// NewLocationView builds the masked staff projection.
func NewLocationView(location models.Location) LocationView {
	return LocationView{
		ID: location.ID, CustomerID: location.CustomerID,
		LocationType: location.LocationType, Label: location.Label,
		City: location.Address.City, StateOrRegion: location.Address.StateOrRegion,
		CountryCode: location.Address.CountryCode, Timezone: location.Timezone,
		OperatingHours: location.OperatingHours, ContactReference: location.ContactReference,
		GeofenceReference: location.GeofenceReference,
		HasCoordinates:    location.Latitude != nil,
		HasAccessNotes:    location.AccessInstructions != "",
		HasSafetyNotes:    location.SafetyInstructions != "",
		Status:            location.Status, Version: location.Version,
		CreatedAt: formatTime(location.CreatedAt), UpdatedAt: formatTime(location.UpdatedAt),
		ArchivedAt: formatTimePtr(location.ArchivedAt),
	}
}

// NewSensitiveLocationView builds the full projection.
func NewSensitiveLocationView(location models.Location) SensitiveLocationView {
	return SensitiveLocationView{
		LocationView:       NewLocationView(location),
		AddressLine1:       location.Address.Line1,
		AddressLine2:       location.Address.Line2,
		PostalCode:         location.Address.PostalCode,
		Latitude:           location.Latitude,
		Longitude:          location.Longitude,
		AccessInstructions: location.AccessInstructions,
		SafetyInstructions: location.SafetyInstructions,
	}
}

// ---------------------------------------------------------------------------
// Opportunities
// ---------------------------------------------------------------------------

// OpportunityView is the staff projection of a sales pursuit.
type OpportunityView struct {
	ID                     string     `json:"id"`
	BranchID               string     `json:"branchId,omitempty"`
	CustomerID             string     `json:"customerId"`
	Title                  string     `json:"title"`
	Description            string     `json:"description,omitempty"`
	Stage                  string     `json:"stage"`
	ExpectedValue          *MoneyView `json:"expectedValue,omitempty"`
	ProbabilityBasisPoints int        `json:"probabilityBasisPoints"`
	ExpectedCloseDate      string     `json:"expectedCloseDate,omitempty"`
	ServiceCapabilities    []string   `json:"serviceCapabilities"`
	AccountOwnerUserID     string     `json:"accountOwnerUserId,omitempty"`
	NextAction             string     `json:"nextAction,omitempty"`
	NextActionAt           string     `json:"nextActionAt,omitempty"`
	LossReason             string     `json:"lossReason,omitempty"`
	HoldReason             string     `json:"holdReason,omitempty"`
	Version                int64      `json:"version"`
	CreatedAt              string     `json:"createdAt"`
	UpdatedAt              string     `json:"updatedAt"`
	ClosedAt               string     `json:"closedAt,omitempty"`
}

// NewOpportunityView builds the staff projection.
func NewOpportunityView(opportunity models.Opportunity) OpportunityView {
	capabilities := opportunity.ServiceCapabilities
	if capabilities == nil {
		capabilities = []string{}
	}
	view := OpportunityView{
		ID: opportunity.ID, BranchID: opportunity.BranchID,
		CustomerID: opportunity.CustomerID, Title: opportunity.Title,
		Description: opportunity.Description, Stage: opportunity.Stage,
		ProbabilityBasisPoints: opportunity.ProbabilityBasisPoints,
		ServiceCapabilities:    capabilities,
		AccountOwnerUserID:     opportunity.AccountOwnerUserID,
		NextAction:             opportunity.NextAction,
		NextActionAt:           formatTimePtr(opportunity.NextActionAt),
		LossReason:             opportunity.LossReason, HoldReason: opportunity.HoldReason,
		Version: opportunity.Version, CreatedAt: formatTime(opportunity.CreatedAt),
		UpdatedAt: formatTime(opportunity.UpdatedAt),
		ClosedAt:  formatTimePtr(opportunity.ClosedAt),
	}
	if opportunity.ExpectedCloseDate != nil {
		view.ExpectedCloseDate = opportunity.ExpectedCloseDate.UTC().Format("2006-01-02")
	}
	if opportunity.ExpectedValueAmountMinor != nil {
		view.ExpectedValue = &MoneyView{
			AmountMinor: *opportunity.ExpectedValueAmountMinor,
			Currency:    opportunity.ExpectedValueCurrency,
		}
	}
	return view
}

// OpportunityHistoryView is one immutable stage change.
type OpportunityHistoryView struct {
	ID            string `json:"id"`
	PreviousStage string `json:"previousStage"`
	NewStage      string `json:"newStage"`
	ReasonCode    string `json:"reasonCode,omitempty"`
	ActorType     string `json:"actorType"`
	ActorID       string `json:"actorId"`
	OccurredAt    string `json:"occurredAt"`
}

// NewOpportunityHistoryView builds the history projection.
func NewOpportunityHistoryView(entry models.OpportunityHistory) OpportunityHistoryView {
	return OpportunityHistoryView{
		ID: entry.ID, PreviousStage: entry.PreviousStage, NewStage: entry.NewStage,
		ReasonCode: entry.ReasonCode, ActorType: entry.ActorType, ActorID: entry.ActorID,
		OccurredAt: formatTime(entry.OccurredAt),
	}
}

// ---------------------------------------------------------------------------
// Quotations
// ---------------------------------------------------------------------------

// LineItemView is one manually priced quotation line.
type LineItemView struct {
	ID                       string `json:"id"`
	Description              string `json:"description"`
	Quantity                 string `json:"quantity"`
	UnitType                 string `json:"unitType,omitempty"`
	UnitAmountMinor          int64  `json:"unitAmountMinor"`
	LineAmountMinor          int64  `json:"lineAmountMinor"`
	Currency                 string `json:"currency"`
	ExternalPricingReference string `json:"externalPricingReference,omitempty"`
	ManualReason             string `json:"manualReason,omitempty"`
	Sequence                 int    `json:"sequence"`
}

// NewLineItemView builds the line projection.
func NewLineItemView(item models.LineItem) LineItemView {
	return LineItemView{
		ID: item.ID, Description: item.Description, Quantity: item.Quantity,
		UnitType: item.UnitType, UnitAmountMinor: item.UnitAmountMinor,
		LineAmountMinor: item.LineAmountMinor, Currency: item.Currency,
		ExternalPricingReference: item.ExternalPricingReference,
		ManualReason:             item.ManualReason, Sequence: item.Sequence,
	}
}

// QuotationView is the standard staff projection of a quotation.
//
// InternalNotes is absent. It is added only by the sensitive view, because
// somebody who may read a quotation is not thereby entitled to the sales team's
// private commentary on the customer.
type QuotationView struct {
	ID                string         `json:"id"`
	BranchID          string         `json:"branchId,omitempty"`
	CustomerID        string         `json:"customerId"`
	OpportunityID     string         `json:"opportunityId,omitempty"`
	QuotationNumber   string         `json:"quotationNumber"`
	Status            string         `json:"status"`
	Total             MoneyView      `json:"total"`
	ValidFrom         string         `json:"validFrom"`
	ValidUntil        string         `json:"validUntil"`
	ServiceScope      map[string]any `json:"serviceScope,omitempty"`
	Terms             map[string]any `json:"terms,omitempty"`
	CurrentVersion    int            `json:"currentVersion"`
	IssuedVersionID   string         `json:"issuedVersionId,omitempty"`
	AcceptedVersionID string         `json:"acceptedVersionId,omitempty"`
	ApprovalRequestID string         `json:"approvalRequestId,omitempty"`
	SupersededByID    string         `json:"supersededById,omitempty"`
	LineItems         []LineItemView `json:"lineItems,omitempty"`
	Version           int64          `json:"version"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
	IssuedAt          string         `json:"issuedAt,omitempty"`
	ResolvedAt        string         `json:"resolvedAt,omitempty"`
}

// SensitiveQuotationView adds the internal notes.
type SensitiveQuotationView struct {
	QuotationView
	InternalNotes string `json:"internalNotes,omitempty"`
}

// NewQuotationView builds the standard staff projection.
func NewQuotationView(quotation models.Quotation, items []models.LineItem) QuotationView {
	lines := make([]LineItemView, 0, len(items))
	for _, item := range items {
		lines = append(lines, NewLineItemView(item))
	}
	return QuotationView{
		ID: quotation.ID, BranchID: quotation.BranchID, CustomerID: quotation.CustomerID,
		OpportunityID: quotation.OpportunityID, QuotationNumber: quotation.QuotationNumber,
		Status: quotation.Status,
		Total: MoneyView{AmountMinor: quotation.TotalAmountMinor,
			Currency: quotation.Currency},
		ValidFrom: formatTime(quotation.ValidFrom), ValidUntil: formatTime(quotation.ValidUntil),
		ServiceScope: quotation.ServiceScope, Terms: quotation.Terms,
		CurrentVersion: quotation.CurrentVersion, IssuedVersionID: quotation.IssuedVersionID,
		AcceptedVersionID: quotation.AcceptedVersionID,
		ApprovalRequestID: quotation.ApprovalRequestID,
		SupersededByID:    quotation.SupersededByID, LineItems: lines,
		Version: quotation.Version, CreatedAt: formatTime(quotation.CreatedAt),
		UpdatedAt:  formatTime(quotation.UpdatedAt),
		IssuedAt:   formatTimePtr(quotation.IssuedAt),
		ResolvedAt: formatTimePtr(quotation.ResolvedAt),
	}
}

// NewSensitiveQuotationView adds the internal notes.
func NewSensitiveQuotationView(quotation models.Quotation,
	items []models.LineItem) SensitiveQuotationView {
	return SensitiveQuotationView{
		QuotationView: NewQuotationView(quotation, items),
		InternalNotes: quotation.InternalNotes,
	}
}

// QuotationVersionView is one immutable issued version.
type QuotationVersionView struct {
	ID                 string         `json:"id"`
	QuotationID        string         `json:"quotationId"`
	VersionNumber      int            `json:"versionNumber"`
	CustomerSnapshot   map[string]any `json:"customerSnapshot"`
	ContactSnapshot    map[string]any `json:"contactSnapshot,omitempty"`
	Total              MoneyView      `json:"total"`
	LineItems          []LineItemView `json:"lineItems"`
	ServiceScope       map[string]any `json:"serviceScope,omitempty"`
	Terms              map[string]any `json:"terms,omitempty"`
	ValidFrom          string         `json:"validFrom"`
	ValidUntil         string         `json:"validUntil"`
	DocumentReferences []string       `json:"documentReferences"`
	ApprovalRequestID  string         `json:"approvalRequestId,omitempty"`
	IssuedBy           string         `json:"issuedBy"`
	IssuedAt           string         `json:"issuedAt"`
	ContentFingerprint string         `json:"contentFingerprint"`
}

// NewQuotationVersionView builds the version projection.
func NewQuotationVersionView(version models.QuotationVersion) QuotationVersionView {
	lines := make([]LineItemView, 0, len(version.LineItemsSnapshot))
	for _, item := range version.LineItemsSnapshot {
		lines = append(lines, NewLineItemView(item))
	}
	references := version.DocumentReferences
	if references == nil {
		references = []string{}
	}
	return QuotationVersionView{
		ID: version.ID, QuotationID: version.QuotationID,
		VersionNumber: version.VersionNumber, CustomerSnapshot: version.CustomerSnapshot,
		ContactSnapshot: version.ContactSnapshot,
		Total:           MoneyView{AmountMinor: version.TotalAmountMinor, Currency: version.Currency},
		LineItems:       lines, ServiceScope: version.ServiceScopeSnapshot,
		Terms: version.TermsSnapshot, ValidFrom: formatTime(version.ValidFrom),
		ValidUntil: formatTime(version.ValidUntil), DocumentReferences: references,
		ApprovalRequestID: version.ApprovalRequestID, IssuedBy: version.IssuedBy,
		IssuedAt: formatTime(version.IssuedAt), ContentFingerprint: version.ContentFingerprint,
	}
}

// QuotationHistoryView is one immutable quotation status change.
type QuotationHistoryView struct {
	ID                string `json:"id"`
	PreviousStatus    string `json:"previousStatus,omitempty"`
	NewStatus         string `json:"newStatus"`
	QuotationVersion  int    `json:"quotationVersion"`
	ReasonCode        string `json:"reasonCode,omitempty"`
	ApprovalRequestID string `json:"approvalRequestId,omitempty"`
	ActorType         string `json:"actorType"`
	ActorID           string `json:"actorId,omitempty"`
	OccurredAt        string `json:"occurredAt"`
}

// NewQuotationHistoryView builds the history projection.
func NewQuotationHistoryView(entry models.QuotationHistory) QuotationHistoryView {
	return QuotationHistoryView{
		ID: entry.ID, PreviousStatus: entry.PreviousStatus, NewStatus: entry.NewStatus,
		QuotationVersion: entry.QuotationVersion, ReasonCode: entry.ReasonCode,
		ApprovalRequestID: entry.ApprovalRequestID, ActorType: entry.ActorType,
		ActorID: entry.ActorID, OccurredAt: formatTime(entry.OccurredAt),
	}
}

// AcceptanceView is the immutable record that a customer took an offer.
//
// The IP and user-agent digests are absent. They exist on the row as evidence
// of provenance; they are not something an API returns, because a hash of an IP
// address is still a claim about where somebody was.
type AcceptanceView struct {
	ID                        string `json:"id"`
	QuotationID               string `json:"quotationId"`
	AcceptedVersionID         string `json:"acceptedVersionId"`
	ActorType                 string `json:"actorType"`
	ActorReference            string `json:"actorReference,omitempty"`
	AcceptanceMethod          string `json:"acceptanceMethod"`
	EvidenceDocumentReference string `json:"evidenceDocumentReference,omitempty"`
	AcceptedAt                string `json:"acceptedAt"`
}

// NewAcceptanceView builds the acceptance projection.
func NewAcceptanceView(acceptance models.Acceptance) AcceptanceView {
	return AcceptanceView{
		ID: acceptance.ID, QuotationID: acceptance.QuotationID,
		AcceptedVersionID: acceptance.AcceptedVersionID, ActorType: acceptance.ActorType,
		ActorReference:            acceptance.ActorReference,
		AcceptanceMethod:          acceptance.AcceptanceMethod,
		EvidenceDocumentReference: acceptance.EvidenceDocumentReference,
		AcceptedAt:                formatTime(acceptance.AcceptedAt),
	}
}

// ConversionIntentView is the durable record that accepted work may later
// become a private order.
//
// It carries no order ID, no shipment ID, no assignment ID and no trip ID.
// Status is always PENDING_SECTION_15: nothing has been ordered, and a
// consumer must not be able to read this as though something had.
type ConversionIntentView struct {
	ID                string `json:"id"`
	QuotationID       string `json:"quotationId"`
	AcceptedVersionID string `json:"acceptedVersionId"`
	AcceptanceID      string `json:"acceptanceId"`
	ConversionType    string `json:"conversionType"`
	Status            string `json:"status"`
	CreatedAt         string `json:"createdAt"`
}

// NewConversionIntentView builds the intent projection.
func NewConversionIntentView(intent models.ConversionIntent) ConversionIntentView {
	return ConversionIntentView{
		ID: intent.ID, QuotationID: intent.QuotationID,
		AcceptedVersionID: intent.AcceptedVersionID, AcceptanceID: intent.AcceptanceID,
		ConversionType: intent.ConversionType, Status: intent.Status,
		CreatedAt: formatTime(intent.CreatedAt),
	}
}

// ---------------------------------------------------------------------------
// Documents
// ---------------------------------------------------------------------------

// DocumentView is Fleet's projection of a Media-owned object.
//
// It carries the opaque object ID and a readiness projection and nothing else.
// There is no URL field, no path field, no checksum field and no MIME field —
// those belong to media-file-service, and a second copy here would be a second
// answer that can go stale exactly when a document's safety matters.
type DocumentView struct {
	ID               string `json:"id"`
	MediaObjectID    string `json:"mediaObjectId"`
	DocumentPurpose  string `json:"documentPurpose"`
	IsMandatory      bool   `json:"isMandatory"`
	StatusProjection string `json:"statusProjection"`
	ReadinessState   string `json:"readinessState"`
	Usable           bool   `json:"usable"`
	VersionID        string `json:"versionId,omitempty"`
	SourceService    string `json:"sourceService"`
	SourceVersion    int64  `json:"sourceVersion"`
	SyncedAt         string `json:"syncedAt,omitempty"`
	CreatedAt        string `json:"createdAt"`
}

// NewDocumentView builds the document projection.
func NewDocumentView(document models.DocumentReference) DocumentView {
	return DocumentView{
		ID: document.ID, MediaObjectID: document.MediaObjectID,
		DocumentPurpose: document.DocumentPurpose, IsMandatory: document.IsMandatory,
		StatusProjection: document.StatusProjection, ReadinessState: document.ReadinessState,
		Usable: document.Usable(), VersionID: document.SubjectVersionID,
		SourceService: document.SourceService, SourceVersion: document.SourceVersion,
		SyncedAt: formatTimePtr(document.SyncedAt), CreatedAt: formatTime(document.CreatedAt),
	}
}

// ---------------------------------------------------------------------------
// Contracts
// ---------------------------------------------------------------------------

// ContractView is the standard staff projection of a contract.
//
// The commercial snapshot is absent: it is the negotiated substance of the
// agreement and the most sensitive thing in the aggregate. The sensitive view
// carries it.
type ContractView struct {
	ID                string         `json:"id"`
	BranchID          string         `json:"branchId,omitempty"`
	CustomerID        string         `json:"customerId"`
	QuotationID       string         `json:"quotationId,omitempty"`
	ContractNumber    string         `json:"contractNumber"`
	Status            string         `json:"status"`
	EffectiveFrom     string         `json:"effectiveFrom"`
	EffectiveTo       string         `json:"effectiveTo,omitempty"`
	ServiceScope      map[string]any `json:"serviceScope,omitempty"`
	RenewalTerms      map[string]any `json:"renewalTerms,omitempty"`
	TerminationTerms  map[string]any `json:"terminationTerms,omitempty"`
	CurrentVersion    int            `json:"currentVersion"`
	ActiveVersionID   string         `json:"activeVersionId,omitempty"`
	ApprovalRequestID string         `json:"approvalRequestId,omitempty"`
	SupersedesID      string         `json:"supersedesId,omitempty"`
	Version           int64          `json:"version"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
	ActivatedAt       string         `json:"activatedAt,omitempty"`
	TerminatedAt      string         `json:"terminatedAt,omitempty"`
}

// SensitiveContractView adds the commercial snapshot.
type SensitiveContractView struct {
	ContractView
	CommercialSnapshot map[string]any `json:"commercialSnapshot,omitempty"`
}

// NewContractView builds the standard staff projection.
func NewContractView(contract models.Contract) ContractView {
	return ContractView{
		ID: contract.ID, BranchID: contract.BranchID, CustomerID: contract.CustomerID,
		QuotationID: contract.QuotationID, ContractNumber: contract.ContractNumber,
		Status: contract.Status, EffectiveFrom: formatTime(contract.EffectiveFrom),
		EffectiveTo: formatTimePtr(contract.EffectiveTo), ServiceScope: contract.ServiceScope,
		RenewalTerms: contract.RenewalTerms, TerminationTerms: contract.TerminationTerms,
		CurrentVersion: contract.CurrentVersion, ActiveVersionID: contract.ActiveVersionID,
		ApprovalRequestID: contract.ApprovalRequestID, SupersedesID: contract.SupersedesID,
		Version: contract.Version, CreatedAt: formatTime(contract.CreatedAt),
		UpdatedAt:    formatTime(contract.UpdatedAt),
		ActivatedAt:  formatTimePtr(contract.ActivatedAt),
		TerminatedAt: formatTimePtr(contract.TerminatedAt),
	}
}

// NewSensitiveContractView adds the commercial snapshot.
func NewSensitiveContractView(contract models.Contract) SensitiveContractView {
	return SensitiveContractView{
		ContractView:       NewContractView(contract),
		CommercialSnapshot: contract.CommercialSnapshot,
	}
}

// ContractVersionView is one immutable activated version.
type ContractVersionView struct {
	ID                  string         `json:"id"`
	ContractID          string         `json:"contractId"`
	VersionNumber       int            `json:"versionNumber"`
	CustomerSnapshot    map[string]any `json:"customerSnapshot"`
	ServiceScope        map[string]any `json:"serviceScope,omitempty"`
	CommercialSnapshot  map[string]any `json:"commercialSnapshot,omitempty"`
	EffectiveFrom       string         `json:"effectiveFrom"`
	EffectiveTo         string         `json:"effectiveTo,omitempty"`
	RenewalTerms        map[string]any `json:"renewalTerms,omitempty"`
	TerminationTerms    map[string]any `json:"terminationTerms,omitempty"`
	DocumentReferences  []string       `json:"documentReferences"`
	SignatoryReferences []string       `json:"signatoryReferences"`
	ApprovalRequestID   string         `json:"approvalRequestId,omitempty"`
	ContentFingerprint  string         `json:"contentFingerprint"`
	ActivatedBy         string         `json:"activatedBy"`
	ActivatedAt         string         `json:"activatedAt"`
}

// NewContractVersionView builds the version projection.
func NewContractVersionView(version models.ContractVersion) ContractVersionView {
	documents := version.DocumentReferences
	if documents == nil {
		documents = []string{}
	}
	signatories := version.SignatoryReferences
	if signatories == nil {
		signatories = []string{}
	}
	return ContractVersionView{
		ID: version.ID, ContractID: version.ContractID,
		VersionNumber: version.VersionNumber, CustomerSnapshot: version.CustomerSnapshot,
		ServiceScope:       version.ServiceScopeSnapshot,
		CommercialSnapshot: version.CommercialSnapshot,
		EffectiveFrom:      formatTime(version.EffectiveFrom),
		EffectiveTo:        formatTimePtr(version.EffectiveTo),
		RenewalTerms:       version.RenewalTerms, TerminationTerms: version.TerminationTerms,
		DocumentReferences: documents, SignatoryReferences: signatories,
		ApprovalRequestID:  version.ApprovalRequestID,
		ContentFingerprint: version.ContentFingerprint, ActivatedBy: version.ActivatedBy,
		ActivatedAt: formatTime(version.ActivatedAt),
	}
}

// ContractHistoryView is one immutable contract status change.
type ContractHistoryView struct {
	ID             string `json:"id"`
	PreviousStatus string `json:"previousStatus,omitempty"`
	NewStatus      string `json:"newStatus"`
	ReasonCode     string `json:"reasonCode,omitempty"`
	ActorType      string `json:"actorType"`
	ActorID        string `json:"actorId"`
	OccurredAt     string `json:"occurredAt"`
}

// NewContractHistoryView builds the history projection.
func NewContractHistoryView(entry models.ContractHistory) ContractHistoryView {
	return ContractHistoryView{
		ID: entry.ID, PreviousStatus: entry.PreviousStatus, NewStatus: entry.NewStatus,
		ReasonCode: entry.ReasonCode, ActorType: entry.ActorType, ActorID: entry.ActorID,
		OccurredAt: formatTime(entry.OccurredAt),
	}
}

// ---------------------------------------------------------------------------
// Service level agreements
// ---------------------------------------------------------------------------

// SLAView is the staff projection of a service commitment.
type SLAView struct {
	ID                         string         `json:"id"`
	CustomerID                 string         `json:"customerId"`
	ContractID                 string         `json:"contractId,omitempty"`
	FutureServiceCatalogItemID string         `json:"futureServiceCatalogItemId,omitempty"`
	MetricCode                 string         `json:"metricCode"`
	TargetValue                string         `json:"targetValue"`
	MeasurementUnit            string         `json:"measurementUnit"`
	MeasurementWindow          string         `json:"measurementWindow"`
	EscalationPolicy           map[string]any `json:"escalationPolicy,omitempty"`
	EffectiveFrom              string         `json:"effectiveFrom"`
	EffectiveTo                string         `json:"effectiveTo,omitempty"`
	Status                     string         `json:"status"`
	Version                    int64          `json:"version"`
	CreatedAt                  string         `json:"createdAt"`
	UpdatedAt                  string         `json:"updatedAt"`
}

// NewSLAView builds the SLA projection.
func NewSLAView(sla models.SLA) SLAView {
	return SLAView{
		ID: sla.ID, CustomerID: sla.CustomerID, ContractID: sla.ContractID,
		FutureServiceCatalogItemID: sla.FutureServiceCatalogItemID,
		MetricCode:                 sla.MetricCode, TargetValue: sla.TargetValue,
		MeasurementUnit: sla.MeasurementUnit, MeasurementWindow: sla.MeasurementWindow,
		EscalationPolicy: sla.EscalationPolicy,
		EffectiveFrom:    formatTime(sla.EffectiveFrom),
		EffectiveTo:      formatTimePtr(sla.EffectiveTo), Status: sla.Status,
		Version: sla.Version, CreatedAt: formatTime(sla.CreatedAt),
		UpdatedAt: formatTime(sla.UpdatedAt),
	}
}

// ObservationView is one append-only measurement.
//
// The outcome is reported exactly as stored. UNKNOWN and INSUFFICIENT_DATA
// travel as themselves; nothing in this layer normalizes them into MET.
type ObservationView struct {
	ID              string  `json:"id"`
	SLAID           string  `json:"slaId"`
	SourceType      string  `json:"sourceType"`
	SourceReference string  `json:"sourceReference,omitempty"`
	MeasuredValue   *string `json:"measuredValue,omitempty"`
	MeasurementUnit string  `json:"measurementUnit,omitempty"`
	MeasuredAt      string  `json:"measuredAt"`
	WindowStart     string  `json:"windowStart"`
	WindowEnd       string  `json:"windowEnd"`
	Outcome         string  `json:"outcome"`
	SourceVersion   int64   `json:"sourceVersion"`
	CreatedAt       string  `json:"createdAt"`
}

// NewObservationView builds the observation projection.
func NewObservationView(observation models.SLAObservation) ObservationView {
	return ObservationView{
		ID: observation.ID, SLAID: observation.SLAID, SourceType: observation.SourceType,
		SourceReference: observation.SourceReference,
		MeasuredValue:   observation.MeasuredValue,
		MeasurementUnit: observation.MeasurementUnit,
		MeasuredAt:      formatTime(observation.MeasuredAt),
		WindowStart:     formatTime(observation.WindowStart),
		WindowEnd:       formatTime(observation.WindowEnd), Outcome: observation.Outcome,
		SourceVersion: observation.SourceVersion,
		CreatedAt:     formatTime(observation.CreatedAt),
	}
}

// SLAHistoryView is one immutable SLA status change.
type SLAHistoryView struct {
	ID             string `json:"id"`
	PreviousStatus string `json:"previousStatus,omitempty"`
	NewStatus      string `json:"newStatus"`
	ReasonCode     string `json:"reasonCode,omitempty"`
	ActorType      string `json:"actorType"`
	ActorID        string `json:"actorId"`
	OccurredAt     string `json:"occurredAt"`
}

// NewSLAHistoryView builds the history projection.
func NewSLAHistoryView(entry models.SLAHistory) SLAHistoryView {
	return SLAHistoryView{
		ID: entry.ID, PreviousStatus: entry.PreviousStatus, NewStatus: entry.NewStatus,
		ReasonCode: entry.ReasonCode, ActorType: entry.ActorType, ActorID: entry.ActorID,
		OccurredAt: formatTime(entry.OccurredAt),
	}
}

// ---------------------------------------------------------------------------
// Credit policy
// ---------------------------------------------------------------------------

// CreditProfileView is the standard staff projection of a credit policy.
//
// The credit limit itself is absent. Whether a customer is on hold is
// operationally relevant to anyone who may read CRM; how much credit the
// organization extends them is a finance figure behind `crm.credit.read`.
type CreditProfileView struct {
	ID                 string `json:"id"`
	CustomerID         string `json:"customerId"`
	Currency           string `json:"currency"`
	PaymentTermsDays   int    `json:"paymentTermsDays"`
	HoldStatus         string `json:"holdStatus"`
	Blocking           bool   `json:"blocking"`
	HasCreditLimit     bool   `json:"hasCreditLimit"`
	RiskStatusSnapshot string `json:"riskStatusSnapshot"`
	Version            int64  `json:"version"`
	EffectiveFrom      string `json:"effectiveFrom"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

// SensitiveCreditProfileView adds the limit, the hold reason and the approval
// reference.
type SensitiveCreditProfileView struct {
	CreditProfileView
	CreditLimit       *MoneyView `json:"creditLimit,omitempty"`
	HoldReason        string     `json:"holdReason,omitempty"`
	ApprovedBy        string     `json:"approvedBy,omitempty"`
	ApprovalRequestID string     `json:"approvalRequestId,omitempty"`
}

// NewCreditProfileView builds the masked staff projection.
func NewCreditProfileView(profile models.CreditProfile) CreditProfileView {
	return CreditProfileView{
		ID: profile.ID, CustomerID: profile.CustomerID, Currency: profile.Currency,
		PaymentTermsDays: profile.PaymentTermsDays, HoldStatus: profile.HoldStatus,
		Blocking:           models.BlockingHold(profile.HoldStatus),
		HasCreditLimit:     profile.CreditLimitAmountMinor != nil,
		RiskStatusSnapshot: profile.RiskStatusSnapshot, Version: profile.Version,
		EffectiveFrom: formatTime(profile.EffectiveFrom),
		CreatedAt:     formatTime(profile.CreatedAt), UpdatedAt: formatTime(profile.UpdatedAt),
	}
}

// NewSensitiveCreditProfileView builds the full finance projection.
func NewSensitiveCreditProfileView(profile models.CreditProfile) SensitiveCreditProfileView {
	view := SensitiveCreditProfileView{
		CreditProfileView: NewCreditProfileView(profile),
		HoldReason:        profile.HoldReason,
		ApprovedBy:        profile.ApprovedBy,
		ApprovalRequestID: profile.ApprovalRequestID,
	}
	if profile.CreditLimitAmountMinor != nil {
		view.CreditLimit = &MoneyView{AmountMinor: *profile.CreditLimitAmountMinor,
			Currency: profile.Currency}
	}
	return view
}

// CreditHistoryView is one immutable credit policy change.
type CreditHistoryView struct {
	ID                 string     `json:"id"`
	PreviousHoldStatus string     `json:"previousHoldStatus,omitempty"`
	NewHoldStatus      string     `json:"newHoldStatus"`
	PreviousLimit      *MoneyView `json:"previousLimit,omitempty"`
	NewLimit           *MoneyView `json:"newLimit,omitempty"`
	ReasonCode         string     `json:"reasonCode,omitempty"`
	ApprovalRequestID  string     `json:"approvalRequestId,omitempty"`
	ActorType          string     `json:"actorType"`
	ActorID            string     `json:"actorId"`
	OccurredAt         string     `json:"occurredAt"`
}

// NewCreditHistoryView builds the history projection.
func NewCreditHistoryView(entry models.CreditHistory, currency string) CreditHistoryView {
	view := CreditHistoryView{
		ID: entry.ID, PreviousHoldStatus: entry.PreviousHoldStatus,
		NewHoldStatus: entry.NewHoldStatus, ReasonCode: entry.ReasonCode,
		ApprovalRequestID: entry.ApprovalRequestID, ActorType: entry.ActorType,
		ActorID: entry.ActorID, OccurredAt: formatTime(entry.OccurredAt),
	}
	if entry.PreviousLimitMinor != nil {
		view.PreviousLimit = &MoneyView{AmountMinor: *entry.PreviousLimitMinor,
			Currency: currency}
	}
	if entry.NewLimitMinor != nil {
		view.NewLimit = &MoneyView{AmountMinor: *entry.NewLimitMinor, Currency: currency}
	}
	return view
}

// NotificationOperationView is one durable CRM notification intent.
//
// The recipient reference is an opaque contact ID. The payload is omitted: it
// is built for delivery, not for display, and returning it here would be a
// second route to whatever it carries.
type NotificationOperationView struct {
	ID                 string `json:"id"`
	SubjectType        string `json:"subjectType"`
	SubjectID          string `json:"subjectId"`
	IntentType         string `json:"intentType"`
	RecipientReference string `json:"recipientReference,omitempty"`
	Channel            string `json:"channel"`
	Status             string `json:"status"`
	Attempts           int    `json:"attempts"`
	NextAttemptAt      string `json:"nextAttemptAt"`
	SafeErrorCode      string `json:"safeErrorCode,omitempty"`
	DeadLetterReason   string `json:"deadLetterReason,omitempty"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

// NewNotificationOperationView builds the operation projection.
func NewNotificationOperationView(operation models.NotificationOperation) NotificationOperationView {
	return NotificationOperationView{
		ID: operation.ID, SubjectType: operation.SubjectType, SubjectID: operation.SubjectID,
		IntentType: operation.IntentType, RecipientReference: operation.RecipientReference,
		Channel: operation.Channel, Status: operation.Status, Attempts: operation.Attempts,
		NextAttemptAt:    formatTime(operation.NextAttemptAt),
		SafeErrorCode:    operation.SafeErrorCode,
		DeadLetterReason: operation.DeadLetterReason,
		CreatedAt:        formatTime(operation.CreatedAt), UpdatedAt: formatTime(operation.UpdatedAt),
	}
}
