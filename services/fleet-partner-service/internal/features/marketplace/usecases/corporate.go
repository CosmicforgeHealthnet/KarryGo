package usecases

// Corporate marketplace surface: the read and command methods the Fleet
// manager's web app calls. This file adds NO new business logic. Every write
// delegates to a method that already exists in this package; reads compose
// existing repository calls.
//
// Naming: every method and type in this file is prefixed or suffixed with
// "Corporate" so nothing collides with the internal, provider-facing names
// already defined in this package.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// CorporatePageFilter is the corporate list filter. Pagination is cursor-based
// (CursorSortValue + CursorLastID), matching tenant.Query and every other
// tenant-scoped list repository in this service.
type CorporatePageFilter struct {
	Status          string
	BranchID        string
	Limit           int
	CursorSortValue string
	CursorLastID    string
}

func (f CorporatePageFilter) boundedLimit() int {
	if f.Limit <= 0 {
		return 25
	}
	if f.Limit > 100 {
		return 100
	}
	return f.Limit
}

// CorporateOfferSummary is one row in the corporate offers list.
type CorporateOfferSummary struct {
	OfferID                 string         `json:"offerId"`
	RequestProjectionID     string         `json:"requestProjectionId"`
	SourceService           string         `json:"sourceService"`
	ServiceType             string         `json:"serviceType"`
	Status                  string         `json:"status"`
	OperatingMode           string         `json:"operatingMode"`
	OfferedAt               string         `json:"offeredAt"`
	ExpiresAt               string         `json:"expiresAt"`
	PickupSummary           map[string]any `json:"pickupSummary"`
	DropoffSummary          map[string]any `json:"dropoffSummary"`
	OperationalRequirements map[string]any `json:"operationalRequirements"`
	Version                 int64          `json:"version"`
}

// CorporateOfferAcceptanceView is the immutable acceptance record.
type CorporateOfferAcceptanceView struct {
	AcceptanceID        string `json:"acceptanceId"`
	AcceptanceSource    string `json:"acceptanceSource"`
	SourceReservationID string `json:"sourceReservationId"`
	AcceptedAt          string `json:"acceptedAt"`
}

// CorporateOfferDetail is the single-offer view.
type CorporateOfferDetail struct {
	Offer      CorporateOfferSummary         `json:"offer"`
	Acceptance *CorporateOfferAcceptanceView `json:"acceptance,omitempty"`
}

// CorporateOfferMetrics is the four-counter panel.
type CorporateOfferMetrics struct {
	OpenOffers    int `json:"openOffers"`
	ExpiringSoon  int `json:"expiringSoon"`
	AcceptedToday int `json:"acceptedToday"`
	DeclinedToday int `json:"declinedToday"`
}

// CorporateCandidate is one eligible provider membership for a request.
type CorporateCandidate struct {
	MembershipID    string `json:"membershipId"`
	ProviderID      string `json:"providerId"`
	SourceService   string `json:"sourceService"`
	ServiceCategory string `json:"serviceCategory"`
	Status          string `json:"status"`
	Eligible        bool   `json:"eligible"`
	Reason          string `json:"reason,omitempty"`
}

// CorporateAcceptOfferInput commits the organization to an offer.
type CorporateAcceptOfferInput struct {
	OfferID         string
	ExpectedVersion int64
	IdempotencyKey  string
}

// CorporateDeclineOfferInput refuses an offer for the organization.
type CorporateDeclineOfferInput struct {
	OfferID    string
	ReasonCode string
	Note       string
}

// CorporateAssignInput assigns an accepted request to a provider.
type CorporateAssignInput struct {
	ProjectionID           string
	MembershipID           string
	AssetReference         string
	ResponseTimeoutSeconds int
	ExpectedRequestVersion int64
	IdempotencyKey         string
}

// CorporateCancelAssignmentInput cancels an assignment.
type CorporateCancelAssignmentInput struct {
	AssignmentID    string
	Reason          string
	ExpectedVersion int64
}

