package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// AssignmentStore lists the repositories.AssignmentRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type AssignmentStore interface {
	AppendAcknowledgementHistory(ctx context.Context, q database.Querier, assignment *models.Assignment, entry models.StateHistoryEntry) error
	ClaimExpired(ctx context.Context, q database.Querier, limit int) ([]models.Assignment, error)
	CountStaleOpenReassignmentCases(ctx context.Context, olderThan time.Duration) (int64, error)
	Create(ctx context.Context, q database.Querier, organizationID string, assignment *models.Assignment) error
	CreateAcknowledgement(ctx context.Context, q database.Querier, assignment *models.Assignment, expectedVersion int64, acknowledgement *models.Acknowledgement) error
	CreateFollowup(ctx context.Context, q database.Querier, action *models.FollowupAction) error
	CreateReassignmentCase(ctx context.Context, q database.Querier, organizationID string, item *models.ReassignmentCase) error
	CreateResponse(ctx context.Context, q database.Querier, response *models.Response) error
	FindAcknowledgementByIdempotencyKey(ctx context.Context, membershipID string, idempotencyKey string) (*models.Acknowledgement, error)
	FindResponseByClientID(ctx context.Context, membershipID string, clientResponseID string) (*models.Response, error)
	GetByID(ctx context.Context, organizationID string, assignmentID string) (*models.Assignment, error)
	GetByIDUnscoped(ctx context.Context, assignmentID string) (*models.Assignment, error)
	GetForProvider(ctx context.Context, assignmentID string, executingProviderID string, sourceService string) (*models.Assignment, error)
	GetOpenReassignmentCase(ctx context.Context, organizationID string, projectionID string) (*models.ReassignmentCase, error)
	GetResponse(ctx context.Context, q database.Querier, assignmentID string) (*models.Response, error)
	HoldReservation(ctx context.Context, q database.Querier, organizationID string, assignmentID string, resourceType string, resourceID string) error
	List(ctx context.Context, query tenant.Query, status string) ([]models.Assignment, error)
	ListForProvider(ctx context.Context, executingProviderID string, sourceService string, limit int, offset int) ([]models.Assignment, error)
	ListHistory(ctx context.Context, organizationID string, assignmentID string) ([]models.StateHistoryEntry, error)
	ListStaleOpenReassignmentCases(ctx context.Context, olderThan time.Duration, limit int) ([]models.ReassignmentCase, error)
	LockByID(ctx context.Context, q database.Querier, assignmentID string) (*models.Assignment, error)
	LockForProvider(ctx context.Context, q database.Querier, assignmentID string, membershipID string) (*models.Assignment, error)
	MembershipHomeBranch(ctx context.Context, organizationID string, membershipID string) (*string, error)
	ReleaseReservations(ctx context.Context, q database.Querier, assignmentID string) error
	RequiresAcknowledgement(ctx context.Context, q database.Querier, organizationID string) (bool, error)
	ResolveReassignmentCase(ctx context.Context, q database.Querier, caseID string, replacementAssignmentID string, expectedVersion int64) error
	ResponseEvidence(ctx context.Context, response *models.Response) ([]string, error)
	SetExecutionStatus(ctx context.Context, q database.Querier, assignmentID string, status string, sourceExecutionID string) error
	Transition(ctx context.Context, q database.Querier, current *models.Assignment, toStatus string, expectedVersion int64, entry models.StateHistoryEntry) error
}

// CommandStore lists the repositories.CommandRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type CommandStore interface {
	Begin(ctx context.Context, q database.Querier, operation *models.CommandOperation) (*models.CommandOperation, bool, error)
	ClaimDue(ctx context.Context, q database.Querier, limit int) ([]models.CommandOperation, error)
	SetStatus(ctx context.Context, q database.Querier, operationID string, status string, externalReference string, errorCode string, nextAttempt *time.Time) error
}

