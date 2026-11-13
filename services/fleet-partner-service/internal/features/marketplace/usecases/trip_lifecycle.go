package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 11.2 / 12.2: the assignment state machine ends
// READY_FOR_EXECUTION -> IN_PROGRESS -> COMPLETED, and those last two moves
// belong to the canonical trip the execution service runs. Fleet never
// decides that work started or finished; it mirrors what the trip projection
// (fed only by source events and read-back) says.

// tripNotStartedStatuses are source trip statuses that do not yet mean the
// provider is working the job.
var tripNotStartedStatuses = map[string]bool{
	"": true, "assigned": true, "accepted": true, "created": true,
	"pending": true, "scheduled": true, "reserved": true,
}

type tripPhase int

const (
	tripPhaseNotStarted tripPhase = iota
	tripPhaseRunning
	tripPhaseCompleted
	tripPhaseCancelled
)

// classifyTrip reads the phase from the projection. Terminal facts come only
// from the source-supplied completion and cancellation timestamps.
func classifyTrip(trip *models.TripProjection) tripPhase {
	switch {
	case trip.CompletedAt != nil:
		return tripPhaseCompleted
	case trip.CancelledAt != nil:
		return tripPhaseCancelled
	case tripNotStartedStatuses[strings.ToLower(strings.TrimSpace(trip.TripStatus))]:
		return tripPhaseNotStarted
	default:
		return tripPhaseRunning
	}
}

