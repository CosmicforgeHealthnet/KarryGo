// Package usecases implements fleet-side incident and dispute reference
// handling.
//
// The single rule everything here serves: fleet may record that a case exists
// and what it is attached to, and may never behave as though it knows what the
// case means. A reference the authoritative service did not confirm is stored
// as an explicitly unverified note and confers no authority at all.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// EventReferenceRecorded is the corporate event emitted when a reference is
// attached.
const EventReferenceRecorded = "organization.safety_reference.recorded.v1"

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, organizationID string,
		correlationID string, payload any) error
}

// Options configure the safety service.
type Options struct {
	Pool       *pgxpool.Pool
	References *repositories.ReferenceRepository
	// Validator confirms references against the authoritative service. A nil
	// validator means this deployment has no support-dispute client, which is
	// treated as "validation unavailable" — not as "validation unnecessary".
	Validator platformclients.SafetyReferenceValidator
	Events    EventPublisher
	Audit     *audit.Recorder
	Metrics   *observability.Metrics
	Now       func() time.Time
}

// Service records and reads fleet-side safety references.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// Actor is the authenticated caller recording a reference.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// RecordReferenceInput attaches an authoritative case to fleet-side work.
type RecordReferenceInput struct {
	Actor               Actor
	ReferenceType       string
	ExternalReference   string
	AssignmentID        string
	PlatformTripID      string
	ExecutingProviderID string
	VehicleReference    string
	IdempotencyKey      string
	CorrelationID       string
	CausationID         string
}

// RecordReferenceResult is what the caller learns.
type RecordReferenceResult struct {
	Reference *models.Reference
	Replayed  bool
}

// RecordReference stores a pointer to an authoritative incident or dispute.
//
// Order of operations, and why:
//  1. shape and tenant ownership are checked first, so a malformed or
//     cross-tenant request never reaches the network;
//  2. validation against the authoritative service happens BEFORE the
//     transaction, because an external call must not hold a database lock;
//  3. the record, its history, the audit row and the outbox event commit
//     together.
//
// The verification outcome is decided entirely in step 2 and carried into the
// row. There is no path that writes VERIFIED without a confirming answer.
func (s *Service) RecordReference(ctx context.Context,
	input RecordReferenceInput) (*RecordReferenceResult, error) {
	if err := s.validateShape(&input); err != nil {
		return nil, err
	}

	requestFingerprint := referenceFingerprint(input)

	// A retry replays; a reused key with different inputs is refused.
	if replay, err := s.findReplay(ctx, input, requestFingerprint); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	// The organization must actually own everything it is attaching to.
	tripProjectionID, err := s.resolveAssociations(ctx, input)
	if err != nil {
		return nil, err
	}

	verification, verifiedAt, err := s.verify(ctx, input)
	if err != nil {
		return nil, err
	}

	reference := &models.Reference{
		OrganizationID:     input.Actor.OrganizationID,
		ReferenceType:      input.ReferenceType,
		AuthorityService:   models.AuthoritySupportDispute,
		ExternalReference:  input.ExternalReference,
		VerificationState:  verification,
		VerifiedAt:         verifiedAt,
		CreatedByActorType: input.Actor.ActorType,
		CreatedByActorID:   input.Actor.ActorID,
		CorrelationID:      input.CorrelationID,
		CausationID:        input.CausationID,
		IdempotencyKey:     input.IdempotencyKey,
		RequestFingerprint: requestFingerprint,
	}
	setOptional(&reference.AssignmentID, input.AssignmentID)
	setOptional(&reference.TripProjectionID, tripProjectionID)
	setOptional(&reference.PlatformTripID, input.PlatformTripID)
	setOptional(&reference.ExecutingProviderID, input.ExecutingProviderID)
	setOptional(&reference.VehicleReference, input.VehicleReference)

	if !reference.HasAssociation() {
		return nil, errorcodes.BadRequest(errorcodes.SafetyReferenceInvalid,
			"A reference must be attached to an assignment, trip or provider.")
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		// Re-check inside the transaction: an existing record for the same
		// case pointing somewhere else is a conflicting reference, and saying
		// so is more useful than a bare unique-violation.
		existing, findErr := s.opts.References.FindByExternalReference(ctx, tx,
			input.Actor.OrganizationID, input.ReferenceType, models.AuthoritySupportDispute,
			input.ExternalReference)
		switch {
		case findErr == nil:
			if conflicts(existing, reference) {
				return errorcodes.Conflict(errorcodes.SafetyReferenceConflict,
					"This case is already recorded against different work.")
			}
			return errorcodes.Conflict(errorcodes.SafetyReferenceConflict,
				"This case is already recorded for this organization.")
		case !errors.Is(findErr, repositories.ErrNotFound):
			return findErr
		}

		if createErr := s.opts.References.Create(ctx, tx,
			input.Actor.OrganizationID, reference); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.SafetyReferenceConflict,
					"This case is already recorded for this organization.")
			}
			return createErr
		}

		if histErr := s.opts.References.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: reference.OrganizationID,
			ReferenceID:    reference.ID,
			Action:         models.ActionCreated,
			ToState:        reference.VerificationState,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Metadata: map[string]any{
				"referenceType":    reference.ReferenceType,
				"authorityService": reference.AuthorityService,
			},
			CorrelationID: input.CorrelationID,
			CausationID:   input.CausationID,
		}); histErr != nil {
			return histErr
		}

		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: reference.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "safety_reference.recorded",
			ResourceType:   "safety_reference",
			ResourceID:     reference.ID,
			NewValues: map[string]any{
				"referenceType":     reference.ReferenceType,
				"verificationState": reference.VerificationState,
				// The external identifier is recorded; no case content is,
				// because fleet never holds any.
				"externalReference": reference.ExternalReference,
			},
			RequestID:     input.CorrelationID,
			CorrelationID: input.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}

		if s.opts.Events == nil {
			return nil
		}
		return s.opts.Events.Publish(ctx, tx, EventReferenceRecorded, "safety_reference",
			reference.ID, reference.Version, reference.OrganizationID, input.CorrelationID,
			map[string]any{
				"referenceId":       reference.ID,
				"organizationId":    reference.OrganizationID,
				"referenceType":     reference.ReferenceType,
				"authorityService":  reference.AuthorityService,
				"externalReference": reference.ExternalReference,
				"verificationState": reference.VerificationState,
				// Stated on the wire so a consumer never has to infer whether
				// this reference may justify a consequence.
				"authorizesConsequences": reference.AuthorizesConsequences(),
				"assignmentId":           stringOrEmpty(reference.AssignmentID),
				"platformTripId":         stringOrEmpty(reference.PlatformTripID),
			})
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_safety_references_total",
		map[string]string{"type": reference.ReferenceType, "verification": reference.VerificationState}, 1)
	return &RecordReferenceResult{Reference: reference}, nil
}

