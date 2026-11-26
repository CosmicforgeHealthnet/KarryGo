package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/repositories"
)

// Repository is the persistence the analytics use cases depend on;
// *repositories.Repository implements it.
type Repository interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	GenerateReport(ctx context.Context, organizationID, reportType string, from, to time.Time, branches []string) ([]models.ReportSection, []string, error)
	InsertDefinition(ctx context.Context, q database.Querier, id, organizationID string, in models.ReportDefinitionInput, filters []byte, next *time.Time, actor string) error
	DefinitionByName(ctx context.Context, organizationID, name string) (string, error)
	GetDefinition(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListDefinitions(ctx context.Context, organizationID string) ([]map[string]any, error)
	DueDefinitions(ctx context.Context, q database.Querier, now time.Time) ([]map[string]any, error)
	AdvanceSchedule(ctx context.Context, q database.Querier, id string, next time.Time) error
	InsertRun(ctx context.Context, q database.Querier, id, organizationID, definitionID, reportType string, filters, permissions []byte, trigger, actor string, expires time.Time) (string, bool, error)
	GetRun(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListRuns(ctx context.Context, organizationID, requester string) ([]map[string]any, error)
	ClaimRun(ctx context.Context, organizationID, id string) (reportType string, filters, permissions []byte, ok bool, err error)
	CompleteRun(ctx context.Context, organizationID, id string, result []byte, rows int) error
	FailRun(ctx context.Context, organizationID, id, code string) error
	ExpireRuns(ctx context.Context, organizationID string, now time.Time) (int64, error)
	MaintenanceCandidates(ctx context.Context, organizationID string) ([]map[string]any, error)
	FuelOdometerAnomalies(ctx context.Context, organizationID string) ([]map[string]any, error)
	StaleLiveTrips(ctx context.Context, organizationID string) ([]map[string]any, error)
	InsertRecommendation(ctx context.Context, q database.Querier, id, organizationID string, v models.Recommendation, hash string, expires time.Time) (bool, error)
	GetRecommendation(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListRecommendations(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockRecommendation(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, expires time.Time, err error)
	DecideRecommendation(ctx context.Context, q database.Querier, organizationID, id, status, actor, reason string) (int64, error)
	ExpireRecommendations(ctx context.Context, organizationID string, now time.Time) (int64, error)
	ListModelVersions(ctx context.Context) ([]map[string]any, error)
	InsertRule(ctx context.Context, q database.Querier, id, organizationID string, in models.AutomationRuleInput, actor string) error
	RuleByName(ctx context.Context, organizationID, name string) (string, error)
	GetRule(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListRules(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockRule(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, creator, submitter string, err error)
	UpdateRuleDefinition(ctx context.Context, q database.Querier, organizationID, id string, in models.AutomationRuleInput) (int64, error)
	SetRuleStatus(ctx context.Context, q database.Querier, organizationID, id, status, actor string) (int64, error)
	InsertRuleVersion(ctx context.Context, q database.Querier, organizationID, id string, version int64, status, actor string) error
	ActiveEventRules(ctx context.Context, organizationID string) ([]map[string]any, error)
	OutboxAfter(ctx context.Context, organizationID string, after time.Time, afterID string, types []string, limit int) ([]repositories.OutboxEvent, error)
	Cursor(ctx context.Context, q database.Querier, organizationID string, now time.Time) (time.Time, string, error)
	AdvanceCursor(ctx context.Context, q database.Querier, organizationID string, at time.Time, id string) error
	InsertExecution(ctx context.Context, q database.Querier, id, organizationID, ruleID string, ruleVersion int64, trigger string, trace []byte, status string) (bool, error)
	ExecutionForRun(ctx context.Context, q database.Querier, organizationID, id string) (status, ruleID, trigger string, plan []byte, trace []byte, err error)
	FinishExecution(ctx context.Context, q database.Querier, organizationID, id, status string, results []byte, code string) error
	ListExecutions(ctx context.Context, organizationID string) ([]map[string]any, error)
	OrganizationsWithAnalytics(ctx context.Context) ([]string, error)
}

var _ Repository = (*repositories.Repository)(nil)