// advanceAssignmentFromTrip moves the trip's assignment to match the trip.
//
// Runs inside the caller's transaction (event ingestion or the sync sweep).
// It is deliberately conservative:
//   - an untrusted trip projection (stale, gapped, reconciling) moves nothing;
//   - an assignment whose execution start has not been confirmed yet
//     (ACCEPTED, EXECUTION_START_PENDING) is left to the execution-start
//     worker, which owns that commit point — the sync sweep catches it up as
//     soon as it reaches READY_FOR_EXECUTION;
//   - the assignment must belong to the trip's organization (tenant
//     revalidation); a mismatch is corrupt data and is never "repaired".
func (s *Service) advanceAssignmentFromTrip(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection, causationID string) (bool, error) {
	if trip == nil || trip.AssignmentID == nil || *trip.AssignmentID == "" || s.opts.Assignments == nil {
		return false, nil
	}
	if blocksCapacityRelease(trip.FreshnessState) {
		return false, nil
	}
	assignment, err := s.opts.Assignments.LockByID(ctx, tx, *trip.AssignmentID)
	if errors.Is(err, repositories.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if assignment.OrganizationID != trip.OrganizationID {
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_trip_assignment_tenant_mismatch_total", nil, 1)
		return false, nil
	}

	var path []string
	switch classifyTrip(trip) {
	case tripPhaseRunning:
		switch assignment.Status {
		case models.AssignmentReadyForExecution, models.AssignmentReconciliationRequired:
			path = []string{models.AssignmentInProgress}
		}
	case tripPhaseCompleted:
		switch assignment.Status {
		case models.AssignmentReadyForExecution, models.AssignmentReconciliationRequired:
			path = []string{models.AssignmentInProgress, models.AssignmentCompleted}
		case models.AssignmentInProgress:
			path = []string{models.AssignmentCompleted}
		}
	case tripPhaseCancelled:
		switch assignment.Status {
		case models.AssignmentReadyForExecution, models.AssignmentReconciliationRequired,
			models.AssignmentInProgress:
			path = []string{models.AssignmentCancelled}
		}
	}
	if len(path) == 0 {
		return false, nil
	}

	for _, next := range path {
		from := assignment.Status
		if err := s.opts.Assignments.Transition(ctx, tx, assignment, next, assignment.Version,
			models.StateHistoryEntry{
				ActorType: "SYSTEM", ActorID: "trip-lifecycle",
				ReasonCode: stringPointer("source_trip_" + strings.ToLower(next)),
				Metadata: map[string]any{
					"platformTripId": trip.PlatformTripID,
					"tripStatus":     trip.TripStatus,
					"sourceVersion":  trip.SourceVersion,
				},
				CorrelationID: causationID,
			}); err != nil {
			if errors.Is(err, concurrency.ErrVersionMismatch) || errors.Is(err, repositories.ErrInvalidTransition) {
				// Someone else moved the assignment first; the sweep re-reads.
				return false, nil
			}
			return false, err
		}
		assignment.Status = next
		assignment.Version++

		if err := s.recordTripDrivenTransition(ctx, tx, assignment, trip, from, next, causationID); err != nil {
			return false, err
		}
	}

	if assignment.Status == models.AssignmentCompleted || assignment.Status == models.AssignmentCancelled {
		// Capacity is released only on a source-confirmed terminal outcome.
		if err := s.opts.Assignments.ReleaseReservations(ctx, tx, assignment.ID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// recordTripDrivenTransition writes the audit record and outbox event for one
// trip-driven assignment move. The completion event carries the canonical
// references downstream finance needs to find the settlement (trip, executing
// provider, settlement organization) and no money: the amount is owned by the
// execution service and payment-wallet-service.
func (s *Service) recordTripDrivenTransition(ctx context.Context, tx pgx.Tx,
	assignment *models.Assignment, trip *models.TripProjection, from string, to string,
	causationID string) error {
	action, eventType := "", ""
	switch to {
	case models.AssignmentInProgress:
		action, eventType = "assignment.started", "organization.assignment.started.v1"
	case models.AssignmentCompleted:
		action, eventType = "assignment.completed", "organization.assignment.completed.v1"
	case models.AssignmentCancelled:
		action, eventType = "assignment.cancelled", "organization.assignment.cancelled.v1"
	default:
		return nil
	}
	settlementOrganizationID := ""
	if trip.SettlementOrganizationID != nil {
		settlementOrganizationID = *trip.SettlementOrganizationID
	}
	references := map[string]any{
		"assignmentId":             assignment.ID,
		"requestProjectionId":      assignment.RequestProjectionID,
		"membershipId":             assignment.MembershipID,
		"sourceService":            trip.SourceService,
		"platformTripId":           trip.PlatformTripID,
		"tripProjectionId":         trip.ID,
		"executingProviderId":      trip.ExecutingProviderID,
		"settlementOrganizationId": settlementOrganizationID,
		"tripStatus":               trip.TripStatus,
	}
	if trip.CompletedAt != nil {
		references["completedAt"] = trip.CompletedAt.UTC()
	}
	if trip.CancelledAt != nil {
		references["cancelledAt"] = trip.CancelledAt.UTC()
	}
	if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
		OrganizationID: assignment.OrganizationID,
		ActorType:      "system",
		ActorID:        "trip-lifecycle",
		Action:         action,
		ResourceType:   "assignment",
		ResourceID:     assignment.ID,
		PreviousValues: map[string]any{"status": from},
		NewValues:      mergeMaps(map[string]any{"status": to}, references),
		CorrelationID:  causationID,
		CausationID:    causationID,
	}); err != nil {
		return err
	}
	if s.opts.Events == nil {
		return nil
	}
	return s.opts.Events.Publish(ctx, tx, eventType, "assignment", assignment.ID,
		assignment.Version, assignment.OrganizationID, causationID, references)
}

func stringPointer(value string) *string { return &value }

// SyncAssignmentsFromTrips is the recovery sweep for trip-driven assignment
// progress: it finds assignments their trip has moved past and advances them,
// one short transaction each. Safe to run from several processes at once —
// each assignment is row-locked and every transition is a compare-and-set.
func (s *Service) SyncAssignmentsFromTrips(ctx context.Context, limit int) (int, error) {
	if s.opts.Trips == nil || s.opts.Assignments == nil {
		return 0, nil
	}
	links, err := s.opts.Trips.ListAssignmentsBehindTrips(ctx, limit)
	if err != nil {
		return 0, err
	}
	advanced := 0
	for _, link := range links {
		var moved bool
		err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			trip, err := s.opts.Trips.LockTripByID(ctx, tx, link.OrganizationID, link.TripID)
			if errors.Is(err, repositories.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			moved, err = s.advanceAssignmentFromTrip(ctx, tx, trip, "trip-sync:"+trip.ID)
			return err
		})
		if err != nil {
			return advanced, err
		}
		if moved {
			advanced++
		}
	}
	if advanced > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_marketplace_trip_assignment_sync_total", nil, float64(advanced))
	}
	return advanced, nil
}

