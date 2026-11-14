package usecases

import (
	"context"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
)

// Store is the persistence the engagement workflows need.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	ActiveOrganizations(ctx context.Context) ([]string, error)

	SupportCaseByCreateKey(ctx context.Context, organizationID, key string) (map[string]any, error)
	InsertCreatedSupportCase(ctx context.Context, q database.Querier, id, organizationID string, in models.SupportCreateInput, p models.SupportProjection, key string) error
	GetSupportCase(ctx context.Context, organizationID, caseID string) (map[string]any, error)
	ListSupportCases(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockSupportCase(ctx context.Context, q database.Querier, organizationID, caseID string) (string, int64, error)
	InsertEventSupportCase(ctx context.Context, q database.Querier, id string, in models.SupportEvent) error
	InsertSupportInbox(ctx context.Context, q database.Querier, in models.SupportEvent, result string) error
	ApplySupportEvent(ctx context.Context, q database.Querier, in models.SupportEvent, gap string) error
	SupportCasesNeedingReconcile(ctx context.Context, organizationID string) ([]string, error)
	ReconcileSupportCase(ctx context.Context, organizationID, caseID string, p models.SupportProjection) error

	InsertQualityCase(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.QualityInput) error
	GetQualityCase(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListQualityCases(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	LockQualityCase(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateQualityCase(ctx context.Context, q database.Querier, organizationID, id string, in models.QualityInput, severity, next, actorID string) (int64, error)
	InsertQualityHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, in models.QualityInput, actorID string, version int64) error
	OverdueSevereQualityCases(ctx context.Context, organizationID string) ([]string, error)

	InsertMeasurement(ctx context.Context, q database.Querier, id string, in models.MeasurementInput, measured any, status string) (string, bool, error)
	GetMeasurement(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListMeasurements(ctx context.Context, organizationID string) ([]map[string]any, error)
	LatestSLAObservations(ctx context.Context, organizationID string) ([]models.SLAObservation, error)

	InsertEscalation(ctx context.Context, q database.Querier, id, organizationID string, in models.EscalationInput) (string, bool, error)
	InsertBreachEscalation(ctx context.Context, q database.Querier, id, organizationID, measurementID string) error
	GetEscalation(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListEscalations(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockEscalation(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateEscalationStatus(ctx context.Context, q database.Querier, organizationID, id, next string) (int64, error)
	InsertEscalationHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actorID string, version int64) error
	DueEscalations(ctx context.Context, organizationID string) ([]models.DueEscalation, error)
	MarkEscalationNotified(ctx context.Context, q database.Querier, organizationID, id, intentID string) error
}
