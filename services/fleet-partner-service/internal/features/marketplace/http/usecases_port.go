package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/providerreceipt"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	AcceptAssignmentForProvider(ctx context.Context, input usecases.AcceptAssignmentInput) (*usecases.AssignmentResult, error)
	AcceptCorporateOffer(ctx context.Context, t tenant.Context, input usecases.CorporateAcceptOfferInput) (*models.Offer, error)
	AcceptOffer(ctx context.Context, input usecases.OfferResponseInput) (*usecases.OfferResponseResult, error)
	AcknowledgeAssignmentForProvider(ctx context.Context, input usecases.AcknowledgeAssignmentInput) (*usecases.AcknowledgementResult, error)
	AssignCorporateRequest(ctx context.Context, t tenant.Context, input usecases.CorporateAssignInput) (*models.Assignment, error)
	CancelCorporateAssignment(ctx context.Context, t tenant.Context, input usecases.CorporateCancelAssignmentInput) error
	CancelCorporateTrip(ctx context.Context, t tenant.Context, input usecases.CorporateCancelTripInput) (*usecases.CorporateCancelTripResult, error)
	DeclineAssignmentForProvider(ctx context.Context, input usecases.DeclineAssignmentInput) (*usecases.AssignmentResult, error)
	DeclineCorporateOffer(ctx context.Context, t tenant.Context, input usecases.CorporateDeclineOfferInput) error
	DeclineOffer(ctx context.Context, input usecases.OfferResponseInput) (*usecases.OfferResponseResult, error)
	GetCorporateAssignment(ctx context.Context, t tenant.Context, assignmentID string) (*models.Assignment, error)
	GetCorporateOffer(ctx context.Context, t tenant.Context, offerID string) (*usecases.CorporateOfferDetail, error)
	GetCorporateOfferMetrics(ctx context.Context, t tenant.Context) (*usecases.CorporateOfferMetrics, error)
	GetCorporateTrip(ctx context.Context, t tenant.Context, tripID string) (*usecases.CorporateTripDetail, error)
	GetProviderAssignment(ctx context.Context, providerService string, executingProviderID string, assignmentID string) (*usecases.ProviderAssignmentSummaryView, error)
	GetProviderOffer(ctx context.Context, providerService string, executingProviderID string, offerID string) (*usecases.ProviderOfferView, error)
	IngestEvent(ctx context.Context, raw []byte, event usecases.InboundEvent) (usecases.IngestResult, error)
	ListCorporateAssignmentHistory(ctx context.Context, t tenant.Context, assignmentID string) ([]models.StateHistoryEntry, error)
	ListCorporateAssignments(ctx context.Context, t tenant.Context, filter usecases.CorporatePageFilter) ([]models.Assignment, error)
	ListCorporateCandidates(ctx context.Context, t tenant.Context, projectionID string) ([]usecases.CorporateCandidate, error)
	ListCorporateOffers(ctx context.Context, t tenant.Context, filter usecases.CorporatePageFilter) ([]usecases.CorporateOfferSummary, error)
	ListCorporateTrips(ctx context.Context, t tenant.Context, filter usecases.CorporateTripFilter) (*usecases.CorporateTripPage, error)
	ListProviderAssignments(ctx context.Context, providerService string, executingProviderID string, limit int, offset int) (*usecases.ProviderReadPage[usecases.ProviderAssignmentSummaryView], error)
	ListProviderMemberships(ctx context.Context, providerService string, providerID string) ([]usecases.ProviderMembershipView, error)
	ListProviderOffers(ctx context.Context, providerService string, executingProviderID string, limit int, offset int) (*usecases.ProviderReadPage[usecases.ProviderOfferView], error)
	ProviderDecisionReceipt(ctx context.Context, assignmentID string, providerID string, sourceService string, key string) (*providerreceipt.Receipt, error)
	ProviderOfferDecisionReceipt(ctx context.Context, offerID string, providerID string, sourceService string, key string) (*providerreceipt.Receipt, error)
	ReassignCorporateAssignment(ctx context.Context, t tenant.Context, input usecases.CorporateReassignAssignmentInput) (*models.Assignment, error)
	VerifyProviderAcceptance(ctx context.Context, assignmentID string) (*usecases.ProviderAcceptanceView, error)
}
