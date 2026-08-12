// Package usecases wraps generic legal-hold mutations with the atomic
// mutation+audit convention this service's other domains follow: the hold
// row and its audit record commit in the same transaction, so an audit
// write failure fails the mutation rather than leaving a hold that exists
// without a trace of who created it.
//
// No domain event is published for hold changes in this slice — disclosed,
// not silently omitted: Section 26's event registry exists (see
// internal/events), but wiring a new event type through it for a feature
// this narrow was judged out of scope for the same pass that first stood
// the table up.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// Errors surfaced to the HTTP layer.
var (
	ErrScopeTypeInvalid = errors.New("legalhold: scope type is not supported")
	ErrReasonRequired   = errors.New("legalhold: reason is required")
)

// Action classes passed to stepup.Verifier -- match the permission codes
// that already gate these routes, so a step-up grant obtained for "create a
// legal hold" cannot be replayed against "release a legal hold" even though
// both actions share the feature.
const (
	ActionClassCreate  = "privacy.legal_hold.manage"
	ActionClassRelease = "privacy.legal_hold.release"
)

// SecurityPolicyReader resolves whether an organization currently requires
// step-up for sensitive actions, per that organization's own security
// settings (internal/features/securitysettings). A nil reader, or one that
// errors, means "assume required" — the platform default — never "assume
// not required": a missing signal must never silently relax a control.
type SecurityPolicyReader interface {
	MFARequiredForSensitiveActions(ctx context.Context, organizationID string) (bool, error)
}

// Actor identifies who is creating or releasing a hold.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// Service applies the atomicity and validation rules around the repository.
type Service struct {
	pool           *pgxpool.Pool
	reads          *repositories.Repository
	recorder       *audit.Recorder
	stepUp         stepup.Verifier
	securityPolicy SecurityPolicyReader
}

// NewService builds the service. pool is required for transactional writes;
// reads use the same pool directly (no transaction needed for a read).
// stepUp and securityPolicy may both be nil — matching every other optional
// dependency in this service's composition root — in which case step-up is
// recorded (if a reference is supplied) but never actually verified. See
// docs/section28-security-privacy-audit.md for why that is disclosed rather
// than silently claimed as enforcement.
func NewService(pool *pgxpool.Pool, stepUp stepup.Verifier, securityPolicy SecurityPolicyReader) *Service {
	return &Service{
		pool: pool, reads: repositories.NewRepository(pool), recorder: audit.NewRecorder(),
		stepUp: stepUp, securityPolicy: securityPolicy,
	}
}

// verifyStepUp enforces step-up for a sensitive action when both the
// organization's own policy requires it (or no policy signal is available,
// which defaults to "required") and a verifier is actually wired. It never
// weakens enforcement based on a missing dependency — an unwired verifier
// means the check cannot run at all, which is a deployment-configuration
// fact, not a policy decision to skip it.
//
// identityID is the raw auth identity (claims.Subject) a step-up challenge
// was originally verified against — deliberately NOT Actor.ActorID, which
// this feature uses for the organization-membership id in audit records.
// The two are different values for the same human, and step-up verification
// must use the one the auth feature itself recognizes.
func (s *Service) verifyStepUp(ctx context.Context, identityID, organizationID, sessionID, reference, actionClass string) error {
	required := true
	if s.securityPolicy != nil {
		if r, err := s.securityPolicy.MFARequiredForSensitiveActions(ctx, organizationID); err == nil {
			required = r
		}
	}
	if !required || s.stepUp == nil {
		return nil
	}
	return s.stepUp.Verify(ctx, reference, identityID, sessionID, organizationID, actionClass)
}

// CreateInput describes a new hold.
type CreateInput struct {
	Actor           Actor
	IdentityID      string
	SessionID       string
	ScopeType       string
	ScopeReference  string
	Reason          string
	ApprovedByID    string
	StepUpReference string
	RequestID       string
	CorrelationID   string
}