// CorporateReassignAssignmentInput replaces an assignment.
type CorporateReassignAssignmentInput struct {
	AssignmentID           string
	MembershipID           string
	AssetReference         string
	ResponseTimeoutSeconds int
	Reason                 string
	IdempotencyKey         string
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListCorporateOffers returns the organization's live offers.
func (s *Service) ListCorporateOffers(ctx context.Context, t tenant.Context, filter CorporatePageFilter) ([]CorporateOfferSummary, error) {
	query := tenant.Query{
		OrganizationID:     t.OrganizationID,
		PermittedBranchIDs: t.PermittedBranchIDs,
		Limit:              filter.boundedLimit(),
		CursorSortValue:    filter.CursorSortValue,
		CursorLastID:       filter.CursorLastID,
	}
	status := filter.Status
	if status == "" {
		status = models.ProjectionOffered
	}
	projections, err := s.opts.Projections.List(ctx, query, status)
	if err != nil {
		return nil, err
	}

	out := make([]CorporateOfferSummary, 0, len(projections))
	for i := range projections {
		projection := projections[i]
		offer, offerErr := s.opts.Offers.GetLiveByProjection(ctx, nil, t.OrganizationID, projection.ID)
		if offerErr != nil {
			if errors.Is(offerErr, repositories.ErrNotFound) {
				continue
			}
			return nil, offerErr
		}
		out = append(out, summarizeCorporateOffer(offer, &projection))
	}
	return out, nil
}

// GetCorporateOffer returns one offer with its acceptance, if any.
func (s *Service) GetCorporateOffer(ctx context.Context, t tenant.Context, offerID string) (*CorporateOfferDetail, error) {
	offer, err := s.opts.Offers.GetByID(ctx, t.OrganizationID, offerID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Offer not found.", nil)
		}
		return nil, err
	}
	projection, err := s.opts.Projections.GetByID(ctx, t.OrganizationID, offer.RequestProjectionID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Request not found.", nil)
		}
		return nil, err
	}
	detail := &CorporateOfferDetail{Offer: summarizeCorporateOffer(offer, projection)}
	if acceptance, aErr := s.opts.Offers.GetAcceptance(ctx, t.OrganizationID, offer.ID); aErr == nil {
		detail.Acceptance = &CorporateOfferAcceptanceView{
			AcceptanceID:        acceptance.ID,
			AcceptanceSource:    acceptance.AcceptanceSource,
			SourceReservationID: acceptance.SourceReservationID,
			AcceptedAt:          acceptance.AcceptedAt.UTC().Format(time.RFC3339),
		}
	} else if !errors.Is(aErr, repositories.ErrNotFound) {
		return nil, aErr
	}
	return detail, nil
}

// GetCorporateOfferMetrics returns the four counters for the offers list.
func (s *Service) GetCorporateOfferMetrics(ctx context.Context, t tenant.Context) (*CorporateOfferMetrics, error) {
	if s.opts.Pool == nil {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Marketplace metrics are unavailable.")
	}
	metrics := &CorporateOfferMetrics{}

	if err := s.opts.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND status = 'OFFERED'`,
		t.OrganizationID).Scan(&metrics.OpenOffers); err != nil {
		return nil, fmt.Errorf("marketplace: count open offers: %w", err)
	}
	if err := s.opts.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1 AND status = 'OFFERED'
		  AND expires_at > now() AND expires_at < now() + interval '20 minutes'`,
		t.OrganizationID).Scan(&metrics.ExpiringSoon); err != nil {
		return nil, fmt.Errorf("marketplace: count expiring soon: %w", err)
	}
	if err := s.opts.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_organization_acceptances
		WHERE organization_id = $1 AND accepted_at >= date_trunc('day', now())`,
		t.OrganizationID).Scan(&metrics.AcceptedToday); err != nil {
		return nil, fmt.Errorf("marketplace: count accepted today: %w", err)
	}
	if err := s.opts.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_organization_request_offers
		WHERE organization_id = $1
		  AND status = 'DECLINED'
		  AND declined_at >= date_trunc('day', now())`,
		t.OrganizationID).Scan(&metrics.DeclinedToday); err != nil {
		return nil, fmt.Errorf("marketplace: count declined today: %w", err)
	}
	return metrics, nil
}

