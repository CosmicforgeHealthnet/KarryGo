package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	governanceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/usecases"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	retentionrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// classSupporter is the slice of the retention runner privacy settings use
// to report whether a class can be enforced in this deployment.
type classSupporter interface{ SupportsClass(class string) bool }

// governancePrivacyRetention adapts the one authoritative retention-policy
// store (fleet_partner_retention_policies) to the governance PrivacyRetention
// port, so privacy settings (spec 59.10) are a view over the retention policy
// that the retention executors actually enforce, never a second store.
type governancePrivacyRetention struct {
	pool   *pgxpool.Pool
	runner classSupporter
}

var _ governanceusecases.PrivacyRetention = governancePrivacyRetention{}

// NewGovernancePrivacyRetention builds the adapter; runner may be nil, in
// which case no class reports its executor as available.
func NewGovernancePrivacyRetention(pool *pgxpool.Pool, runner interface{ SupportsClass(string) bool }) governanceusecases.PrivacyRetention {
	return governancePrivacyRetention{pool: pool, runner: runner}
}

func (g governancePrivacyRetention) querier(tx pgx.Tx) database.Querier {
	if tx != nil {
		return tx
	}
	return g.pool
}

func (g governancePrivacyRetention) Read(ctx context.Context, tx pgx.Tx, organizationID string, classes []string) (map[string]governancemodels.RetentionValue, error) {
	stored, err := retentionrepos.NewRepository(g.querier(tx)).List(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	byClass := make(map[string]retentionmodels.Policy, len(stored))
	for _, p := range stored {
		byClass[p.RetentionClass] = p
	}
	out := make(map[string]governancemodels.RetentionValue, len(classes))
	for _, class := range classes {
		minimum := retentionmodels.PlatformMinimumDays[class]
		value := governancemodels.RetentionValue{RetentionDays: minimum, PlatformMinimumDays: minimum}
		if p, ok := byClass[class]; ok {
			value.RetentionDays = p.EffectiveDays()
			value.Version = p.Version
		}
		out[class] = value
	}
	return out, nil
}

func (g governancePrivacyRetention) Write(ctx context.Context, tx pgx.Tx, organizationID, class string, days int, expectedVersion int64) error {
	if !retentionmodels.ValidClass(class) || days < retentionmodels.PlatformMinimumDays[class] {
		return errors.New("governance privacy: retention value outside platform policy")
	}
	_, err := retentionrepos.NewRepository(tx).Upsert(ctx, retentionmodels.Policy{OrganizationID: organizationID, RetentionClass: class, RetentionDays: days}, expectedVersion)
	if errors.Is(err, retentionrepos.ErrVersionMismatch) {
		return governanceusecases.ErrRetentionVersionConflict
	}
	return err
}

func (g governancePrivacyRetention) Lock(ctx context.Context, tx pgx.Tx, organizationID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('fleet-privacy-settings:' || $1, 0))`, organizationID)
	return err
}

func (g governancePrivacyRetention) ExecutionAvailable(class string) bool {
	return g.runner != nil && g.runner.SupportsClass(class)
}
