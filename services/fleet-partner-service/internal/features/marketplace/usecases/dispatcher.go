package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// OperatingModeReader resolves an organization's marketplace operating mode.
type OperatingModeReader interface {
	OperatingMode(ctx context.Context, organizationID string) (string, error)
}

// AcceptForOrganizationInput is a dispatcher committing the organization.
type AcceptForOrganizationInput struct {
	Tenant          tenant.Context
	ProjectionID    string
	ExpectedVersion int64
	IdempotencyKey  string
}

// AcceptForOrganization reserves a marketplace request for the organization.
//
// Dispatcher acceptance is NOT provider acceptance: it only wins the request
// for the organization. A provider must still be assigned and must still
// accept before any work becomes executable.
func (s *Service) AcceptForOrganization(ctx context.Context, modes OperatingModeReader, input AcceptForOrganizationInput) (*models.Offer, error) {
	if !s.opts.Flags.OrganizationAcceptEnabled {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Marketplace acceptance is not enabled.")
	}
	if input.IdempotencyKey == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}

	mode, err := modes.OperatingMode(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	if !models.AllowsDispatcherAcceptance(mode) {
		return nil, errorcodes.Conflict(errorcodes.MarketplaceModeNotAllowed,
			"Your organization's operating mode does not allow dispatcher acceptance.")
	}

	projection, err := s.opts.Projections.GetByID(ctx, input.Tenant.OrganizationID, input.ProjectionID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Request not found.", nil)
		}
		return nil, err
	}
	if err := s.assertFreshEnough(projection); err != nil {
		return nil, err
	}
	if projection.ProjectionStatus != models.ProjectionOffered {
		return nil, errorcodes.Conflict(errorcodes.RequestNotAssignable,
			"This request is no longer open.")
	}

	offer, err := s.opts.Offers.GetLiveByProjection(ctx, nil, input.Tenant.OrganizationID, projection.ID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.New(404, errorcodes.MarketplaceOfferNotFound, "Offer not found.", nil)
		}
		return nil, err
	}
	if !offer.Actionable(s.opts.Now()) {
		return nil, errorcodes.Conflict(errorcodes.MarketplaceOfferExpired,
			"This opportunity is no longer available.")
	}

	// The source service performs the canonical reservation and decides the
	// single winner. Nothing is recorded locally until it confirms.
	outcome, err := s.reserveAtSource(ctx, projection, platformclients.ReserveRequest{
		SourceRequestID:       projection.PlatformRequestID,
		ExpectedSourceVersion: projection.SourceVersion,
		OrganizationID:        input.Tenant.OrganizationID,
		AcceptanceSource:      models.AcceptanceDispatcher,
		IdempotencyKey:        input.IdempotencyKey,
		CorrelationID:         input.Tenant.CorrelationID,
	}, offer)
	if err != nil {
		return nil, err
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		locked, err := s.opts.Offers.LockByID(ctx, tx, input.Tenant.OrganizationID, offer.ID)
		if err != nil {
			return err
		}
		if locked.Status != models.OfferOffered {
			return errorcodes.Conflict(errorcodes.MarketplaceOfferCancelled,
				"This opportunity was already resolved.")
		}
		if err := s.opts.Offers.SetStatus(ctx, tx, input.Tenant.OrganizationID, offer.ID,
			models.OfferAccepted, locked.Version, "ORGANIZATION_USER",
			input.Tenant.OrganizationUserID, "", ""); err != nil {
			return err
		}
		if err := s.opts.Offers.SetSourceReservation(ctx, tx, offer.ID, outcome.ReservationID); err != nil {
			return err
		}

		userID := input.Tenant.OrganizationUserID
		if err := s.opts.Offers.CreateAcceptance(ctx, tx, input.Tenant.OrganizationID, &models.Acceptance{
			OfferID:              offer.ID,
			AcceptedByUserID:     &userID,
			AcceptanceSource:     models.AcceptanceDispatcher,
			SourceReservationID:  outcome.ReservationID,
			SourceIdempotencyKey: input.IdempotencyKey,
			Snapshot: map[string]any{
				"platformRequestId": projection.PlatformRequestID,
				"sourceVersion":     projection.SourceVersion,
			},
			RequestID:     input.Tenant.RequestID,
			CorrelationID: input.Tenant.CorrelationID,
		}); err != nil {
			return err
		}

		locked2, err := s.opts.Projections.LockByID(ctx, tx, input.Tenant.OrganizationID, projection.ID)
		if err != nil {
			return err
		}
		if err := s.opts.Projections.SetStatus(ctx, tx, input.Tenant.OrganizationID, projection.ID,
			models.ProjectionOrganizationAccepted, locked2.Version); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "marketplace.accepted_for_organization",
			"marketplace_request", projection.ID)
		entry.NewValues = map[string]any{"offerId": offer.ID, "reservationId": outcome.ReservationID}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.request.accepted.v1", "marketplace_request",
			projection.ID, locked2.Version+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"requestProjectionId": projection.ID,
				"offerId":             offer.ID,
				"sourceService":       projection.SourceService,
				"acceptanceSource":    models.AcceptanceDispatcher,
			})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_marketplace_offer_accepts_total",
		map[string]string{"source": projection.SourceService, "actor": "dispatcher"}, 1)
	return s.opts.Offers.GetByID(ctx, input.Tenant.OrganizationID, offer.ID)
}

