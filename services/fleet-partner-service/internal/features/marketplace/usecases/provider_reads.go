package usecases

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// ProviderOfferView is the provider-safe view of an organization offer.
//
// Built from an explicit allowlist. Nothing here comes from the source
// booking payload, which carries the customer fare — a provider needs to know
// where the work is and when they must answer, not what anyone paid.
type ProviderOfferView struct {
	OfferID        string    `json:"offerId"`
	RequestID      string    `json:"requestId"`
	SourceService  string    `json:"sourceService"`
	ServiceType    string    `json:"serviceType"`
	Status         string    `json:"status"`
	ExpiresAt      time.Time `json:"expiresAt"`
	Version        int64     `json:"version"`
	AllowedActions []string  `json:"allowedActions"`
}

// ProviderAssignmentSummaryView is the provider-safe view of an assignment.
type ProviderAssignmentSummaryView struct {
	AssignmentID     string    `json:"assignmentId"`
	RequestID        string    `json:"requestId"`
	SourceService    string    `json:"sourceService"`
	Status           string    `json:"status"`
	ResponseDeadline time.Time `json:"responseDeadline"`
	Version          int64     `json:"version"`
	AllowedActions   []string  `json:"allowedActions"`
}

// ProviderReadPage is a page of provider-facing results.
type ProviderReadPage[T any] struct {
	Items      []T  `json:"items"`
	NextOffset int  `json:"nextOffset"`
	HasMore    bool `json:"hasMore"`
}

// maxProviderPageSize bounds a provider read so one call cannot enumerate an
// organization's entire history.
const maxProviderPageSize = 50

func clampPageSize(requested int) int {
	if requested <= 0 {
		return 20
	}
	if requested > maxProviderPageSize {
		return maxProviderPageSize
	}
	return requested
}

// ListProviderOffers returns the live organization offers addressed to one
// dispatch provider.
//
// Scoping is by the provider's own memberships, resolved in SQL. A provider
// can only ever enumerate offers that were actually made to them.
func (s *Service) ListProviderOffers(ctx context.Context, providerService string,
	executingProviderID string, limit int, offset int) (*ProviderReadPage[ProviderOfferView], error) {
	sourceService, err := sourceServiceFor(providerService)
	if err != nil {
		return nil, err
	}
	size := clampPageSize(limit)

	// Fetch one extra row to detect a further page without a second count.
	recipients, err := s.opts.Offers.ListRecipientsForProvider(ctx, executingProviderID,
		sourceService, size+1, offset)
	if err != nil {
		return nil, err
	}

	hasMore := len(recipients) > size
	if hasMore {
		recipients = recipients[:size]
	}

	now := s.opts.Now()
	page := &ProviderReadPage[ProviderOfferView]{Items: []ProviderOfferView{}}
	for index := range recipients {
		recipient := recipients[index]
		offer, err := s.opts.Offers.GetByIDUnscoped(ctx, recipient.OfferID)
		if err != nil {
			continue
		}
		projection, err := s.opts.Projections.GetByID(ctx, offer.OrganizationID,
			offer.RequestProjectionID)
		if err != nil {
			continue
		}
		page.Items = append(page.Items, ProviderOfferView{
			OfferID:       offer.ID,
			RequestID:     projection.PlatformRequestID,
			SourceService: projection.SourceService,
			ServiceType:   projection.ServiceType,
			Status:        recipient.Status,
			ExpiresAt:     recipient.ExpiresAt,
			Version:       recipient.Version,
			// Server-computed from state. While the offer is actionable the
			// provider always has both; no policy narrows this.
			AllowedActions: recipientActions(recipient, now),
		})
	}
	page.HasMore = hasMore
	if hasMore {
		page.NextOffset = offset + size
	}
	return page, nil
}