// ---------------------------------------------------------------------------
// Organization trip lifecycle reads (spec 12.3 "Trip lifecycle: consume and
// display projection"). Distinct from /tracking, which is the live position
// projection (latitude, heading, speed) fed by telemetry: this is the
// canonical lifecycle — status, history, outcome and the references that tie
// a trip to its request, assignment, provider and settlement organization.
// ---------------------------------------------------------------------------

// CorporateTripFilter narrows the trip list.
type CorporateTripFilter struct {
	Status        string
	SourceService string
	MembershipID  string
	Limit         int
	Offset        int
}

// CorporateTripView is the organization-facing trip lifecycle projection.
// Built from an explicit field list; it carries no fare, commission or
// settlement amount.
type CorporateTripView struct {
	TripID                   string     `json:"tripId"`
	SourceService            string     `json:"sourceService"`
	PlatformTripID           string     `json:"platformTripId"`
	PlatformRequestID        *string    `json:"platformRequestId,omitempty"`
	PlatformBookingID        *string    `json:"platformBookingId,omitempty"`
	AssignmentID             *string    `json:"assignmentId,omitempty"`
	OrganizationMembershipID *string    `json:"organizationMembershipId,omitempty"`
	ExecutingProviderID      string     `json:"executingProviderId"`
	SettlementOrganizationID *string    `json:"settlementOrganizationId,omitempty"`
	ServiceType              string     `json:"serviceType"`
	TripStatus               string     `json:"tripStatus"`
	SourceVersion            int64      `json:"sourceVersion"`
	FreshnessState           string     `json:"freshnessState"`
	LastEventAt              *time.Time `json:"lastEventAt,omitempty"`
	LastReconciledAt         *time.Time `json:"lastReconciledAt,omitempty"`
	CompletedAt              *time.Time `json:"completedAt,omitempty"`
	CancelledAt              *time.Time `json:"cancelledAt,omitempty"`
	Version                  int64      `json:"version"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

// CorporateTripPage is one page of trips.
type CorporateTripPage struct {
	Items      []CorporateTripView `json:"items"`
	HasMore    bool                `json:"hasMore"`
	NextOffset int                 `json:"nextOffset,omitempty"`
}

// CorporateDeliveryOutcomeView is the projected delivery outcome.
type CorporateDeliveryOutcomeView struct {
	RecipientConfirmedAt *time.Time `json:"recipientConfirmedAt,omitempty"`
	ConfirmationMethod   string     `json:"confirmationMethod,omitempty"`
	RecipientMatched     bool       `json:"recipientMatchedExpected"`
	FailedAt             *time.Time `json:"failedAt,omitempty"`
	FailureReasonCode    string     `json:"failureReasonCode,omitempty"`
	FailureCategory      string     `json:"failureReasonCategory,omitempty"`
	RetryEligible        bool       `json:"retryEligible"`
}

// CorporateTripDetail is one trip with its history and outcome.
type CorporateTripDetail struct {
	Trip            CorporateTripView                 `json:"trip"`
	History         []repositories.TripHistoryEntry   `json:"history"`
	DeliveryOutcome *CorporateDeliveryOutcomeView     `json:"deliveryOutcome,omitempty"`
	Assignment      *CorporateTripAssignmentReference `json:"assignment,omitempty"`
}

// CorporateTripAssignmentReference is the assignment a trip executes.
type CorporateTripAssignmentReference struct {
	AssignmentID string     `json:"assignmentId"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

func tripView(trip *models.TripProjection) CorporateTripView {
	return CorporateTripView{
		TripID:                   trip.ID,
		SourceService:            trip.SourceService,
		PlatformTripID:           trip.PlatformTripID,
		PlatformRequestID:        trip.PlatformRequestID,
		PlatformBookingID:        trip.PlatformBookingID,
		AssignmentID:             trip.AssignmentID,
		OrganizationMembershipID: trip.OrganizationMembershipID,
		ExecutingProviderID:      trip.ExecutingProviderID,
		SettlementOrganizationID: trip.SettlementOrganizationID,
		ServiceType:              trip.ServiceType,
		TripStatus:               trip.TripStatus,
		SourceVersion:            trip.SourceVersion,
		FreshnessState:           trip.FreshnessState,
		LastEventAt:              trip.LastEventAt,
		LastReconciledAt:         trip.LastReconciledAt,
		CompletedAt:              trip.CompletedAt,
		CancelledAt:              trip.CancelledAt,
		Version:                  trip.Version,
		CreatedAt:                trip.CreatedAt,
		UpdatedAt:                trip.UpdatedAt,
	}
}

// ListCorporateTrips returns the organization's trip lifecycle projections.
func (s *Service) ListCorporateTrips(ctx context.Context, t tenant.Context,
	filter CorporateTripFilter) (*CorporateTripPage, error) {
	if s.opts.Trips == nil {
		return nil, apperrors.Unavailable("Trip projections are not available.", nil)
	}
	source := ""
	if filter.SourceService != "" {
		source = SourceCategory(strings.ToUpper(filter.SourceService))
		if source == "" {
			return nil, apperrors.Validation("Unknown source service.",
				[]apperrors.FieldViolation{{Field: "sourceService", Message: "Use DISPATCH, TAXI or HAULING."}})
		}
	}
	trips, hasMore, err := s.opts.Trips.ListPage(ctx, tenant.Query{
		OrganizationID: t.OrganizationID, PermittedBranchIDs: t.PermittedBranchIDs,
	}, repositories.TripPageFilter{
		Status: filter.Status, SourceService: source, MembershipID: filter.MembershipID,
		Limit: filter.Limit, Offset: filter.Offset,
	})
	if err != nil {
		return nil, err
	}
	page := &CorporateTripPage{Items: make([]CorporateTripView, 0, len(trips)), HasMore: hasMore}
	for index := range trips {
		page.Items = append(page.Items, tripView(&trips[index]))
	}
	if hasMore {
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}
		page.NextOffset = offset + len(trips)
	}
	return page, nil
}

