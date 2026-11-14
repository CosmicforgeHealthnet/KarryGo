package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	ActiveOrganizationUser(ctx context.Context, q database.Querier, organizationID string, userID string) (bool, error)
	AppendContactHistory(ctx context.Context, q database.Querier, entry models.ContactHistory) error
	AppendContractHistory(ctx context.Context, q database.Querier, entry models.ContractHistory) error
	AppendCreditHistory(ctx context.Context, q database.Querier, entry models.CreditHistory) error
	AppendCustomerHistory(ctx context.Context, q database.Querier, entry models.CustomerHistory) error
	AppendLeadHistory(ctx context.Context, q database.Querier, entry models.LeadHistory) error
	AppendLocationHistory(ctx context.Context, q database.Querier, entry models.LocationHistory) error
	AppendObservation(ctx context.Context, q database.Querier, observation models.SLAObservation) (models.SLAObservation, bool, error)
	AppendOpportunityHistory(ctx context.Context, q database.Querier, entry models.OpportunityHistory) error
	AppendQuotationHistory(ctx context.Context, q database.Querier, entry models.QuotationHistory) error
	AppendSLAHistory(ctx context.Context, q database.Querier, entry models.SLAHistory) error
	ArchiveContact(ctx context.Context, q database.Querier, organizationID string, customerID string, contactID string, expectedVersion int64, actor string) (models.Contact, error)
	ArchiveLocation(ctx context.Context, q database.Querier, organizationID string, customerID string, locationID string, expectedVersion int64, actor string) (models.Location, error)
	AttachContractDocument(ctx context.Context, q database.Querier, doc models.DocumentReference) (models.DocumentReference, error)
	AttachCreditProfile(ctx context.Context, q database.Querier, organizationID string, customerID string, creditProfileID string) error
	AttachQuotationDocument(ctx context.Context, q database.Querier, doc models.DocumentReference) (models.DocumentReference, error)
	BindContractDocumentsToVersion(ctx context.Context, q database.Querier, organizationID string, contractID string, versionID string) error
	BindQuotationDocumentsToVersion(ctx context.Context, q database.Querier, organizationID string, quotationID string, versionID string) error
	ClaimExpiredQuotations(ctx context.Context, limit int) ([]models.Quotation, error)
	ClaimExpiringContracts(ctx context.Context, limit int) ([]models.Contract, error)
	ClaimMissedObservations(ctx context.Context, since time.Time, limit int) ([]models.SLAObservation, error)
	ClaimNotifications(ctx context.Context, owner string, leaseFor time.Duration, limit int) ([]models.NotificationOperation, error)
	ClaimRenewalDueContracts(ctx context.Context, window time.Duration, limit int) ([]models.Contract, error)
	ClaimUnevaluatedObservations(ctx context.Context, limit int) ([]models.SLAObservation, error)
	CountContacts(ctx context.Context) (int64, error)
	CountContractsByStatus(ctx context.Context) (map[string]int64, error)
	CountCreditHolds(ctx context.Context) (int64, int64, error)
	CountCustomersByStatus(ctx context.Context) (map[string]int64, error)
	CountLocations(ctx context.Context) (int64, error)
	CountObservationsByOutcome(ctx context.Context) (map[string]int64, error)
	CountOpportunitiesByStage(ctx context.Context) (map[string]int64, error)
	CountSLAs(ctx context.Context) (int64, error)
	CreateAcceptance(ctx context.Context, q database.Querier, acceptance models.Acceptance) (models.Acceptance, error)
	CreateAccessToken(ctx context.Context, q database.Querier, token models.QuotationAccessToken) (models.QuotationAccessToken, error)
	CreateContact(ctx context.Context, q database.Querier, c models.Contact) (models.Contact, error)
	CreateContract(ctx context.Context, q database.Querier, contract models.Contract) (models.Contract, error)
	CreateContractVersion(ctx context.Context, q database.Querier, version models.ContractVersion) (models.ContractVersion, error)
	CreateConversionIntent(ctx context.Context, q database.Querier, intent models.ConversionIntent) (models.ConversionIntent, error)
	CreateCustomer(ctx context.Context, q database.Querier, c models.Customer) (models.Customer, error)
	CreateLead(ctx context.Context, q database.Querier, l models.Lead) (models.Lead, error)
	CreateLocation(ctx context.Context, q database.Querier, l models.Location) (models.Location, error)
	CreateOpportunity(ctx context.Context, q database.Querier, o models.Opportunity) (models.Opportunity, error)
	CreateQuotation(ctx context.Context, q database.Querier, quotation models.Quotation) (models.Quotation, error)
	CreateQuotationVersion(ctx context.Context, q database.Querier, version models.QuotationVersion) (models.QuotationVersion, error)
	CreateSLA(ctx context.Context, q database.Querier, sla models.SLA) (models.SLA, error)
	DemotePrimaryContacts(ctx context.Context, q database.Querier, organizationID string, customerID string, purpose string, exceptContactID string) error
	EnqueueNotification(ctx context.Context, q database.Querier, operation models.NotificationOperation) (models.NotificationOperation, error)
	FindAcceptanceByKey(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) (models.Acceptance, error)
	FindAccessTokenByHash(ctx context.Context, q database.Querier, tokenHash string) (models.QuotationAccessToken, error)
	GetContact(ctx context.Context, q database.Querier, organizationID string, customerID string, contactID string) (models.Contact, error)
	GetContract(ctx context.Context, q database.Querier, organizationID string, contractID string) (models.Contract, error)
	GetContractDocument(ctx context.Context, q database.Querier, organizationID string, contractID string, documentID string) (models.DocumentReference, error)
	GetContractVersion(ctx context.Context, q database.Querier, organizationID string, contractID string, versionID string) (models.ContractVersion, error)
	GetConversionIntent(ctx context.Context, q database.Querier, organizationID string, acceptedVersionID string) (models.ConversionIntent, error)
	GetCreditProfile(ctx context.Context, q database.Querier, organizationID string, customerID string) (models.CreditProfile, error)
	GetCustomer(ctx context.Context, q database.Querier, organizationID string, customerID string) (models.Customer, error)
	GetLead(ctx context.Context, q database.Querier, organizationID string, leadID string) (models.Lead, error)
	GetLeadByCreateKey(ctx context.Context, q database.Querier, organizationID string, key string) (models.Lead, error)
	GetLocation(ctx context.Context, q database.Querier, organizationID string, customerID string, locationID string) (models.Location, error)
	GetOpportunity(ctx context.Context, q database.Querier, organizationID string, opportunityID string) (models.Opportunity, error)
	GetQuotation(ctx context.Context, q database.Querier, organizationID string, quotationID string) (models.Quotation, error)
	GetQuotationVersion(ctx context.Context, q database.Querier, organizationID string, quotationID string, versionID string) (models.QuotationVersion, error)
	GetSLA(ctx context.Context, q database.Querier, organizationID string, slaID string) (models.SLA, error)
	LatestObservationVersion(ctx context.Context, q database.Querier, slaID string, sourceType string, sourceReference string) (int64, error)
	ListContacts(ctx context.Context, organizationID string, customerID string, filter repositories.ContactFilter) ([]models.Contact, error)
	ListContractDocuments(ctx context.Context, q database.Querier, organizationID string, contractID string) ([]models.DocumentReference, error)
	ListContractHistory(ctx context.Context, organizationID string, contractID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.ContractHistory, error)
	ListContractVersions(ctx context.Context, organizationID string, contractID string, limit int) ([]models.ContractVersion, error)
	ListContracts(ctx context.Context, organizationID string, branchIDs []string, filter repositories.ContractFilter) ([]models.Contract, error)
	ListCreditHistory(ctx context.Context, organizationID string, creditProfileID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.CreditHistory, error)
	ListCustomerHistory(ctx context.Context, organizationID string, customerID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.CustomerHistory, error)
	ListCustomers(ctx context.Context, organizationID string, branchIDs []string, filter repositories.CustomerFilter) ([]models.Customer, error)
	ListLeadHistory(ctx context.Context, organizationID string, leadID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.LeadHistory, error)
	ListLeads(ctx context.Context, organizationID string, branchIDs []string, filter repositories.LeadFilter) ([]models.Lead, error)
	ListLineItems(ctx context.Context, q database.Querier, organizationID string, quotationID string) ([]models.LineItem, error)
	ListLocations(ctx context.Context, organizationID string, customerID string, filter repositories.LocationFilter) ([]models.Location, error)
	ListNotifications(ctx context.Context, organizationID string, status string, subjectID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.NotificationOperation, error)
	ListObservations(ctx context.Context, organizationID string, slaID string, outcome string, page cursor.Cursor, hasCursor bool, limit int) ([]models.SLAObservation, error)
	ListOpportunities(ctx context.Context, organizationID string, branchIDs []string, filter repositories.OpportunityFilter) ([]models.Opportunity, error)
	ListOpportunityHistory(ctx context.Context, organizationID string, opportunityID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.OpportunityHistory, error)
	ListQuotationDocuments(ctx context.Context, q database.Querier, organizationID string, quotationID string) ([]models.DocumentReference, error)
	ListQuotationHistory(ctx context.Context, organizationID string, quotationID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.QuotationHistory, error)
	ListQuotationVersions(ctx context.Context, organizationID string, quotationID string, limit int) ([]models.QuotationVersion, error)
	ListQuotations(ctx context.Context, organizationID string, branchIDs []string, filter repositories.QuotationFilter) ([]models.Quotation, error)
	ListSLAHistory(ctx context.Context, organizationID string, slaID string, page cursor.Cursor, hasCursor bool, limit int) ([]models.SLAHistory, error)
	ListSLAs(ctx context.Context, organizationID string, filter repositories.SLAFilter) ([]models.SLA, error)
	LockContact(ctx context.Context, q database.Querier, organizationID string, customerID string, contactID string) (models.Contact, error)
	LockContract(ctx context.Context, q database.Querier, organizationID string, contractID string) (models.Contract, error)
	LockCreditProfile(ctx context.Context, q database.Querier, organizationID string, customerID string) (models.CreditProfile, error)
	LockCustomer(ctx context.Context, q database.Querier, organizationID string, customerID string) (models.Customer, error)
	LockLead(ctx context.Context, q database.Querier, organizationID string, leadID string) (models.Lead, error)
	LockLocation(ctx context.Context, q database.Querier, organizationID string, customerID string, locationID string) (models.Location, error)
	LockOpportunity(ctx context.Context, q database.Querier, organizationID string, opportunityID string) (models.Opportunity, error)
	LockQuotation(ctx context.Context, q database.Querier, organizationID string, quotationID string) (models.Quotation, error)
	LockSLA(ctx context.Context, q database.Querier, organizationID string, slaID string) (models.SLA, error)
	MarkLeadConverted(ctx context.Context, q database.Querier, organizationID string, leadID string, expectedVersion int64, fromStatus string, customerID string, opportunityID string, idempotencyKey string, actor string) (models.Lead, error)
	MarkNotificationDelivered(ctx context.Context, owner string, operationID string, deliveryReference string) error
	MarkNotificationPermanent(ctx context.Context, owner string, operationID string, safeErrorCode string) error
	MarkNotificationRetryable(ctx context.Context, owner string, operationID string, safeErrorCode string, backoff time.Duration, maxAttempts int) error
	MarkNotificationUnknown(ctx context.Context, owner string, operationID string, safeErrorCode string, backoff time.Duration) error
	NextContractNumber(ctx context.Context, q database.Querier, organizationID string, now time.Time) (string, error)
	NextQuotationNumber(ctx context.Context, q database.Querier, organizationID string, now time.Time) (string, error)
	NotificationBacklog(ctx context.Context) (int64, int64, error)
	OpenRenewalOf(ctx context.Context, q database.Querier, organizationID string, contractID string) (models.Contract, error)
	OpportunityPriorStages(ctx context.Context, q database.Querier, organizationID string, opportunityID string) ([]string, error)
	PrimaryContact(ctx context.Context, q database.Querier, organizationID string, customerID string, purpose string) (models.Contact, error)
	PromoteContact(ctx context.Context, q database.Querier, organizationID string, customerID string, contactID string, expectedVersion int64, actor string) (models.Contact, error)
	RecordTokenView(ctx context.Context, q database.Querier, tokenID string) error
	RecoverExpiredNotificationLeases(ctx context.Context) (int64, error)
	RemoveContractDocument(ctx context.Context, q database.Querier, organizationID string, contractID string, documentID string) error
	ReplaceLineItems(ctx context.Context, q database.Querier, organizationID string, quotationID string, items []models.LineItem) ([]models.LineItem, error)
	ReplayNotification(ctx context.Context, q database.Querier, organizationID string, operationID string) (models.NotificationOperation, error)
	RequireMandatoryContractDocumentsReady(ctx context.Context, q database.Querier, organizationID string, contractID string) error
	RequireMandatoryDocumentsReady(ctx context.Context, q database.Querier, organizationID string, quotationID string) error
	RevokeQuotationTokens(ctx context.Context, q database.Querier, organizationID string, quotationID string, reason string) error
	SetCreditHold(ctx context.Context, q database.Querier, organizationID string, customerID string, expectedVersion int64, fromHold string, toHold string, reason string, approvalRequestID string, actor string) (models.CreditProfile, error)
	SetCustomerRisk(ctx context.Context, q database.Querier, organizationID string, customerID string, expectedVersion int64, riskStatus string, actor string) (models.Customer, error)
	SetCustomerStatus(ctx context.Context, q database.Querier, organizationID string, customerID string, expectedVersion int64, status string, actor string, archive bool) (models.Customer, error)
	SyncCreditRiskSnapshot(ctx context.Context, q database.Querier, organizationID string, customerID string, riskStatus string, forceRestrict bool) error
	TransitionContract(ctx context.Context, q database.Querier, organizationID string, contractID string, expectedVersion int64, transition repositories.ContractTransition) (models.Contract, error)
	TransitionLead(ctx context.Context, q database.Querier, organizationID string, leadID string, expectedVersion int64, fromStatus string, toStatus string, disqualificationReason string, actor string) (models.Lead, error)
	TransitionOpportunity(ctx context.Context, q database.Querier, organizationID string, opportunityID string, expectedVersion int64, fromStage string, toStage string, lossReason string, holdReason string, actor string) (models.Opportunity, error)
	TransitionQuotation(ctx context.Context, q database.Querier, organizationID string, quotationID string, expectedVersion int64, transition repositories.QuotationTransition) (models.Quotation, error)
	TransitionSLA(ctx context.Context, q database.Querier, organizationID string, slaID string, expectedVersion int64, fromStatus string, toStatus string, actor string) (models.SLA, error)
	UpdateContact(ctx context.Context, q database.Querier, organizationID string, customerID string, contactID string, expectedVersion int64, update repositories.ContactUpdate) (models.Contact, error)
	UpdateContract(ctx context.Context, q database.Querier, organizationID string, contractID string, expectedVersion int64, update repositories.ContractUpdate) (models.Contract, error)
	UpdateContractDocumentProjection(ctx context.Context, q database.Querier, organizationID string, documentID string, statusProjection string, readinessState string, lastEventID string, sourceVersion int64) error
	UpdateCustomer(ctx context.Context, q database.Querier, organizationID string, customerID string, expectedVersion int64, update repositories.CustomerUpdate) (models.Customer, error)
	UpdateLead(ctx context.Context, q database.Querier, organizationID string, leadID string, expectedVersion int64, update repositories.LeadUpdate) (models.Lead, error)
	UpdateLocation(ctx context.Context, q database.Querier, organizationID string, customerID string, locationID string, expectedVersion int64, update repositories.LocationUpdate) (models.Location, error)
	UpdateOpportunity(ctx context.Context, q database.Querier, organizationID string, opportunityID string, expectedVersion int64, update repositories.OpportunityUpdate) (models.Opportunity, error)
	UpdateQuotation(ctx context.Context, q database.Querier, organizationID string, quotationID string, expectedVersion int64, update repositories.QuotationUpdate) (models.Quotation, error)
	UpdateSLA(ctx context.Context, q database.Querier, organizationID string, slaID string, expectedVersion int64, update repositories.SLAUpdate) (models.SLA, error)
	UpsertCreditProfile(ctx context.Context, q database.Querier, profile models.CreditProfile, expectedVersion int64, requireVersion bool) (models.CreditProfile, error)
}
