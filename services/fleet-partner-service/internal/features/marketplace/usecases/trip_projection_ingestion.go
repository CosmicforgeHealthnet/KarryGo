package usecases

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
)

// tripAggregateEvents are the canonical events whose version sequence belongs
// to the TRIP aggregate rather than the request.
//
// The distinction is not cosmetic. A request's source version counts events
// against one broadcast; a trip's counts events against one trip. They are
// independent sequences that both start low and both advance by one. Feeding
// one into the other's counter — which is what this service did before — makes
// every trip event look like either a stale replay or a version jump, so
// projections would oscillate between ignored updates and spurious
// GAP_DETECTED. Routing on the aggregate is what makes contiguity meaningful.
var tripAggregateEvents = map[string]bool{
	EventTripCreated:            true,
	EventTripStatusChanged:      true,
	EventTripCompleted:          true,
	EventTripCancelled:          true,
	EventProofCompleted:         true,
	EventTripRecipientConfirmed: true,
	EventTripDeliveryFailed:     true,
}

// blocksCapacityRelease reports whether a projection is too uncertain to
// justify releasing capacity or triggering a financial consequence.
//
// Stricter than models.BlocksStateChangingCommands, which admits STALE. A
// stale projection is fine to read and even to advance, but it is not adequate
// grounds for standing down an offer or settling money: silence from the
// source is equally consistent with delivery having stopped, and the two
// readings have opposite consequences. Kept separate rather than widening the
// existing helper, whose callers deliberately tolerate staleness.
func blocksCapacityRelease(freshness string) bool {
	return freshness == models.FreshnessGapDetected ||
		freshness == models.FreshnessUnknown ||
		freshness == models.FreshnessStale ||
		freshness == models.FreshnessReconciling
}

// applyTripEvent projects one trip-aggregate event.
//
// Runs inside the ingestion transaction, so the trip projection, its history,
// the associated request projection and any reconciliation case all commit
// together or not at all.
func (s *Service) applyTripEvent(ctx context.Context, tx pgx.Tx,
	event InboundEvent) (repositories.SourceEventOutcome, error) {
	if s.opts.Trips == nil {
		// Refusing is the only safe answer: reporting success would have the
		// producer mark the event delivered and never send it again, losing it
		// permanently against a deployment that cannot store it.
		return "", &IngestError{
			Code:    "trip_projection_unavailable",
			Message: "Trip projections are not available in this deployment.",
			Status:  http.StatusInternalServerError,
		}
	}

	sourceEvent := s.buildTripSourceEvent(event)
	if sourceEvent.PlatformTripID == "" {
		return "", codedBadRequest("invalid_envelope",
			"Trip events must identify the canonical trip.")
	}

	// The organization comes from the request projection this trip belongs to,
	// never from the event body. A producer must not be able to attribute a
	// trip to an organization by asserting it.
	if err := s.resolveTripOrganization(ctx, tx, event, &sourceEvent); err != nil {
		return "", err
	}

	outcome, trip, err := s.opts.Trips.ApplyTripSourceEvent(ctx, tx, sourceEvent)
	if err != nil {
		return "", err
	}

	// The request association is read back from the projection rather than
	// from this event. Only trip.created.v1 carries requestId; every later
	// event would otherwise appear unassociated, and completion would silently
	// stop reaching the request projection.
	requestProjection, err := s.associatedRequest(ctx, tx, trip)
	if err != nil {
		return "", err
	}

	switch outcome {
	case repositories.TripEventStale:
		return repositories.SourceEventStale, nil

	case repositories.TripEventDuplicate:
		return repositories.SourceEventStale, nil

	case repositories.TripEventConflict:
		// Two different payloads at one version. The projection is now marked
		// GAP_DETECTED; a read-back decides which is true. Reported as applied
		// rather than rejected because the event was legitimately delivered —
		// the contradiction is the source's, not the transport's.
		if err := s.openTripReconciliation(ctx, tx, trip, "source_version_conflict"); err != nil {
			return "", err
		}
		return repositories.SourceEventGap, nil
	}

	if err := s.recordTripHistory(ctx, tx, trip, event, sourceEvent); err != nil {
		return "", err
	}

	// The delivery-outcome columns are written after the version rules have
	// already decided this event should be applied, so a stale or duplicate
	// delivery event can never reach them.
	if err := s.applyDeliveryOutcome(ctx, tx, trip, event); err != nil {
		return "", err
	}

	if outcome == repositories.TripEventGap {
		if err := s.openTripReconciliation(ctx, tx, trip, "trip_version_gap"); err != nil {
			return "", err
		}
	}

	if err := s.applyTripToRequestProjection(ctx, tx, requestProjection, trip, event); err != nil {
		return "", err
	}

	if outcome == repositories.TripEventGap {
		return repositories.SourceEventGap, nil
	}
	return repositories.SourceEventApplied, nil
}