// GetProviderOffer returns one offer belonging to this provider.
func (s *Service) GetProviderOffer(ctx context.Context, providerService string,
	executingProviderID string, offerID string) (*ProviderOfferView, error) {
	sourceService, err := sourceServiceFor(providerService)
	if err != nil {
		return nil, err
	}

	// Ownership is resolved in SQL by joining membership → provider. An offer
	// that exists but was made to somebody else is reported as NOT FOUND, not
	// as forbidden: telling a provider "that exists but isn't yours" would
	// leak which offers other providers hold.
	recipient, err := s.opts.Offers.GetRecipientForProvider(ctx, offerID,
		executingProviderID, sourceService)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.NotFound(errorcodes.MarketplaceOfferNotFound, "Offer not found.")
		}
		return nil, err
	}

	offer, err := s.opts.Offers.GetByIDUnscoped(ctx, recipient.OfferID)
	if err != nil {
		return nil, errorcodes.NotFound(errorcodes.MarketplaceOfferNotFound, "Offer not found.")
	}
	projection, err := s.opts.Projections.GetByID(ctx, offer.OrganizationID,
		offer.RequestProjectionID)
	if err != nil {
		return nil, errorcodes.NotFound(errorcodes.MarketplaceOfferNotFound, "Offer not found.")
	}

	return &ProviderOfferView{
		OfferID:        offer.ID,
		RequestID:      projection.PlatformRequestID,
		SourceService:  projection.SourceService,
		ServiceType:    projection.ServiceType,
		Status:         recipient.Status,
		ExpiresAt:      recipient.ExpiresAt,
		Version:        recipient.Version,
		AllowedActions: recipientActions(*recipient, s.opts.Now()),
	}, nil
}

// ListProviderAssignments returns one provider's company assignments.
func (s *Service) ListProviderAssignments(ctx context.Context, providerService string,
	executingProviderID string, limit int, offset int) (*ProviderReadPage[ProviderAssignmentSummaryView], error) {
	sourceService, err := sourceServiceFor(providerService)
	if err != nil {
		return nil, err
	}
	size := clampPageSize(limit)

	assignments, err := s.opts.Assignments.ListForProvider(ctx, executingProviderID,
		sourceService, size+1, offset)
	if err != nil {
		return nil, err
	}
	hasMore := len(assignments) > size
	if hasMore {
		assignments = assignments[:size]
	}

	now := s.opts.Now()
	page := &ProviderReadPage[ProviderAssignmentSummaryView]{
		Items: []ProviderAssignmentSummaryView{},
	}
	for index := range assignments {
		assignment := assignments[index]
		projection, err := s.opts.Projections.GetByID(ctx, assignment.OrganizationID,
			assignment.RequestProjectionID)
		requestID := ""
		if err == nil {
			requestID = projection.PlatformRequestID
		}
		page.Items = append(page.Items, ProviderAssignmentSummaryView{
			AssignmentID:     assignment.ID,
			RequestID:        requestID,
			SourceService:    assignment.SourceService,
			Status:           assignment.Status,
			ResponseDeadline: assignment.ResponseDeadline,
			Version:          assignment.Version,
			AllowedActions:   assignment.AllowedActions(now),
		})
	}
	page.HasMore = hasMore
	if hasMore {
		page.NextOffset = offset + size
	}
	return page, nil
}

// GetProviderAssignment returns one assignment belonging to this provider.
func (s *Service) GetProviderAssignment(ctx context.Context, providerService string,
	executingProviderID string, assignmentID string) (*ProviderAssignmentSummaryView, error) {
	sourceService, err := sourceServiceFor(providerService)
	if err != nil {
		return nil, err
	}

	assignment, err := s.opts.Assignments.GetForProvider(ctx, assignmentID,
		executingProviderID, sourceService)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			// Same reasoning as offers: not-found, never "exists but not yours".
			return nil, errorcodes.NotFound(errorcodes.AssignmentNotFound, "Assignment not found.")
		}
		return nil, err
	}

	requestID := ""
	if projection, err := s.opts.Projections.GetByID(ctx, assignment.OrganizationID,
		assignment.RequestProjectionID); err == nil {
		requestID = projection.PlatformRequestID
	}

	return &ProviderAssignmentSummaryView{
		AssignmentID:     assignment.ID,
		RequestID:        requestID,
		SourceService:    assignment.SourceService,
		Status:           assignment.Status,
		ResponseDeadline: assignment.ResponseDeadline,
		Version:          assignment.Version,
		AllowedActions:   assignment.AllowedActions(s.opts.Now()),
	}, nil
}

// recipientActions computes the actions available on an offer recipient row.
func recipientActions(recipient models.OfferRecipient, now time.Time) []string {
	if recipient.Actionable(now) {
		return []string{"accept", "decline"}
	}
	return []string{}
}

// sourceServiceFor maps an asserted provider service to its source service.
func sourceServiceFor(providerService string) (string, error) {
	switch providerService {
	case "DISPATCH":
		return models.SourceDispatch, nil
	case "TAXI":
		return models.SourceTaxi, nil
	case "HAULING":
		return models.SourceHauling, nil
	default:
		return "", errorcodes.BadRequest(errorcodes.ValidationFailed,
			"providerService is not recognised.")
	}
}