// validateShape rejects malformed input before any I/O.
func (s *Service) validateShape(input *RecordReferenceInput) error {
	input.ReferenceType = strings.ToUpper(strings.TrimSpace(input.ReferenceType))
	input.ExternalReference = strings.TrimSpace(input.ExternalReference)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)

	switch {
	case input.Actor.OrganizationID == "":
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	case !models.ValidReferenceType(input.ReferenceType):
		return errorcodes.BadRequest(errorcodes.SafetyReferenceInvalid,
			"referenceType must be INCIDENT or DISPUTE.")
	case input.ExternalReference == "":
		return errorcodes.BadRequest(errorcodes.SafetyReferenceInvalid,
			"An external case reference is required.")
	case len(input.ExternalReference) > 200:
		return errorcodes.BadRequest(errorcodes.SafetyReferenceInvalid,
			"The external case reference is too long.")
	case input.IdempotencyKey == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An idempotency key is required.")
	}
	return nil
}

// resolveAssociations proves the organization owns what it is attaching to.
//
// This is the tenant boundary for writes. Without it, an operator could point
// their organization's reference at another tenant's assignment id and, from
// then on, every listing that joins the two would be wrong.
func (s *Service) resolveAssociations(ctx context.Context,
	input RecordReferenceInput) (string, error) {
	if input.AssignmentID != "" {
		owned, err := s.opts.References.AssignmentBelongsToOrganization(ctx, nil,
			input.Actor.OrganizationID, input.AssignmentID)
		if err != nil {
			return "", err
		}
		if !owned {
			// Never disclose that the assignment exists elsewhere.
			return "", errorcodes.NotFound(errorcodes.AssignmentNotFound, "Assignment not found.")
		}
	}

	if input.PlatformTripID == "" {
		return "", nil
	}
	// A trip projection may legitimately not exist yet; the canonical trip id
	// is still worth storing, so absence is not an error.
	projectionID, err := s.opts.References.TripProjectionForOrganization(ctx, nil,
		input.Actor.OrganizationID, input.PlatformTripID)
	if errors.Is(err, repositories.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return projectionID, nil
}

// verify asks the authoritative service whether the reference is real.
//
// Three outcomes, each deliberate:
//
//   - the authoritative service confirms it        -> VERIFIED;
//   - no by-identifier endpoint exists for the type -> UNVERIFIED, stored and
//     clearly marked, authorizing nothing;
//   - validation is possible in principle but unavailable, or the service says
//     the case does not exist -> the write is REFUSED.
//
// The third case is what "fail closed" means here. A deployment that cannot
// reach the authority must not quietly downgrade to UNVERIFIED, because that
// would make an unreachable authority indistinguishable from an absent one and
// turn an outage into a way to record whatever you like.
func (s *Service) verify(ctx context.Context, input RecordReferenceInput) (string, *time.Time, error) {
	meta := platformclients.CallMeta{
		CorrelationID:  input.CorrelationID,
		IdempotencyKey: input.IdempotencyKey,
	}

	var check platformclients.ReferenceCheck
	var err error
	switch {
	case s.opts.Validator == nil:
		err = platformclients.ErrNotConfigured
	case input.ReferenceType == models.ReferenceIncident:
		check, err = s.opts.Validator.ValidateIncident(ctx, meta, input.ExternalReference)
	default:
		check, err = s.opts.Validator.ValidateDispute(ctx, meta, input.ExternalReference)
	}

	if errors.Is(err, platformclients.ErrValidationEndpointAbsent) {
		s.opts.Metrics.IncCounter("fleet_partner_safety_reference_unverified_total",
			map[string]string{"type": input.ReferenceType}, 1)
		return models.VerificationUnverified, nil, nil
	}
	if err != nil {
		s.opts.Metrics.IncCounter("fleet_partner_safety_validation_failures_total",
			map[string]string{"type": input.ReferenceType}, 1)
		return "", nil, errorcodes.Unavailable(errorcodes.SafetyValidationUnavailable,
			"The case could not be confirmed right now. Please try again.")
	}
	if !check.Exists {
		return "", nil, errorcodes.BadRequest(errorcodes.SafetyReferenceInvalid,
			"That case reference was not recognised.")
	}

	verifiedAt := s.opts.Now().UTC()
	return models.VerificationVerified, &verifiedAt, nil
}

// findReplay resolves a retried write.
func (s *Service) findReplay(ctx context.Context, input RecordReferenceInput,
	requestFingerprint string) (*RecordReferenceResult, error) {
	existing, err := s.opts.References.FindByIdempotencyKey(ctx, nil,
		input.Actor.OrganizationID, input.IdempotencyKey)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if existing.RequestFingerprint != requestFingerprint {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for a different request.")
	}
	return &RecordReferenceResult{Reference: existing, Replayed: true}, nil
}

// conflicts reports whether an existing record points somewhere else.
func conflicts(existing *models.Reference, proposed *models.Reference) bool {
	return stringOrEmpty(existing.AssignmentID) != stringOrEmpty(proposed.AssignmentID) ||
		stringOrEmpty(existing.PlatformTripID) != stringOrEmpty(proposed.PlatformTripID) ||
		stringOrEmpty(existing.ExecutingProviderID) != stringOrEmpty(proposed.ExecutingProviderID)
}

// List returns the organization's references.
func (s *Service) List(ctx context.Context, query tenant.Query,
	filter repositories.ListFilter) ([]models.Reference, error) {
	if query.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	}
	return s.opts.References.List(ctx, query, filter)
}

// Get returns one reference inside the organization scope.
func (s *Service) Get(ctx context.Context, organizationID string,
	referenceID string) (*models.Reference, error) {
	reference, err := s.opts.References.GetByID(ctx, organizationID, referenceID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.SafetyReferenceNotFound,
			"Reference not found.")
	}
	return reference, err
}

// History returns the append-only history of one reference.
func (s *Service) History(ctx context.Context, organizationID string,
	referenceID string) ([]models.HistoryEntry, error) {
	// Prove the reference is this organization's before returning its history.
	if _, err := s.Get(ctx, organizationID, referenceID); err != nil {
		return nil, err
	}
	return s.opts.References.ListHistory(ctx, organizationID, referenceID)
}

// referenceFingerprint hashes the meaningful inputs of a record request.
func referenceFingerprint(input RecordReferenceInput) string {
	hasher := sha256.New()
	for _, part := range []string{
		input.Actor.OrganizationID, input.ReferenceType, input.ExternalReference,
		input.AssignmentID, input.PlatformTripID, input.ExecutingProviderID,
		input.VehicleReference,
	} {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func setOptional(target **string, value string) {
	if value == "" {
		return
	}
	copied := value
	*target = &copied
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