// reserveAtSource performs the canonical reservation with a durable command
// record, translating a lost race into a definite conflict and a timeout into
// a reconciliation marker.
func (s *Service) reserveAtSource(ctx context.Context, projection *models.RequestProjection, request platformclients.ReserveRequest, offer *models.Offer) (platformclients.ReservationOutcome, error) {
	if !s.opts.Flags.SourceEnabled(projection.SourceService) {
		return platformclients.ReservationOutcome{}, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Live acceptance for this service is not enabled yet.")
	}
	contract, ok := s.opts.Executions.For(projection.SourceService)
	if !ok || !contract.Configured() {
		return platformclients.ReservationOutcome{}, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"The execution service is not available.")
	}

	command := &models.CommandOperation{
		OrganizationID:  projection.OrganizationID,
		OperationType:   models.CommandReserveRequest,
		SourceService:   projection.SourceService,
		SourceRequestID: projection.PlatformRequestID,
		IdempotencyKey:  request.IdempotencyKey,
		RequestFingerprint: fingerprint(projection.PlatformRequestID, projection.OrganizationID,
			request.AcceptanceSource, request.AcceptingProviderID),
	}
	var operation *models.CommandOperation
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		stored, replayed, err := s.opts.Commands.Begin(ctx, tx, command)
		if errors.Is(err, repositories.ErrIdempotencyConflict) {
			return errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
				"This idempotency key was already used with a different request.")
		}
		if err != nil {
			return err
		}
		_ = replayed
		operation = stored
		return nil
	}); err != nil {
		return platformclients.ReservationOutcome{}, err
	}

	// A confirmed reservation is never re-sent.
	if operation.Status == models.CommandConfirmed {
		return platformclients.ReservationOutcome{
			ReservationID: operation.ExternalReferenceOrEmpty(),
			Status:        platformclients.ReservationReserved,
		}, nil
	}

	start := s.opts.Now()
	outcome, err := contract.ReserveForOrganization(ctx,
		platformclients.CallMeta{RequestID: request.CorrelationID, CorrelationID: request.CorrelationID}, request)
	s.opts.Metrics.ObserveDuration("fleet_partner_source_command_duration",
		map[string]string{"operation": models.CommandReserveRequest}, s.opts.Now().Sub(start))

	if err != nil {
		clientErr, ok := err.(*platformclients.Error)
		if ok && clientErr.Outcome == platformclients.OutcomeConflict {
			// A definite loss, not an unknown: another organization won.
			s.recordCommand(ctx, operation, models.CommandRejected, "", string(errorcodes.RequestAlreadyClaimed))
			s.opts.Metrics.IncCounter("fleet_partner_marketplace_acceptance_conflicts_total", nil, 1)
			return platformclients.ReservationOutcome{}, errorcodes.Conflict(errorcodes.RequestAlreadyClaimed,
				"This request has already been taken.")
		}
		// Unknown outcome: mark for read-back and tell the caller honestly.
		s.recordCommand(ctx, operation, models.CommandUnknownOutcome, "", "reservation_unknown_outcome")
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			locked, lockErr := s.opts.Offers.LockByID(ctx, tx, projection.OrganizationID, offer.ID)
			if lockErr != nil {
				return nil
			}
			return s.opts.Offers.SetStatus(ctx, tx, projection.OrganizationID, offer.ID,
				models.OfferReserved, locked.Version, "", "", "", "")
		})
		s.opts.Metrics.IncCounter("fleet_partner_source_unknown_outcomes_total",
			map[string]string{"operation": models.CommandReserveRequest}, 1)
		return platformclients.ReservationOutcome{}, errorcodes.Unavailable(errorcodes.SourceOutcomeUnknown,
			"We could not confirm this acceptance. It is being checked and will resolve shortly.")
	}

	if outcome.Status != platformclients.ReservationReserved {
		s.recordCommand(ctx, operation, models.CommandRejected, "", outcome.Status)
		switch outcome.Status {
		case platformclients.ReservationAlreadyClaimed:
			s.opts.Metrics.IncCounter("fleet_partner_marketplace_acceptance_conflicts_total", nil, 1)
			return platformclients.ReservationOutcome{}, errorcodes.Conflict(errorcodes.RequestAlreadyClaimed,
				"This request has already been taken.")
		case platformclients.ReservationVersionMismatch:
			return platformclients.ReservationOutcome{}, errorcodes.Conflict(errorcodes.SourceVersionMismatch,
				"This request has changed. Refresh and try again.")
		default:
			return platformclients.ReservationOutcome{}, errorcodes.Conflict(errorcodes.RequestNotAssignable,
				"This request is no longer available.")
		}
	}

	s.recordCommand(ctx, operation, models.CommandConfirmed, outcome.ReservationID, "")
	s.opts.Metrics.IncCounter("fleet_partner_marketplace_offer_reservations_total",
		map[string]string{"source": projection.SourceService}, 1)
	return outcome, nil
}