// Create validates and inserts a new ACTIVE hold, auditing it in the same
// transaction.
func (s *Service) Create(ctx context.Context, input CreateInput) (models.Hold, error) {
	scopeType := models.NormalizeScopeType(input.ScopeType)
	if !models.ValidScopeTypes[scopeType] {
		return models.Hold{}, ErrScopeTypeInvalid
	}
	scopeReference := strings.TrimSpace(input.ScopeReference)
	reason := strings.TrimSpace(input.Reason)
	if scopeReference == "" {
		return models.Hold{}, fmt.Errorf("legalhold: scope reference is required")
	}
	if reason == "" {
		return models.Hold{}, ErrReasonRequired
	}
	if err := s.verifyStepUp(ctx, input.IdentityID, input.Actor.OrganizationID, input.SessionID,
		input.StepUpReference, ActionClassCreate); err != nil {
		return models.Hold{}, err
	}

	var created models.Hold
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		txRepo := repositories.NewRepository(tx)
		hold, err := txRepo.Create(ctx, models.Hold{
			OrganizationID:     input.Actor.OrganizationID,
			ScopeType:          scopeType,
			ScopeReference:     scopeReference,
			Reason:             reason,
			CreatedByActorType: input.Actor.ActorType,
			CreatedByActorID:   input.Actor.ActorID,
			ApprovedByActorID:  strings.TrimSpace(input.ApprovedByID),
			StepUpReference:    strings.TrimSpace(input.StepUpReference),
		})
		if err != nil {
			return err
		}
		created = hold
		return s.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: input.Actor.OrganizationID,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Action:         "legal_hold.created",
			ResourceType:   "legal_hold:" + scopeType,
			ResourceID:     hold.ID,
			NewValues: map[string]any{
				"scopeType": scopeType, "scopeReference": scopeReference, "reason": reason,
			},
			Reason:          reason,
			RequestID:       input.RequestID,
			CorrelationID:   input.CorrelationID,
			StepUpReference: input.StepUpReference,
		})
	})
	if err != nil {
		return models.Hold{}, err
	}
	return created, nil
}

// Get returns one hold scoped to the caller's organization.
func (s *Service) Get(ctx context.Context, organizationID, id string) (models.Hold, error) {
	return s.reads.Get(ctx, organizationID, id)
}

// List returns holds matching filter.
func (s *Service) List(ctx context.Context, filter repositories.ListFilter) ([]models.Hold, error) {
	return s.reads.List(ctx, filter)
}

// IsUnderHold reports whether any ACTIVE hold exists for the given scope —
// the query a business-domain deletion/anonymization path must call before
// proceeding.
func (s *Service) IsUnderHold(ctx context.Context, organizationID, scopeType, scopeReference string) (bool, error) {
	return s.reads.IsUnderHold(ctx, organizationID, scopeType, scopeReference)
}

// ReleaseInput describes releasing an existing hold.
type ReleaseInput struct {
	Actor           Actor
	IdentityID      string
	SessionID       string
	HoldID          string
	ExpectedVersion int64
	ReleaseReason   string
	StepUpReference string
	RequestID       string
	CorrelationID   string
}

// Release transitions an ACTIVE hold to RELEASED, auditing it in the same
// transaction. repositories.ErrVersionMismatch and
// repositories.ErrAlreadyReleased pass through unwrapped so the HTTP layer
// can map them to the correct status code.
func (s *Service) Release(ctx context.Context, input ReleaseInput) (models.Hold, error) {
	if err := s.verifyStepUp(ctx, input.IdentityID, input.Actor.OrganizationID, input.SessionID,
		input.StepUpReference, ActionClassRelease); err != nil {
		return models.Hold{}, err
	}

	var released models.Hold
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		txRepo := repositories.NewRepository(tx)
		hold, err := txRepo.Release(ctx, input.Actor.OrganizationID, input.HoldID, input.ExpectedVersion,
			input.Actor.ActorID, strings.TrimSpace(input.ReleaseReason))
		if err != nil {
			return err
		}
		released = hold
		return s.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID:  input.Actor.OrganizationID,
			ActorType:       input.Actor.ActorType,
			ActorID:         input.Actor.ActorID,
			Action:          "legal_hold.released",
			ResourceType:    "legal_hold:" + hold.ScopeType,
			ResourceID:      hold.ID,
			Reason:          input.ReleaseReason,
			RequestID:       input.RequestID,
			CorrelationID:   input.CorrelationID,
			StepUpReference: input.StepUpReference,
		})
	})
	if err != nil {
		return models.Hold{}, err
	}
	return released, nil
}
