package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"time"
)

// Repository is the persistence the asset use cases depend on. Methods take
// the Querier to run on (nil means the pool) so a workflow composes them into
// one transaction; a missing row is pgx.ErrNoRows.
type Repository interface {
	InsertAsset(ctx context.Context, q database.Querier, org string, v models.Asset) (models.Asset, error)
	GetAsset(ctx context.Context, org, id string) (models.Asset, error)
	FindAssetByReference(ctx context.Context, org, reference string) (models.Asset, error)
	ListAssets(ctx context.Context, org string, f models.Page, permitted []string, limit, offset int) ([]models.Asset, error)
	ActiveAssignmentExists(ctx context.Context, q database.Querier, org, assetID string) (bool, error)
	LockAsset(ctx context.Context, q database.Querier, org, id string) (models.AssetLock, error)
	AssetStatus(ctx context.Context, q database.Querier, org, id string) (operational, lifecycle string, e error)
	AssetLifecycleAndBranch(ctx context.Context, q database.Querier, org, id string) (lifecycle, branch string, e error)
	UpdateAsset(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Asset, lifecycle string) (models.Asset, error)
	InsertAssetHistory(ctx context.Context, q database.Querier, org, assetID, prevOp, newOp, prevLife, newLife, reason, actor string, version int64, snapshot any) error
	ListAssetHistory(ctx context.Context, org, assetID string) ([]models.AssetHistoryEntry, error)
	SetOperationalStatus(ctx context.Context, q database.Querier, org, id, status string) (int64, error)
	MarkAssigned(ctx context.Context, q database.Querier, assetID string) error
	MarkOutOfService(ctx context.Context, q database.Querier, org, id string) error
	ActiveMembership(ctx context.Context, org, id string) bool
	BranchExists(ctx context.Context, q database.Querier, org, id string) (bool, error)
	AssignmentConflict(ctx context.Context, org, assetID, provider string) bool
	ProviderAuthorized(ctx context.Context, org, assetID, provider string, at time.Time) bool
	DocumentExpired(ctx context.Context, org, assetID string) bool
	MaintenanceOverdue(ctx context.Context, org, assetID string, odometer, hours float64) bool
	InsertAssignment(ctx context.Context, q database.Querier, v *models.Assignment) error
	LockCurrentAssignment(ctx context.Context, q database.Querier, org, assetID, assignmentID string) (models.Assignment, error)
	ProviderHasActiveWork(ctx context.Context, q database.Querier, org, membershipID, assetID, platformVehicleID string) (bool, error)
	EndAssignment(ctx context.Context, q database.Querier, org, actor, reason string, out *models.Assignment) error
	InsertDocument(ctx context.Context, q database.Querier, v *models.Document) error
	ListDocuments(ctx context.Context, org, assetID string) ([]models.Document, error)
	ExpireDocumentsAndAssignments(ctx context.Context, org string) error
	ActiveOrganizationIDs(ctx context.Context) ([]string, error)
	LastMeterReadingAt(ctx context.Context, q database.Querier, org, assetID, meterType string) (*time.Time, error)
	InsertMeterReading(ctx context.Context, q database.Querier, v *models.MeterReading) error
	SetMeter(ctx context.Context, q database.Querier, org, id, meterType string, reading float64) (int64, error)
	SchedulesCrossed(ctx context.Context, q database.Querier, org, assetID, meterType string, reading, previous float64) ([]string, error)
	ListMeterReadings(ctx context.Context, org, assetID, meterType string) ([]models.MeterReading, error)
	CostSourceLinked(ctx context.Context, org, sourceType, reference, assetID string) (bool, error)
	InsertCost(ctx context.Context, q database.Querier, v *models.CostEntry) error
	ListCostEntries(ctx context.Context, org, assetID string) ([]models.CostEntry, error)
	CostTotals(ctx context.Context, org, assetID string) ([]models.CostTotal, error)
	BlockAsset(ctx context.Context, q database.Querier, org, id string) (int64, error)
	BlockActiveAsset(ctx context.Context, q database.Querier, org, id string) (int64, error)
	BlockAssetForDecline(ctx context.Context, q database.Querier, org, id string) error
	InsertSafetyBlock(ctx context.Context, q database.Querier, org, assetID, sourceType, sourceRef, reason, createdBy string, dedupe bool) error
	UncoveredSafetyBlocks(ctx context.Context, q database.Querier, org, assetID string) (int, error)
	ReleaseBlock(ctx context.Context, q database.Querier, org, assetID, workOrderID, actor, reason string) (int64, error)
	ResolveAssetID(ctx context.Context, q database.Querier, org, reference string) (string, string, error)
	InsertBreakdown(ctx context.Context, q database.Querier, v *models.Breakdown) error
	UpdateBreakdown(ctx context.Context, q database.Querier, org, id string, expected int64, v *models.Breakdown) error
	GetBreakdown(ctx context.Context, org, id string) (models.Breakdown, error)
	ListBreakdownIDs(ctx context.Context, org string) ([]string, error)
	InsertPart(ctx context.Context, q database.Querier, v *models.Part) error
	UpdatePart(ctx context.Context, q database.Querier, org, id string, expected int64, v *models.Part) error
	GetPart(ctx context.Context, org, id string) (models.Part, error)
	ListPartIDs(ctx context.Context, org string) ([]string, error)
	PartsAtReorderLevel(ctx context.Context, org string) ([]string, error)
	LockPartStock(ctx context.Context, q database.Querier, org, id string) (float64, float64, error)
	SetPartProjection(ctx context.Context, q database.Querier, org, id string, projected float64) (int64, error)
	InsertStockMovement(ctx context.Context, q database.Querier, v *models.StockMovement) error
	ListStockMovements(ctx context.Context, org string) ([]models.StockMovement, error)
	InsertDefect(ctx context.Context, q database.Querier, org, branchID string, in models.ReportDefectInput, workOrderID, actor string) (models.Defect, error)
	InsertDefectHistory(ctx context.Context, q database.Querier, org, defectID, previous, next, actor, reason string, version int64) error
	LinkDefectWorkOrder(ctx context.Context, q database.Querier, org string, d *models.Defect, workOrderID string) error
	LockAssetRow(ctx context.Context, q database.Querier, org, id string) error
	GetDefect(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Defect, error)
	ListDefects(ctx context.Context, org string, f models.DefectFilter) ([]models.Defect, error)
	WorkOrderUsable(ctx context.Context, q database.Querier, org, workOrderID, assetID string) (bool, error)
	WorkOrderCompleted(ctx context.Context, q database.Querier, org, workOrderID string) (bool, error)
	TransitionDefect(ctx context.Context, q database.Querier, org, id string, version int64, to, workOrderID string, d models.DefectAction, reason, actor string) (models.Defect, error)
	UnrepairedCriticalDefects(ctx context.Context, q database.Querier, org, assetID string) (int, error)
	ResolveCriticalDefects(ctx context.Context, q database.Querier, org, assetID, notes, actor string) ([]models.ResolvedDefect, error)
	OpenTyreDefectExists(ctx context.Context, q database.Querier, org, itemCode string) (bool, error)
	InsertTyreEvent(ctx context.Context, q database.Querier, org, tyreID, kind, assetID, from, to, reason, actor string, depth, odometer *float64) (string, error)
	InsertTyre(ctx context.Context, q database.Querier, org string, in models.RegisterTyreInput, minTread float64) (models.Tyre, error)
	GetTyre(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Tyre, error)
	ListTyres(ctx context.Context, org, assetID, status string) ([]models.Tyre, error)
	UpdateTyre(ctx context.Context, q database.Querier, org, id string, version int64, status, assetID, position string, depth *float64) (models.Tyre, error)
}
