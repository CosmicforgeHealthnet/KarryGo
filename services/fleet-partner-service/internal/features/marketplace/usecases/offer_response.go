package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// OfferResponseInput is a provider answering an organization marketplace
// offer, relayed by the execution service that authenticated them.
type OfferResponseInput struct {
	Actor                  ProviderActor
	OfferID                string
	ClientResponseID       string
	ExpectedVersion        int64
	ReasonCode             string
	Note                   string
	EvidenceFileIDs        []string
	DispatchReservationID  string
	DispatchRequestVersion int64
	RequestID              string
	CorrelationID          string
}

// OfferResponseResult is the stable answer returned to the owning service.
type OfferResponseResult struct {
	OfferID        string
	RecipientID    string
	Status         string
	Assignment     *models.Assignment
	AllowedActions []string
	Replayed       bool
}

// AcceptOffer records a provider's direct acceptance of an organization offer.
//
// This is the DELEGATED path. The provider has already been authenticated by
// their owning service and has already won the request at the source, so this
// commits the whole organization-side consequence in ONE transaction:
// organization acceptance, the provider assignment, the immutable provider
// response, and the durable execution-start work.
//
// It must never ask the provider to accept a second time. They already did.
func (s *Service) AcceptOffer(ctx context.Context, input OfferResponseInput) (*OfferResponseResult, error) {
	if input.ClientResponseID == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"A client response identifier is required.")
	}

	// A retrying relay replays the original outcome rather than colliding with
	// its own earlier delivery.
	if replay, err := s.findOfferReplay(ctx, input); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	var result OfferResponseResult
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		// Lock ORDER matters, and it is projection-then-recipient everywhere.
		// Acceptance updates the accepting provider's recipient row AND every
		// other recipient (they lose the race). If one transaction took its own
		// recipient first and then the projection, while another took the
		// projection first, two providers racing the same offer would deadlock.
		// Taking the coarser projection lock first serialises the whole race.
		offer, err := s.loadOffer(ctx, input.OfferID)
		if err != nil {
			return err
		}
		projection, err := s.opts.Projections.LockByID(ctx, tx, offer.OrganizationID,
			offer.RequestProjectionID)
		if err != nil {
			return err
		}

		recipient, err := s.lockOwnedRecipient(ctx, tx, input, offer)
		if err != nil {
			return err
		}
		if !recipient.Actionable(s.opts.Now()) {
			return errorcodes.Conflict(errorcodes.MarketplaceOfferAlreadyResolved,
				"This offer is no longer open to you.")
		}
		if input.ExpectedVersion != 0 && input.ExpectedVersion != recipient.Version {
			return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
				"This offer changed. Reload and try again.")
		}

		// 1. The provider's own recipient row is answered.
		if err := s.opts.Offers.SetRecipientStatus(ctx, tx, recipient.ID,
			models.RecipientAccepted, recipient.Version, input.ClientResponseID, "", ""); err != nil {
			return mapTransitionError(err)
		}
		// Everyone else's offer loses the race. They are informed, not blamed:
		// losing a race is not a decline and must not be recorded as one.
		if err := s.opts.Offers.MarkLosersOfRace(ctx, tx, offer.ID, recipient.ID); err != nil {
			return err
		}

		// 2. Organization acceptance, sourced from the provider rather than a
		// dispatcher. The two acceptance sources stay distinguishable forever.
		acceptance := &models.Acceptance{
			OfferID:                offer.ID,
			AcceptedByMembershipID: &recipient.MembershipID,
			AcceptanceSource:       models.AcceptanceProviderDirect,
			SourceReservationID:    input.DispatchReservationID,
			SourceIdempotencyKey:   input.ClientResponseID,
		}
		if err := s.opts.Offers.CreateAcceptance(ctx, tx, offer.OrganizationID, acceptance); err != nil {
			return errorcodes.Conflict(errorcodes.MarketplaceOfferAlreadyResolved,
				"This offer has already been accepted.")
		}
		if err := s.opts.Offers.SetStatus(ctx, tx, offer.OrganizationID, offer.ID,
			models.OfferAccepted, offer.Version, "PROVIDER", recipient.MembershipID, "", ""); err != nil {
			return mapTransitionError(err)
		}

		// 3. The provider assignment. It is created ALREADY accepted: the
		// provider's decision is the reason this assignment exists, so
		// presenting it as pending would invite a second acceptance.
		assignment := &models.Assignment{
			RequestProjectionID: projection.ID,
			OfferID:             &offer.ID,
			MembershipID:        recipient.MembershipID,
			ExecutingProviderID: input.Actor.ExecutingProviderID,
			SourceService:       projection.SourceService,
			// The provider assigned themselves by accepting the offer. This
			// reuses the schema's existing actor vocabulary rather than
			// widening a CHECK constraint; acceptance_source on the
			// organization acceptance is what records that this was the
			// delegated path.
			AssignedByType:   "PROVIDER",
			AssignedByID:     recipient.MembershipID,
			Status:           models.AssignmentCreated,
			ResponseDeadline: s.opts.Now().Add(s.opts.ResponseTimeoutDefault),
		}
		if err := s.opts.Assignments.Create(ctx, tx, offer.OrganizationID, assignment); err != nil {
			if errors.Is(err, repositories.ErrAssignmentConflict) {
				return errorcodes.Conflict(errorcodes.AssignmentConflict,
					"This request already has a live assignment.")
			}
			return err
		}
		if err := s.opts.Assignments.HoldReservation(ctx, tx, offer.OrganizationID,
			assignment.ID, models.ResourceProvider, recipient.MembershipID); err != nil {
			return errorcodes.Conflict(errorcodes.ProviderHasConflictingWork,
				"That provider is already committed to other work.")
		}
		if err := s.opts.Assignments.Transition(ctx, tx, assignment,
			models.AssignmentPendingProviderResponse, assignment.Version,
			models.StateHistoryEntry{ActorType: "PROVIDER", ActorID: recipient.MembershipID}); err != nil {
			return err
		}
		// Transition validates against the struct it is given and does not
		// mutate it, so advance the in-memory copy before the next hop.
		assignment.Status = models.AssignmentPendingProviderResponse
		assignment.Version++

		// 4. The immutable provider response.
		response := &models.Response{
			OrganizationID:   offer.OrganizationID,
			AssignmentID:     assignment.ID,
			MembershipID:     recipient.MembershipID,
			Response:         models.ResponseAccepted,
			ClientResponseID: input.ClientResponseID,
			OccurredAt:       s.opts.Now(),
			RequestID:        input.RequestID,
			CorrelationID:    input.CorrelationID,
		}
		if err := s.opts.Assignments.CreateResponse(ctx, tx, response); err != nil {
			if errors.Is(err, repositories.ErrResponseExists) {
				return errorcodes.Conflict(errorcodes.AssignmentAlreadyResolved,
					"This assignment already has a provider response.")
			}
			return err
		}
		if err := s.opts.Assignments.Transition(ctx, tx, assignment, models.AssignmentAccepted,
			assignment.Version, models.StateHistoryEntry{
				ActorType: "PROVIDER", ActorID: recipient.MembershipID,
				RequestID: input.RequestID, CorrelationID: input.CorrelationID,
			}); err != nil {
			return err
		}

		// 5. Durable execution-start work, in this same transaction.
		if s.opts.ExecutionStarts != nil {
			if err := s.opts.ExecutionStarts.Create(ctx, tx, &models.ExecutionStartOperation{
				OrganizationID:      offer.OrganizationID,
				AssignmentID:        assignment.ID,
				ProviderResponseID:  response.ID,
				RequestProjectionID: projection.ID,
				SourceService:       projection.SourceService,
				IdempotencyKey:      "exec-" + assignment.ID,
				RequestFingerprint:  fingerprint(assignment.ID, response.ID, assignment.ExecutingProviderID),
			}); err != nil {
				return err
			}
		}

		if err := s.opts.Projections.SetStatus(ctx, tx, offer.OrganizationID, projection.ID,
			models.ProjectionProviderAccepted, projection.Version); err != nil {
			return err
		}

		entry := audit.Entry{
			OrganizationID: offer.OrganizationID,
			ActorType:      "PROVIDER", ActorID: recipient.MembershipID,
			Action: "offer.provider_accepted", ResourceType: "offer", ResourceID: offer.ID,
			NewValues: map[string]any{"assignmentId": assignment.ID},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if err := s.opts.Events.Publish(ctx, tx, "organization.offer.provider_accepted.v1", "offer",
			offer.ID, offer.Version+1, offer.OrganizationID, input.CorrelationID,
			map[string]any{
				"offerId":       offer.ID,
				"assignmentId":  assignment.ID,
				"membershipId":  recipient.MembershipID,
				"sourceService": projection.SourceService,
			}); err != nil {
			return err
		}

		assignment.Status = models.AssignmentAccepted
		result = OfferResponseResult{
			OfferID: offer.ID, RecipientID: recipient.ID,
			Status: models.RecipientAccepted, Assignment: assignment,
			// The provider has answered; no action remains for them here.
			AllowedActions: []string{},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_offer_provider_accepts_total", nil, 1)
	return &result, nil
}

// DeclineOffer records a provider's refusal of an organization offer.
//
// It touches ONLY that provider's recipient row. The organization's offer
// stays open for every other eligible provider: one provider's "no" is not the
// organization's "no", and must never be recorded as one.
//
// This path consults no feature flag. Accepting work can be disabled; refusing
// work already offered cannot.
func (s *Service) DeclineOffer(ctx context.Context, input OfferResponseInput) (*OfferResponseResult, error) {
	if input.ReasonCode == "" {
		return nil, errorcodes.BadRequest(errorcodes.DeclineReasonRequired,
			"A decline reason is required.")
	}
	if !models.ValidDeclineReason(input.ReasonCode) {
		return nil, errorcodes.BadRequest(errorcodes.DeclineReasonInvalid,
			"That decline reason is not recognised.")
	}
	if models.RequiresNote(input.ReasonCode) && input.Note == "" {
		return nil, errorcodes.BadRequest(errorcodes.DeclineNoteRequired,
			"Please explain the reason for declining.")
	}
	if len(input.Note) > models.MaxDeclineNoteLength {
		return nil, errorcodes.BadRequest(errorcodes.DeclineReasonInvalid,
			"The explanation is too long.")
	}

	var result OfferResponseResult
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		offer, err := s.loadOffer(ctx, input.OfferID)
		if err != nil {
			return err
		}
		recipient, err := s.lockOwnedRecipient(ctx, tx, input, offer)
		if err != nil {
			return err
		}
		if !recipient.Actionable(s.opts.Now()) {
			return errorcodes.Conflict(errorcodes.MarketplaceOfferAlreadyResolved,
				"This offer is no longer open to you.")
		}
		if input.ExpectedVersion != 0 && input.ExpectedVersion != recipient.Version {
			return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
				"This offer changed. Reload and try again.")
		}

		if err := s.opts.Offers.SetRecipientStatus(ctx, tx, recipient.ID,
			models.RecipientDeclined, recipient.Version, input.ClientResponseID,
			input.ReasonCode, input.Note); err != nil {
			return mapTransitionError(err)
		}

		// The organization's offer is deliberately NOT touched here.

		entry := audit.Entry{
			OrganizationID: offer.OrganizationID,
			ActorType:      "PROVIDER", ActorID: recipient.MembershipID,
			Action: "offer.provider_declined", ResourceType: "offer_recipient",
			ResourceID: recipient.ID,
			// The reason is recorded for operations, never as a sanction.
			Reason:    input.ReasonCode,
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if err := s.opts.Events.Publish(ctx, tx, "organization.offer.provider_declined.v1",
			"offer_recipient", recipient.ID, recipient.Version+1, offer.OrganizationID,
			input.CorrelationID, map[string]any{
				"offerId":      offer.ID,
				"recipientId":  recipient.ID,
				"membershipId": recipient.MembershipID,
				"reasonCode":   input.ReasonCode,
				// Safety reasons are flagged so downstream consumers treat them
				// as welfare signals, never as performance failures.
				"safetyProtected": models.SafetyProtected(input.ReasonCode),
			}); err != nil {
			return err
		}

		result = OfferResponseResult{
			OfferID: offer.ID, RecipientID: recipient.ID,
			Status: models.RecipientDeclined, AllowedActions: []string{},
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_offer_provider_declines_total",
		map[string]string{"category": declineCategory(input.ReasonCode)}, 1)
	return &result, nil
}

// lockOwnedRecipient loads the provider's own recipient row under lock.
//
// Ownership is proved in SQL, not in Go: LockRecipientForMembership matches on
// both the recipient id and the membership, so a valid service signature can
// never be used to answer another provider's offer.
func (s *Service) lockOwnedRecipient(ctx context.Context, tx pgx.Tx, input OfferResponseInput,
	offer *models.Offer) (*models.OfferRecipient, error) {
	recipient, err := s.opts.Offers.LockRecipientForProvider(ctx, tx, input.OfferID,
		input.Actor.ExecutingProviderID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.Forbidden(errorcodes.AssignmentNotOwnedByProvider,
				"This offer was not made to that provider.")
		}
		return nil, err
	}
	return recipient, nil
}

// loadOffer reads the offer without a tenant predicate.
//
// The caller is the owning execution service, which holds the offer id from
// its own relay and does not know the organization. The organization is
// returned for the caller to bind against, never accepted as input.
func (s *Service) loadOffer(ctx context.Context, offerID string) (*models.Offer, error) {
	offer, err := s.opts.Offers.GetByIDUnscoped(ctx, offerID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.NotFound(errorcodes.MarketplaceOfferNotFound, "Offer not found.")
		}
		return nil, err
	}
	return offer, nil
}

// findOfferReplay returns the original outcome for a redelivered decision.
func (s *Service) findOfferReplay(ctx context.Context, input OfferResponseInput) (*OfferResponseResult, error) {
	recipient, err := s.opts.Offers.FindRecipientByClientResponse(ctx, input.OfferID, input.ClientResponseID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &OfferResponseResult{
		OfferID: input.OfferID, RecipientID: recipient.ID,
		Status: recipient.Status, AllowedActions: []string{}, Replayed: true,
	}, nil
}

// OfferExpiryDeadline reports when an offer recipient stops being actionable.
func OfferExpiryDeadline(recipient models.OfferRecipient) time.Time { return recipient.ExpiresAt }
