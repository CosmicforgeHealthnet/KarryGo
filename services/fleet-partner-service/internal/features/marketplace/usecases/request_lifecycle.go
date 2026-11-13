package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
)

// claimedSourceStatuses are source request statuses meaning someone outside
// every organization (an independent provider) has taken the request.
var claimedSourceStatuses = map[string]bool{"accepted": true, "assigned": true, "matched": true}

// applyRequestUpdate keeps an organization's copy in step with a source
// update: a later broadcast window extends a still-open offer, and a request
// an independent provider took is stood down for every organization that had
// not yet committed to it.
func (s *Service) applyRequestUpdate(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, event InboundEvent) error {
	if projection.ProjectionStatus != models.ProjectionOffered {
		// An organization that already accepted owns its own progress; a
		// source update about the open pool says nothing about it.
		return nil
	}
	status := strings.ToLower(payloadString(event.Payload, "status"))
	if claimedSourceStatuses[status] {
		if err := s.opts.Projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionCancelled); err != nil {
			return err
		}
		return s.cancelOpenWork(ctx, tx, projection, "source_request_claimed")
	}
	expiry := payloadTime(event.Payload, "expiresAt")
	if expiry == nil {
		return nil
	}
	if err := s.opts.Projections.SetExpiresAt(ctx, tx, projection.ID, *expiry); err != nil {
		return err
	}
	offer, err := s.opts.Offers.GetLiveByProjection(ctx, tx, projection.OrganizationID, projection.ID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.opts.Offers.ExtendOpenOffer(ctx, tx, projection.OrganizationID, offer.ID,
		s.offerDeadline(expiry))
	return err
}

// applyRequestReserved projects a source reservation.
//
// The source names the winning organization. The winner's copy records the
// reservation (never regressing a copy that has already moved past OFFERED);
// every other organization's still-open copy is stood down, because only one
// organization acceptance can win a marketplace request (spec 11.7).
func (s *Service) applyRequestReserved(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, event InboundEvent) error {
	winner := payloadString(event.Payload, "organizationId")
	if winner == "" || winner == projection.OrganizationID {
		if projection.ProjectionStatus != models.ProjectionOffered {
			return nil
		}
		return s.opts.Projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionReservationPending)
	}
	if projection.ProjectionStatus != models.ProjectionOffered {
		return nil
	}
	if err := s.opts.Projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
		models.ProjectionCancelled); err != nil {
		return err
	}
	return s.cancelOpenWork(ctx, tx, projection, "source_request_reserved_by_other_organization")
}