// ExecutionStartStore lists the repositories.ExecutionStartRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type ExecutionStartStore interface {
	ClaimEnabled(ctx context.Context, q database.Querier, owner string, lease time.Duration, limit int, enabledSources []string) ([]models.ExecutionStartOperation, error)
	Confirm(ctx context.Context, q database.Querier, id string, tripID string, snapshot any) error
	Create(ctx context.Context, q database.Querier, op *models.ExecutionStartOperation) error
	Fail(ctx context.Context, q database.Querier, id string, errorCode string, backoff time.Duration, maxAttempts int, permanent bool) error
	MarkUnknown(ctx context.Context, q database.Querier, id string, errorCode string, backoff time.Duration) error
	RecoverExpiredLeases(ctx context.Context, q database.Querier) (int64, error)
	SetStatus(ctx context.Context, q database.Querier, id string, status string) error
}

// OfferStore lists the repositories.OfferRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type OfferStore interface {
	ClaimDueOffers(ctx context.Context, q database.Querier, now time.Time, limit int) ([]models.Offer, error)
	CloseOpenRecipients(ctx context.Context, q database.Querier, organizationID string, offerID string, status string) (int64, error)
	Create(ctx context.Context, q database.Querier, organizationID string, offer *models.Offer) error
	CreateAcceptance(ctx context.Context, q database.Querier, organizationID string, acceptance *models.Acceptance) error
	CreateRecipient(ctx context.Context, q database.Querier, organizationID string, recipient *models.OfferRecipient) error
	ExtendOpenOffer(ctx context.Context, q database.Querier, organizationID string, offerID string, expiresAt time.Time) (bool, error)
	GetAcceptance(ctx context.Context, organizationID string, offerID string) (*models.Acceptance, error)
	GetByID(ctx context.Context, organizationID string, offerID string) (*models.Offer, error)
	GetByIDUnscoped(ctx context.Context, offerID string) (*models.Offer, error)
	GetLiveByProjection(ctx context.Context, q database.Querier, organizationID string, projectionID string) (*models.Offer, error)
	GetRecipientForProvider(ctx context.Context, offerID string, executingProviderID string, sourceService string) (*models.OfferRecipient, error)
	ListRecipientsForProvider(ctx context.Context, executingProviderID string, sourceService string, limit int, offset int) ([]models.OfferRecipient, error)
	LockByID(ctx context.Context, q database.Querier, organizationID string, offerID string) (*models.Offer, error)
	LockRecipientForProvider(ctx context.Context, q database.Querier, offerID string, executingProviderID string, sourceService string) (*models.OfferRecipient, error)
	MarkLosersOfRace(ctx context.Context, q database.Querier, offerID string, winnerRecipientID string) error
	SetRecipientStatus(ctx context.Context, q database.Querier, recipientID string, status string, expectedVersion int64, clientResponseID string, reasonCode string, reasonText string) error
	SetSourceReservation(ctx context.Context, q database.Querier, offerID string, reservationID string) error
	SetStatus(ctx context.Context, q database.Querier, organizationID string, offerID string, status string, expectedVersion int64, actorType string, actorID string, reasonCode string, reasonText string) error
}

