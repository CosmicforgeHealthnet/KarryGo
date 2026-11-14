package usecases

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
)

// Repository is the persistence inspections depend on.
type Repository interface {
	InsertTemplate(ctx context.Context, v *models.Template) error
	UpdateTemplate(ctx context.Context, org, id string, expected int64, v *models.Template) error
	GetTemplate(ctx context.Context, org, id string) (models.Template, error)
	ListTemplates(ctx context.Context, org string) ([]models.Template, error)
	InsertInspection(ctx context.Context, v *models.Inspection) error
	GetInspection(ctx context.Context, org, id string) (models.Inspection, error)
	ListInspections(ctx context.Context, org, status string, limit int, after *time.Time, afterID string) ([]models.Inspection, error)
	CompleteInspection(ctx context.Context, q database.Querier, org, id string, expected int64, result string, defectSummary json.RawMessage) (models.Inspection, error)
	InsertResult(ctx context.Context, q database.Querier, org, inspectionID string, a models.Answer, result string, critical bool) error
	PendingSafetyDeclines(ctx context.Context) ([]models.SafetyDecline, error)
	InsertDeclineInbox(ctx context.Context, q database.Querier, v models.SafetyDecline, payloadHash string) (bool, error)
	LockSystemTemplate(ctx context.Context, q database.Querier, org string) error
	SystemTemplateID(ctx context.Context, q database.Querier, org string) (string, error)
	InsertSystemTemplate(ctx context.Context, q database.Querier, id, org string, items, rules []byte) error
	InsertDeclineInspection(ctx context.Context, q database.Querier, id, org, assetID, templateID, membershipID, assignmentID string, defectSummary []byte) error
}
