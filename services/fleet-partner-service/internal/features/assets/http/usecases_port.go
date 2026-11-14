package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases is the slice of the asset use cases the handler calls.
type UseCases interface {
	ListAssetsPage(ctx context.Context, t tenant.Context, f models.Page) ([]models.Asset, bool, error)
	GetAsset(ctx context.Context, t tenant.Context, id string) (models.Asset, error)
	CreateAsset(ctx context.Context, t tenant.Context, v models.Asset) (models.Asset, error)
	UpdateAsset(ctx context.Context, t tenant.Context, id string, expected int64, v models.Asset, action string) (models.Asset, error)
	Assign(ctx context.Context, t tenant.Context, assetID string, v models.Assignment) (models.Assignment, error)
	ListDocuments(ctx context.Context, t tenant.Context, assetID string) ([]models.Document, error)
	AddDocument(ctx context.Context, t tenant.Context, assetID string, v models.Document) (models.Document, error)
	UnassignProvider(ctx context.Context, t tenant.Context, assetID string, in models.UnassignInput) (models.Assignment, error)
	AssetHistory(ctx context.Context, t tenant.Context, assetID string) ([]models.AssetHistoryEntry, error)
	ListCosts(ctx context.Context, t tenant.Context, assetID string) (models.CostHistory, error)
	RecordCost(ctx context.Context, t tenant.Context, assetID string, in models.CostInput) (models.CostEntry, error)
	ListMeterReadings(ctx context.Context, t tenant.Context, assetID, meterType string) ([]models.MeterReading, error)
	RecordMeterReading(ctx context.Context, t tenant.Context, in models.MeterReadingInput) (models.MeterReading, error)
	ListBreakdowns(ctx context.Context, t tenant.Context) ([]models.Breakdown, error)
	GetBreakdown(ctx context.Context, t tenant.Context, id string) (models.Breakdown, error)
	SaveBreakdown(ctx context.Context, t tenant.Context, id string, expected int64, v models.Breakdown) (models.Breakdown, error)
	ListParts(ctx context.Context, t tenant.Context) ([]models.Part, error)
	GetPart(ctx context.Context, t tenant.Context, id string) (models.Part, error)
	SavePart(ctx context.Context, t tenant.Context, id string, expected int64, v models.Part) (models.Part, error)
	ListMovements(ctx context.Context, t tenant.Context) ([]models.StockMovement, error)
	CreateMovement(ctx context.Context, t tenant.Context, v models.StockMovement) (models.StockMovement, error)
	ListDefects(ctx context.Context, t tenant.Context, f models.DefectFilter) ([]models.Defect, error)
	GetDefect(ctx context.Context, t tenant.Context, id string) (models.Defect, error)
	ReportDefect(ctx context.Context, t tenant.Context, in models.ReportDefectInput) (models.Defect, error)
	DefectTransition(ctx context.Context, t tenant.Context, id, action string, in models.DefectAction) (models.Defect, error)
	ListTyres(ctx context.Context, t tenant.Context, assetID, status string) ([]models.Tyre, error)
	GetTyre(ctx context.Context, t tenant.Context, id string) (models.Tyre, error)
	RegisterTyre(ctx context.Context, t tenant.Context, in models.RegisterTyreInput) (models.Tyre, error)
	TyreTransition(ctx context.Context, t tenant.Context, id, action string, in models.TyreAction) (models.Tyre, error)
}