// ProjectionStore lists the repositories.ProjectionRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type ProjectionStore interface {
	ApplySourceEvent(ctx context.Context, q database.Querier, projectionID string, sourceVersion int64, eventID string, occurredAt time.Time, apply func(freshness string) error) (repositories.SourceEventOutcome, error)
	FindBySourceRequestAnyOrganization(ctx context.Context, q database.Querier, sourceService string, platformRequestID string) (*models.RequestProjection, error)
	GetByID(ctx context.Context, organizationID string, projectionID string) (*models.RequestProjection, error)
	InsertOpportunity(ctx context.Context, q database.Querier, projection *models.RequestProjection) (bool, error)
	List(ctx context.Context, query tenant.Query, status string) ([]models.RequestProjection, error)
	ListBySourceRequest(ctx context.Context, q database.Querier, sourceService string, platformRequestID string) ([]models.RequestProjection, error)
	ListOpportunityCandidates(ctx context.Context, q database.Querier, serviceCategory string, organizationIDs []string, providerIDs []string, limit int) ([]repositories.OpportunityCandidate, error)
	LockByID(ctx context.Context, q database.Querier, organizationID string, projectionID string) (*models.RequestProjection, error)
	MarkStale(ctx context.Context, olderThan time.Duration, limit int) (int64, error)
	ReconcileFromSource(ctx context.Context, q database.Querier, projectionID string, sourceVersion int64, sourceStatus string, projectionStatus string) error
	SetExecutionContext(ctx context.Context, q database.Querier, organizationID string, projectionID string, membershipID string, executingProviderID string, assignmentID string, acceptanceSource string) error
	SetExpiresAt(ctx context.Context, q database.Querier, projectionID string, expiresAt time.Time) error
	SetPlatformTripID(ctx context.Context, q database.Querier, projectionID string, tripID string) error
	SetProjectionStatusUnversioned(ctx context.Context, q database.Querier, projectionID string, status string) error
	SetSourceStatus(ctx context.Context, q database.Querier, projectionID string, sourceStatus string) error
	SetStatus(ctx context.Context, q database.Querier, organizationID string, projectionID string, status string, expectedVersion int64) error
}

// TripStore lists the repositories.TripRepository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type TripStore interface {
	AppendHistory(ctx context.Context, q database.Querier, organizationID string, tripProjectionID string, fromStatus string, toStatus string, sourceVersion int64, eventID string) error
	ApplyDeliveryFailure(ctx context.Context, q database.Querier, tripProjectionID string, failure repositories.DeliveryFailureProjection) error
	ApplyRecipientConfirmation(ctx context.Context, q database.Querier, tripProjectionID string, confirmation repositories.RecipientConfirmationProjection) error
	ApplyTripSourceEvent(ctx context.Context, q database.Querier, event repositories.TripSourceEvent) (repositories.TripEventOutcome, *models.TripProjection, error)
	ClaimReconciliationCases(ctx context.Context, q database.Querier, limit int) ([]repositories.ReconciliationCase, error)
	ClaimTripReconciliationLease(ctx context.Context, q database.Querier, caseID string, owner string, lease time.Duration) error
	GetBySourceTripAnyOrganization(ctx context.Context, q database.Querier, sourceService string, platformTripID string) (*models.TripProjection, error)
	GetDeliveryOutcome(ctx context.Context, organizationID string, tripProjectionID string) (*repositories.DeliveryOutcome, error)
	GetScoped(ctx context.Context, query tenant.Query, tripID string) (*models.TripProjection, error)
	ListAssignmentsBehindTrips(ctx context.Context, limit int) ([]repositories.AssignmentTripLink, error)
	ListForReconciliation(ctx context.Context, limit int) ([]models.TripProjection, error)
	ListPage(ctx context.Context, query tenant.Query, filter repositories.TripPageFilter) ([]models.TripProjection, bool, error)
	ListTripHistory(ctx context.Context, organizationID string, tripID string, limit int) ([]repositories.TripHistoryEntry, error)
	LockTripByID(ctx context.Context, q database.Querier, organizationID string, tripID string) (*models.TripProjection, error)
	MarkStaleTrips(ctx context.Context, olderThan time.Duration, limit int) (int64, error)
	OpenReconciliation(ctx context.Context, q database.Querier, organizationID string, sourceService string, aggregateType string, aggregateSourceID string, reason string, nextAttempt time.Time) error
	ReconcileTripFromSource(ctx context.Context, q database.Querier, tripProjectionID string, sourceVersion int64, tripStatus string, completedAt *time.Time, cancelledAt *time.Time) error
	RecoverExpiredReconciliationLeases(ctx context.Context, q database.Querier) (int64, error)
	ResolveGaps(ctx context.Context, q database.Querier, sourceService string, aggregateType string, aggregateSourceID string) error
	ResolveReconciliation(ctx context.Context, q database.Querier, caseID string, status string, errorCode string, nextAttempt *time.Time) error
}