// buildTripSourceEvent maps an envelope onto the projection's input.
//
// Every field is read explicitly by name. Nothing is copied wholesale from the
// payload, so a producer that starts including fare, customer contact or
// settlement amounts cannot have them land in fleet's tables by accident.
func (s *Service) buildTripSourceEvent(event InboundEvent) repositories.TripSourceEvent {
	sourceEvent := repositories.TripSourceEvent{
		SourceService:            event.SourceService,
		PlatformTripID:           payloadString(event.Payload, "tripId"),
		PlatformRequestID:        sourceRequestID(event),
		PlatformBookingID:        payloadString(event.Payload, "bookingId"),
		ExecutingProviderID:      payloadString(event.Payload, "executingProviderId"),
		OrganizationMembershipID: payloadString(event.Payload, "organizationMembershipId"),
		SettlementOrganizationID: payloadString(event.Payload, "settlementOrganizationId"),
		AssignmentID:             payloadString(event.Payload, "fleetAssignmentId"),
		ServiceType:              payloadString(event.Payload, "serviceType"),
		TripStatus:               payloadString(event.Payload, "status"),
		SourceVersion:            event.AggregateVersion,
		EventID:                  event.EventID,
		PayloadHash:              event.PayloadHash,
		OccurredAt:               event.OccurredAt,
	}
	if sourceEvent.PlatformTripID == "" && event.AggregateType == "TRIP" {
		sourceEvent.PlatformTripID = event.AggregateID
	}
	if sourceEvent.ExecutingProviderID == "" {
		sourceEvent.ExecutingProviderID = payloadString(event.Payload, "providerId")
	}

	// Terminal timestamps are only ever taken from the source payload. A trip
	// is completed because the execution service says so, never because fleet
	// saw a completion-shaped event arrive.
	switch event.EventType {
	case EventTripCompleted:
		if at := payloadTime(event.Payload, "completedAt"); at != nil {
			sourceEvent.CompletedAt = at
		} else {
			occurred := event.OccurredAt
			sourceEvent.CompletedAt = &occurred
		}
	case EventTripCancelled:
		if at := payloadTime(event.Payload, "cancelledAt"); at != nil {
			sourceEvent.CancelledAt = at
		} else {
			occurred := event.OccurredAt
			sourceEvent.CancelledAt = &occurred
		}
	}
	return sourceEvent
}

// resolveTripOrganization finds the request projection that owns this trip.
//
// Returns nil when no request projection exists. That is not fatal for a trip
// projection whose row already exists — later events legitimately arrive after
// a request projection has been archived — but a trip that has never been seen
// and cannot be attributed to an organization is refused, because inventing a
// tenant is the one mistake that cannot be walked back.
func (s *Service) resolveTripOrganization(ctx context.Context, tx pgx.Tx,
	event InboundEvent, sourceEvent *repositories.TripSourceEvent) error {
	// An existing trip projection is the strongest answer: fleet already
	// decided this trip's tenant, and a later event must not be able to move
	// it by naming a different request.
	existing, err := s.opts.Trips.GetBySourceTripAnyOrganization(ctx, tx,
		event.SourceService, sourceEvent.PlatformTripID)
	if err == nil {
		sourceEvent.OrganizationID = existing.OrganizationID
		if sourceEvent.PlatformRequestID == "" && existing.PlatformRequestID != nil {
			sourceEvent.PlatformRequestID = *existing.PlatformRequestID
		}
		return nil
	}
	if !errors.Is(err, repositories.ErrNotFound) {
		return err
	}

	if sourceEvent.PlatformRequestID != "" {
		projection, findErr := s.opts.Projections.FindBySourceRequestAnyOrganization(ctx, tx,
			event.SourceService, sourceEvent.PlatformRequestID)
		if findErr == nil {
			sourceEvent.OrganizationID = projection.OrganizationID
			return nil
		}
		if !errors.Is(findErr, repositories.ErrNotFound) {
			return findErr
		}
	}

	// Neither a trip projection nor a request projection: nothing ties this
	// trip to an organization this service manages. Inventing a tenant is the
	// one mistake that cannot be walked back, so the event is ignored.
	return repositories.ErrNotFound
}

