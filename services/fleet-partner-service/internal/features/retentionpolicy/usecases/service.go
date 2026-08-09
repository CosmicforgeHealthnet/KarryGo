// Package usecases wraps retention-policy mutations with the atomic
// mutation+audit convention this service's other domains follow, and adds
// the one rule the repository alone cannot enforce: an organization may only
// extend retention, never shorten it below the platform minimum for that
// class.
package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// ErrClassInvalid means the caller supplied an unrecognized retention class.
var ErrClassInvalid = errors.New("retentionpolicy: retention class is not supported")

// ErrBelowPlatformMinimum means the requested duration is shorter than the
// platform floor for that class.
var ErrBelowPlatformMinimum = errors.New("retentionpolicy: retention days is below the platform minimum for this class")

// Actor identifies who is changing a policy.
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
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, reads: repositories.NewRepository(pool), recorder: audit.NewRecorder()}
}

// Get returns the organization's effective policy for class (platform
// minimum if never overridden).
func (s *Service) Get(ctx context.Context, organizationID, class string) (models.Policy, error) {
	if !models.ValidClass(class) {
		return models.Policy{}, ErrClassInvalid
	}
	return s.reads.Get(ctx, organizationID, class)
}

// List returns every recognized class with its effective policy: explicit
// overrides from the repository, platform minimum for everything else.
func (s *Service) List(ctx context.Context, organizationID string) ([]models.Policy, error) {
	overrides, err := s.reads.List(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	byClass := make(map[string]models.Policy, len(overrides))
	for _, p := range overrides {
		byClass[p.RetentionClass] = p
	}
	result := make([]models.Policy, 0, len(models.PlatformMinimumDays))
	for class, minimum := range models.PlatformMinimumDays {
		if p, ok := byClass[class]; ok {
			result = append(result, p)
			continue
		}
		result = append(result, models.Policy{OrganizationID: organizationID, RetentionClass: class, RetentionDays: minimum, Version: 0})
	}
	return result, nil
}

// UpdateInput describes a policy change for one class.
type UpdateInput struct {
	Actor           Actor
	RetentionClass  string
	RetentionDays   int
	ExpectedVersion int64
	RequestID       string
	CorrelationID   string
}

// Update validates and persists a retention override, auditing it in the
// same transaction.
func (s *Service) Update(ctx context.Context, input UpdateInput) (models.Policy, error) {
	if !models.ValidClass(input.RetentionClass) {
		return models.Policy{}, ErrClassInvalid
	}
	if input.RetentionDays < models.PlatformMinimumDays[input.RetentionClass] {
		return models.Policy{}, ErrBelowPlatformMinimum
	}

	var updated models.Policy
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		txRepo := repositories.NewRepository(tx)
		result, err := txRepo.Upsert(ctx, models.Policy{
			OrganizationID: input.Actor.OrganizationID, RetentionClass: input.RetentionClass, RetentionDays: input.RetentionDays,
		}, input.ExpectedVersion)
		if err != nil {
			return err
		}
		updated = result
		return s.recorder.Record(ctx, tx, audit.Entry{
			OrganizationID: input.Actor.OrganizationID,
			ActorType:      input.Actor.ActorType,
			ActorID:        input.Actor.ActorID,
			Action:         "retention_policy.updated",
			ResourceType:   "retention_policy:" + input.RetentionClass,
			ResourceID:     fmt.Sprintf("%s:%s", input.Actor.OrganizationID, input.RetentionClass),
			NewValues:      map[string]any{"retentionClass": input.RetentionClass, "retentionDays": input.RetentionDays},
			RequestID:      input.RequestID,
			CorrelationID:  input.CorrelationID,
		})
	})
	if err != nil {
		return models.Policy{}, err
	}
	return updated, nil
}