// GetCorporateTrip returns one trip with history, delivery outcome and its
// assignment. Another organization's trip, or one outside the caller's
// branch scope, is indistinguishable from a trip that does not exist.
func (s *Service) GetCorporateTrip(ctx context.Context, t tenant.Context,
	tripID string) (*CorporateTripDetail, error) {
	if s.opts.Trips == nil {
		return nil, apperrors.Unavailable("Trip projections are not available.", nil)
	}
	trip, err := s.opts.Trips.GetScoped(ctx, tenant.Query{
		OrganizationID: t.OrganizationID, PermittedBranchIDs: t.PermittedBranchIDs,
	}, tripID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	history, err := s.opts.Trips.ListTripHistory(ctx, t.OrganizationID, trip.ID, 200)
	if err != nil {
		return nil, err
	}
	detail := &CorporateTripDetail{Trip: tripView(trip), History: history}

	outcome, err := s.opts.Trips.GetDeliveryOutcome(ctx, t.OrganizationID, trip.ID)
	if err != nil && !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	if outcome != nil && (outcome.Confirmation != nil || outcome.Failure != nil) {
		view := &CorporateDeliveryOutcomeView{}
		if outcome.Confirmation != nil {
			confirmedAt := outcome.Confirmation.ConfirmedAt
			view.RecipientConfirmedAt = &confirmedAt
			view.ConfirmationMethod = outcome.Confirmation.Method
			view.RecipientMatched = outcome.Confirmation.MatchedExpected
		}
		if outcome.Failure != nil {
			failedAt := outcome.Failure.FailedAt
			view.FailedAt = &failedAt
			view.FailureReasonCode = outcome.Failure.ReasonCode
			view.FailureCategory = outcome.Failure.ReasonCategory
			view.RetryEligible = outcome.Failure.RetryEligible
		}
		detail.DeliveryOutcome = view
	}

	if trip.AssignmentID != nil && *trip.AssignmentID != "" && s.opts.Assignments != nil {
		assignment, err := s.opts.Assignments.GetByID(ctx, t.OrganizationID, *trip.AssignmentID)
		if err == nil {
			detail.Assignment = &CorporateTripAssignmentReference{
				AssignmentID: assignment.ID, Status: assignment.Status,
				StartedAt: assignment.StartedAt, CompletedAt: assignment.CompletedAt,
			}
		} else if !errors.Is(err, repositories.ErrNotFound) {
			return nil, err
		}
	}
	return detail, nil
}