// associatedRequest loads the request projection a trip belongs to.
//
// Uses the association fleet stored when the trip was created, so it survives
// later events that do not repeat requestId. Returns nil when the trip has no
// request association or the projection has since gone — neither is an error,
// because a trip projection is useful on its own.
func (s *Service) associatedRequest(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection) (*models.RequestProjection, error) {
	if trip == nil || trip.PlatformRequestID == nil || *trip.PlatformRequestID == "" {
		return nil, nil
	}
	projection, err := s.opts.Projections.FindBySourceRequestAnyOrganization(ctx, tx,
		trip.SourceService, *trip.PlatformRequestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return projection, nil
}

// recordTripHistory appends the append-only status transition.
func (s *Service) recordTripHistory(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection, event InboundEvent,
	sourceEvent repositories.TripSourceEvent) error {
	from := payloadString(event.Payload, "fromStatus")
	to := trip.TripStatus
	if event.EventType == EventProofCompleted {
		// Proof does not change trip status; the history row records that
		// evidence landed at this version, which is what an operator
		// reconstructing the trip needs to see.
		from, to = trip.TripStatus, trip.TripStatus
	}
	return s.opts.Trips.AppendHistory(ctx, tx, trip.OrganizationID, trip.ID,
		from, to, sourceEvent.SourceVersion, event.EventID)
}

// applyTripToRequestProjection mirrors trip outcomes onto the request.
//
// The request projection's own source_version is deliberately NOT advanced
// here: it counts request-aggregate events and this is a trip-aggregate one.
// Only the derived status changes, through the unversioned setter.
func (s *Service) applyTripToRequestProjection(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, trip *models.TripProjection,
	event InboundEvent) error {
	if projection == nil {
		return nil
	}
	projections := s.opts.Projections

	switch event.EventType {
	case EventTripCreated:
		if trip.PlatformTripID != "" {
			if err := projections.SetPlatformTripID(ctx, tx, projection.ID,
				trip.PlatformTripID); err != nil {
				return err
			}
		}
		return projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionExecutionCreated)

	case EventTripStatusChanged:
		return projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionInProgress)

	case EventTripCompleted:
		if blocksCapacityRelease(trip.FreshnessState) {
			// The trip says completed but the projection is not trustworthy.
			// Recording completion here would be the record downstream
			// settlement reads, so it waits for reconciliation.
			return s.openTripReconciliation(ctx, tx, trip, "completion_on_untrusted_projection")
		}
		return projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionCompleted)

	case EventTripCancelled:
		if blocksCapacityRelease(trip.FreshnessState) {
			// Cancelling open work frees the provider and the vehicle. Doing
			// that on an untrusted projection risks releasing capacity for a
			// trip that is in fact still running.
			return s.openTripReconciliation(ctx, tx, trip, "cancellation_on_untrusted_projection")
		}
		if err := projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
			models.ProjectionCancelled); err != nil {
			return err
		}
		return s.cancelOpenWork(ctx, tx, projection, "source_trip_cancelled")

	case EventProofCompleted:
		// Proof is evidence, not a lifecycle transition. It never advances the
		// request projection and never releases anything.
		return nil

	case EventTripRecipientConfirmed:
		// Confirmation is a fact about the delivery, not a lifecycle move. It
		// never advances the request projection, never releases capacity and
		// never implies completion — the trip is completed when dispatch says
		// trip.completed.v1 and not a moment earlier.
		return nil

	case EventTripDeliveryFailed:
		return s.applyDeliveryFailureToRequest(ctx, tx, projection, trip)
	}
	return nil
}

