// Package usecases wraps security-settings mutations with the atomic
// mutation+audit convention this service's other domains follow.
package usecases

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// ActionClassUpdate is passed to stepup.Verifier for a settings change.
const ActionClassUpdate = "security.settings.manage"

// Actor identifies who is changing settings.
type Actor struct {
	OrganizationID string
	ActorType      string
	ActorID        string
}

// Service applies validation and atomicity around the repository.
type Service struct {
	pool     *pgxpool.Pool
	reads    *repositories.Repository
	recorder *audit.Recorder
	stepUp   stepup.Verifier
}

// NewService builds the service. stepUp may be nil (same optional-dependency
// convention as every other feature this pass touched) — see
// docs/section28-security-privacy-audit.md.
func NewService(pool *pgxpool.Pool, stepUp stepup.Verifier) *Service {
	return &Service{pool: pool, reads: repositories.NewRepository(pool), recorder: audit.NewRecorder(), stepUp: stepUp}
}

// Get returns the organization's settings (platform defaults if unset).
func (s *Service) Get(ctx context.Context, organizationID string) (models.Settings, error) {
	return s.reads.Get(ctx, organizationID)
}

// MFARequiredForSensitiveActions satisfies the narrow SecurityPolicyReader
// port other features (legalhold) consume via a composition-root adapter,
// so a step-up decision elsewhere in the service reads the SAME setting
// this feature's own Update enforces against, rather than a second,
// independently-drifting copy of the same policy.
func (s *Service) MFARequiredForSensitiveActions(ctx context.Context, organizationID string) (bool, error) {
	settings, err := s.reads.Get(ctx, organizationID)
	if err != nil {
		return false, err
	}
	return settings.MFARequiredForSensitiveActions, nil
}

// UpdateInput describes a settings change.
type UpdateInput struct {
	Actor           Actor
	IdentityID      string
	SessionID       string
	StepUpReference string
	Settings        models.Settings
	ExpectedVersion int64
	RequestID       string
	CorrelationID   string
}

// Update validates and persists new settings, auditing the change in the
// same transaction. Returns models.ValidationError values (via a
// []models.ValidationError, not wrapped) when validation fails.
//
// Step-up is required for the change whenever the organization's CURRENT
// (pre-change) settings say so — a caller cannot use this very update to
// simultaneously turn MFA-required off and skip proving they were allowed
// to: the gate is evaluated against what was true before the request, not
// what the request is asking to become true.
func (s *Service) Update(ctx context.Context, input UpdateInput) (models.Settings, []models.ValidationError, error) {
	input.Settings.OrganizationID = input.Actor.OrganizationID
	if errs := input.Settings.Validate(); len(errs) > 0 {
		return models.Settings{}, errs, nil
	}

	if s.stepUp != nil {
		current, err := s.reads.Get(ctx, input.Actor.OrganizationID)
		if err != nil {
			return models.Settings{}, nil, err
		}
		if current.MFARequiredForSensitiveActions {
			if err := s.stepUp.Verify(ctx, input.StepUpReference, input.IdentityID, input.SessionID,
				input.Actor.OrganizationID, ActionClassUpdate); err != nil {
				return models.Settings{}, nil, err
			}
		}
	}

	var updated models.Settings
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		txRepo := repositories.NewRepository(tx)
		result, err := txRepo.Upsert(ctx, input.Settings, input.ExpectedVersion)
		if err != nil {
			return err
		}
		updated = result
		return s.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: input.Actor.OrganizationID,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Action:         "security_settings.updated",
			ResourceType:   "security_settings",
			ResourceID:     input.Actor.OrganizationID,
			NewValues: map[string]any{
				"mfaRequiredForSensitiveActions":   result.MFARequiredForSensitiveActions,
				"stepUpFreshnessSeconds":           result.StepUpFreshnessSeconds,
				"sessionRevocationPolicy":          result.SessionRevocationPolicy,
				"allowedProviderAccessPolicy":      result.AllowedProviderAccessPolicy,
				"restrictedDocumentStepUpRequired": result.RestrictedDocumentStepUpRequired,
				"auditCapturePolicy":               result.AuditCapturePolicy,
				"retentionPreferenceDays":          result.RetentionPreferenceDays,
			},
			RequestID:       input.RequestID,
			CorrelationID:   input.CorrelationID,
			StepUpReference: input.StepUpReference,
		})
	})
	if err != nil {
		return models.Settings{}, nil, err
	}
	return updated, nil, nil
}