// ListCorporateCandidates returns eligible memberships for a request.
func (s *Service) ListCorporateCandidates(ctx context.Context, t tenant.Context, projectionID string) ([]CorporateCandidate, error) {
	if s.opts.Pool == nil {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Marketplace candidates are unavailable.")
	}
	projection, err := s.opts.Projections.GetByID(ctx, t.OrganizationID, projectionID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Request not found.", nil)
		}
		return nil, err
	}

	rows, err := s.opts.Pool.Query(ctx, `
		SELECT id, COALESCE(provider_id, ''), provider_service, service_category, status
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1 AND status = 'ACTIVE'
		ORDER BY created_at DESC`, t.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list candidate memberships: %w", err)
	}
	defer rows.Close()

	out := make([]CorporateCandidate, 0, 16)
	for rows.Next() {
		var c CorporateCandidate
		if err := rows.Scan(&c.MembershipID, &c.ProviderID, &c.SourceService,
			&c.ServiceCategory, &c.Status); err != nil {
			return nil, fmt.Errorf("marketplace: scan candidate: %w", err)
		}
		if c.ServiceCategory != projection.SourceService {
			continue
		}
		eligible, reason := s.probeCorporateCandidate(ctx, projection, c)
		c.Eligible = eligible
		c.Reason = reason
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) probeCorporateCandidate(ctx context.Context, projection *models.RequestProjection, c CorporateCandidate) (bool, string) {
	if s.opts.Eligibility == nil {
		return true, ""
	}
	_, decision, err := s.opts.Eligibility.CheckMembership(ctx, MembershipInput{
		OrganizationID:  projection.OrganizationID,
		MembershipID:    c.MembershipID,
		ServiceCategory: projection.SourceService,
		SourceRequestID: projection.PlatformRequestID,
	})
	if err != nil {
		return false, "eligibility_unavailable"
	}
	if !decision.Eligible {
		return false, decision.ReasonCode
	}
	return true, ""
}

// ListCorporateAssignments returns the organization's assignments.
func (s *Service) ListCorporateAssignments(ctx context.Context, t tenant.Context, filter CorporatePageFilter) ([]models.Assignment, error) {
	query := tenant.Query{
		OrganizationID:     t.OrganizationID,
		PermittedBranchIDs: t.PermittedBranchIDs,
		Limit:              filter.boundedLimit(),
		CursorSortValue:    filter.CursorSortValue,
		CursorLastID:       filter.CursorLastID,
	}
	return s.opts.Assignments.List(ctx, query, filter.Status)
}

// GetCorporateAssignment returns one assignment.
func (s *Service) GetCorporateAssignment(ctx context.Context, t tenant.Context, assignmentID string) (*models.Assignment, error) {
	assignment, err := s.opts.Assignments.GetByID(ctx, t.OrganizationID, assignmentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Assignment not found.", nil)
		}
		return nil, err
	}
	return assignment, nil
}