// applyDeliveryFailureToRequest projects a failed delivery onto the request.
//
// Five things this must not do, each of which would be a plausible
// simplification and each of which would be wrong:
//
//   - it must not release capacity. cancelOpenWork is deliberately NOT called:
//     a failed delivery may be re-attempted, and standing down the offer would
//     free a provider who is still holding the package.
//   - it must not mark the request COMPLETED, which is what downstream
//     settlement reads.
//   - it must not mark it CANCELLED. Cancellation is a different fact with
//     different consequences, and only dispatch may say a trip was cancelled.
//   - it must not authorize a refund or reversal. Nothing here touches money,
//     and the failure event carries no monetary field to be tempted by.
//   - it must not overwrite a newer completion or cancellation. A trip that
//     already carries one of those and now claims to have failed is dispatch
//     contradicting itself; that is a reconciliation case, not a status
//     update.
func (s *Service) applyDeliveryFailureToRequest(ctx context.Context, tx pgx.Tx,
	projection *models.RequestProjection, trip *models.TripProjection) error {
	if trip != nil && (trip.CompletedAt != nil || trip.CancelledAt != nil) {
		return s.openTripReconciliation(ctx, tx, trip, "failure_after_terminal_outcome")
	}
	if trip != nil && blocksCapacityRelease(trip.FreshnessState) {
		// The trip says failed but the projection is not trustworthy. Recording
		// the failure here would be the record an operator acts on, so it waits
		// for reconciliation.
		return s.openTripReconciliation(ctx, tx, trip, "failure_on_untrusted_projection")
	}
	switch projection.ProjectionStatus {
	case models.ProjectionCompleted, models.ProjectionCancelled, models.ProjectionExpired:
		// A newer terminal outcome already stands. Failure does not displace it.
		return nil
	}
	return s.opts.Projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID,
		models.ProjectionDeliveryFailed)
}

// applyDeliveryOutcome writes the confirmation and failure projection columns.
//
// Values are read explicitly by name from the payload. Nothing is copied
// wholesale, so a producer that starts including recipient contact details
// cannot have them land in fleet's tables by accident.
func (s *Service) applyDeliveryOutcome(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection, event InboundEvent) error {
	if trip == nil {
		return nil
	}
	switch event.EventType {
	case EventTripRecipientConfirmed:
		confirmedAt := payloadTime(event.Payload, "confirmedAt")
		if confirmedAt == nil {
			occurred := event.OccurredAt
			confirmedAt = &occurred
		}
		return s.opts.Trips.ApplyRecipientConfirmation(ctx, tx, trip.ID,
			repositories.RecipientConfirmationProjection{
				ConfirmedAt:        *confirmedAt,
				Method:             payloadString(event.Payload, "verificationMethod"),
				RecipientReference: payloadString(event.Payload, "recipientReference"),
				MatchedExpected:    payloadBool(event.Payload, "recipientMatchedExpected"),
			})

	case EventTripDeliveryFailed:
		failedAt := payloadTime(event.Payload, "failedAt")
		if failedAt == nil {
			occurred := event.OccurredAt
			failedAt = &occurred
		}
		return s.opts.Trips.ApplyDeliveryFailure(ctx, tx, trip.ID,
			repositories.DeliveryFailureProjection{
				FailedAt:             *failedAt,
				ReasonCode:           payloadString(event.Payload, "reasonCode"),
				ReasonCategory:       payloadString(event.Payload, "reasonCategory"),
				RetryEligible:        payloadBool(event.Payload, "retryEligible"),
				RecipientUnavailable: payloadBool(event.Payload, "recipientUnavailable"),
				IncidentReference:    payloadString(event.Payload, "incidentReference"),
			})
	}
	return nil
}

// payloadBool reads a boolean field, tolerating absence.
//
// Absence is false rather than an error: a producer that omits retryEligible
// has not said a failure may be re-attempted, and the safe reading of silence
// is "no".
func payloadBool(payload map[string]any, key string) bool {
	value, ok := payload[key].(bool)
	return ok && value
}

// openTripReconciliation records a durable repair case for a trip.
//
// Idempotent against the partial unique index on live cases: a second gap for
// an aggregate already awaiting repair adds nothing.
func (s *Service) openTripReconciliation(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection, reason string) error {
	if trip == nil {
		return nil
	}
	err := s.opts.Trips.OpenReconciliation(ctx, tx, trip.OrganizationID, trip.SourceService,
		models.ReconciliationAggregateTrip, trip.PlatformTripID, reason, time.Now().UTC())
	if errors.Is(err, repositories.ErrDuplicateCase) {
		return nil
	}
	return err
}

// payloadString reads a string field, tolerating absence.
func payloadString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}

// payloadTime reads an RFC3339 timestamp, tolerating absence.
func payloadTime(payload map[string]any, key string) *time.Time {
	raw, ok := payload[key].(string)
	if !ok || raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return &parsed
}