// DeclineForOrganizationInput declines an opportunity for the whole company.
type DeclineForOrganizationInput struct {
	Tenant       tenant.Context
	ProjectionID string
	ReasonCode   string
	Note         string
}

// DeclineForOrganization explicitly refuses an opportunity on behalf of the
// organization. It is distinct from one provider declining a delegated offer.
func (s *Service) DeclineForOrganization(ctx context.Context, input DeclineForOrganizationInput) error {
	if !models.ValidOrganizationDeclineReason(input.ReasonCode) {
		return errorcodes.BadRequest(errorcodes.DeclineReasonInvalid,
			"That decline reason is not recognised.")
	}

	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		projection, err := s.opts.Projections.LockByID(ctx, tx, input.Tenant.OrganizationID, input.ProjectionID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return apperrors.NotFound("Request not found.", nil)
			}
			return err
		}
		offer, err := s.opts.Offers.GetLiveByProjection(ctx, tx, input.Tenant.OrganizationID, projection.ID)
		if err != nil {
			return errorcodes.New(404, errorcodes.MarketplaceOfferNotFound, "Offer not found.", nil)
		}
		if offer.Status != models.OfferOffered {
			return errorcodes.Conflict(errorcodes.MarketplaceOfferCancelled,
				"This opportunity was already resolved.")
		}

		if err := s.opts.Offers.SetStatus(ctx, tx, input.Tenant.OrganizationID, offer.ID,
			models.OfferDeclined, offer.Version, "ORGANIZATION_USER",
			input.Tenant.OrganizationUserID, input.ReasonCode, input.Note); err != nil {
			return err
		}
		// Every pending provider recipient is closed: the organization has
		// refused, so no provider should still see an actionable offer.
		if err := s.opts.Offers.MarkLosersOfRace(ctx, tx, offer.ID, ""); err != nil {
			return err
		}
		if err := s.opts.Projections.SetStatus(ctx, tx, input.Tenant.OrganizationID, projection.ID,
			models.ProjectionOrganizationDeclined, projection.Version); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "marketplace.declined_for_organization",
			"marketplace_request", projection.ID)
		entry.Reason = input.ReasonCode
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_offer_declines_total",
			map[string]string{"actor": "organization"}, 1)
		return s.opts.Events.Publish(ctx, tx, "organization.request.declined.v1", "marketplace_request",
			projection.ID, projection.Version+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"requestProjectionId": projection.ID,
				"offerId":             offer.ID,
				"reasonCode":          input.ReasonCode,
			})
	})
}

func (s *Service) assertFreshEnough(projection *models.RequestProjection) error {
	if models.BlocksStateChangingCommands(projection.FreshnessState) {
		return errorcodes.Conflict(errorcodes.MarketplaceProjectionGap,
			"This request's status is being re-synchronised. Try again shortly.")
	}
	if projection.FreshnessState == models.FreshnessStale {
		return errorcodes.Conflict(errorcodes.MarketplaceProjectionStale,
			"This request's information is out of date. Refresh and try again.")
	}
	return nil
}

var _ = time.Now
