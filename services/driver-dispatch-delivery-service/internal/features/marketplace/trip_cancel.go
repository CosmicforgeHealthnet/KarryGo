package marketplace

import (
	"context"
	"net/http"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

// TripCanceller is the trip feature's organization-cancel authority. It is a
// narrow port so the marketplace package does not import the trip package;
// the router adapts trip.Service.CancelOrganizationTrip to it.
type TripCanceller interface {
	CancelOrganizationTrip(ctx context.Context, tripID, reason, correlationID string) error
}

// WithTripCanceller installs the organization trip-cancel authority. Without
// one, the cancel route fails closed.
func (s *Service) WithTripCanceller(canceller TripCanceller) *Service {
	s.tripCanceller = canceller
	return s
}

// CancelOrganizationTripInput is Fleet's cancellation of an
// organization-managed canonical trip.
type CancelOrganizationTripInput struct {
	TripID         string
	OrganizationID string
	Reason         string
	IdempotencyKey string
	CorrelationID  string
}

// CancelOrganizationTrip cancels a canonical trip on behalf of the
// organization that holds it. The caller must name the organization the trip
// belongs to: a trip of another organization, or an independent trip, is
// reported as not found, so one organization can never cancel another's work
// or a consumer booking. Repeating a cancellation of an already cancelled
// trip returns the trip unchanged, which makes Fleet's retries and read-back
// safe. The trip feature decides which states may still be cancelled (before
// pickup); later states are a conflict, never a local override.
func (s *Service) CancelOrganizationTrip(ctx context.Context, in CancelOrganizationTripInput) (*TripView, error) {
	if !s.flags.InternalAPIEnabled {
		return nil, apperrors.Unavailable("The internal marketplace API is not enabled.", nil)
	}
	in.TripID = strings.TrimSpace(in.TripID)
	in.OrganizationID = strings.TrimSpace(in.OrganizationID)
	if in.TripID == "" || in.OrganizationID == "" {
		return nil, apperrors.BadRequest("tripId and organizationId are required.", nil)
	}
	if s.tripCanceller == nil {
		return nil, apperrors.Unavailable("Organization trip cancellation is not available.", nil)
	}
	trip, err := s.ReadTrip(ctx, in.TripID)
	if err != nil {
		return nil, err
	}
	if trip.OrganizationID == nil || *trip.OrganizationID != in.OrganizationID {
		return nil, errCoded(http.StatusNotFound, "execution_not_found", "Trip not found.")
	}
	if strings.EqualFold(trip.Status, "cancelled") {
		return trip, nil
	}
	correlation := in.CorrelationID
	if correlation == "" {
		correlation = in.IdempotencyKey
	}
	if err := s.tripCanceller.CancelOrganizationTrip(ctx, in.TripID, in.Reason, correlation); err != nil {
		return nil, err
	}
	return s.ReadTrip(ctx, in.TripID)
}

type cancelTripBody struct {
	OrganizationID string `json:"organizationId"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
	CorrelationID  string `json:"correlationId"`
}

// CancelTrip handles POST /trips/:tripId/cancel.
func (h *Handler) CancelTrip(c *gin.Context) {
	var body cancelTripBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if body.IdempotencyKey == "" {
		body.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	result, err := h.service.CancelOrganizationTrip(c.Request.Context(), CancelOrganizationTripInput{
		TripID: c.Param("tripId"), OrganizationID: body.OrganizationID, Reason: body.Reason,
		IdempotencyKey: body.IdempotencyKey, CorrelationID: body.CorrelationID,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusOK, result)
}