// ListCorporateAssignmentHistory returns the append-only state history.
func (s *Service) ListCorporateAssignmentHistory(ctx context.Context, t tenant.Context, assignmentID string) ([]models.StateHistoryEntry, error) {
	if _, err := s.GetCorporateAssignment(ctx, t, assignmentID); err != nil {
		return nil, err
	}
	return s.opts.Assignments.ListHistory(ctx, t.OrganizationID, assignmentID)
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// AcceptCorporateOffer commits the organization to an open offer.
func (s *Service) AcceptCorporateOffer(ctx context.Context, t tenant.Context, input CorporateAcceptOfferInput) (*models.Offer, error) {
	if s.opts.OperatingModes == nil {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Marketplace operating modes are unavailable.")
	}
	offer, err := s.opts.Offers.GetByID(ctx, t.OrganizationID, input.OfferID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Offer not found.", nil)
		}
		return nil, err
	}
	return s.AcceptForOrganization(ctx, s.opts.OperatingModes, AcceptForOrganizationInput{
		Tenant:          t,
		ProjectionID:    offer.RequestProjectionID,
		ExpectedVersion: input.ExpectedVersion,
		IdempotencyKey:  input.IdempotencyKey,
	})
}

// DeclineCorporateOffer refuses an offer on behalf of the organization.
func (s *Service) DeclineCorporateOffer(ctx context.Context, t tenant.Context, input CorporateDeclineOfferInput) error {
	offer, err := s.opts.Offers.GetByID(ctx, t.OrganizationID, input.OfferID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return apperrors.NotFound("Offer not found.", nil)
		}
		return err
	}
	return s.DeclineForOrganization(ctx, DeclineForOrganizationInput{
		Tenant:       t,
		ProjectionID: offer.RequestProjectionID,
		ReasonCode:   input.ReasonCode,
		Note:         input.Note,
	})
}

// AssignCorporateRequest creates a PENDING_PROVIDER_RESPONSE assignment.
func (s *Service) AssignCorporateRequest(ctx context.Context, t tenant.Context, input CorporateAssignInput) (*models.Assignment, error) {
	return s.Assign(ctx, AssignInput{
		Tenant:                 t,
		ProjectionID:           input.ProjectionID,
		MembershipID:           input.MembershipID,
		AssetReference:         input.AssetReference,
		ResponseTimeoutSeconds: input.ResponseTimeoutSeconds,
		ExpectedRequestVersion: input.ExpectedRequestVersion,
		IdempotencyKey:         input.IdempotencyKey,
	})
}

// CancelCorporateAssignment cancels an assignment.
func (s *Service) CancelCorporateAssignment(ctx context.Context, t tenant.Context, input CorporateCancelAssignmentInput) error {
	return s.Cancel(ctx, CancelInput{
		Tenant:          t,
		AssignmentID:    input.AssignmentID,
		Reason:          input.Reason,
		ExpectedVersion: input.ExpectedVersion,
	})
}

// ReassignCorporateAssignment replaces an assignment.
func (s *Service) ReassignCorporateAssignment(ctx context.Context, t tenant.Context, input CorporateReassignAssignmentInput) (*models.Assignment, error) {
	return s.Reassign(ctx, ReassignInput{
		Tenant:                 t,
		AssignmentID:           input.AssignmentID,
		MembershipID:           input.MembershipID,
		AssetReference:         input.AssetReference,
		ResponseTimeoutSeconds: input.ResponseTimeoutSeconds,
		Reason:                 input.Reason,
		IdempotencyKey:         input.IdempotencyKey,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func summarizeCorporateOffer(offer *models.Offer, projection *models.RequestProjection) CorporateOfferSummary {
	out := CorporateOfferSummary{
		OfferID:             offer.ID,
		RequestProjectionID: offer.RequestProjectionID,
		Status:              offer.Status,
		OperatingMode:       offer.OperatingMode,
		OfferedAt:           offer.OfferedAt.UTC().Format(time.RFC3339),
		ExpiresAt:           offer.ExpiresAt.UTC().Format(time.RFC3339),
		Version:             offer.Version,
	}
	if projection != nil {
		out.SourceService = projection.SourceService
		out.ServiceType = projection.ServiceType
		out.PickupSummary = corporateOrEmptyMap(projection.PickupSummary)
		out.DropoffSummary = corporateOrEmptyMap(projection.DropoffSummary)
		out.OperationalRequirements = corporateOrEmptyMap(projection.OperationalRequirements)
	}
	return out
}

func corporateOrEmptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}