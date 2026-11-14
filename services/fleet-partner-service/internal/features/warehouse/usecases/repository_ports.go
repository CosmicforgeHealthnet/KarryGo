package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"encoding/json"
	"time"
)

// Repository is the persistence the warehouse use cases depend on. Methods
// take the Querier to run on (nil means the pool) so a workflow composes them
// into one transaction; a missing row is pgx.ErrNoRows.
type Repository interface {
	InsertWarehouse(ctx context.Context, q database.Querier, v models.Warehouse) (models.Warehouse, error)
	GetWarehouse(ctx context.Context, org, id string) (models.Warehouse, error)
	ListWarehouses(ctx context.Context, org string, branches []string) ([]models.Warehouse, error)
	WarehouseHasStock(ctx context.Context, id string) bool
	UpdateWarehouse(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Warehouse, status string) (models.Warehouse, error)
	WarehouseBranch(ctx context.Context, id string) string
	OrganizationsWithActiveWarehouses(ctx context.Context) ([]string, error)
	GetLocation(ctx context.Context, org, warehouseID, id string) (models.Location, error)
	InsertLocation(ctx context.Context, q database.Querier, v models.Location) (models.Location, error)
	ListLocations(ctx context.Context, org, warehouseID string) ([]models.Location, error)
	LocationParentCycle(ctx context.Context, id, parent string) (bool, error)
	UpdateLocation(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Location, status string) (models.Location, error)
	LockLocationCapacity(ctx context.Context, q database.Querier, org, location string) (*float64, error)
	LocationUsedQuantity(ctx context.Context, q database.Querier, org, location string) (float64, error)
	PackageShipment(ctx context.Context, q database.Querier, org, packageID string) (string, error)
	PackageExists(ctx context.Context, q database.Querier, org, packageID string) bool
	ManifestExists(ctx context.Context, org, id string) bool
	InsertReceiptCustody(ctx context.Context, q database.Querier, org string, in models.ReceiptInput, shipment string, at time.Time) error
	InsertReturnCustody(ctx context.Context, q database.Querier, org string, v models.Return, shipment string) error
	MovementItemByCorrelation(ctx context.Context, org, correlation string) (string, error)
	GetMovementByCorrelation(ctx context.Context, org, correlation string) (models.Movement, error)
	LockReceiptItem(ctx context.Context, q database.Querier, org string, in models.ReceiptInput) (models.Item, error)
	InsertItem(ctx context.Context, q database.Querier, item models.Item, quantity float64) (models.Item, error)
	AddItemQuantity(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error)
	InsertReceiptMovement(ctx context.Context, q database.Querier, m models.Movement) error
	GetItem(ctx context.Context, org, id string) (models.Item, error)
	ListItems(ctx context.Context, org string, branches []string) ([]models.Item, error)
	ListMovements(ctx context.Context, org, itemID string) ([]models.Movement, error)
	LockItem(ctx context.Context, q database.Querier, org, id string) (models.Item, error)
	ApplyItemMove(ctx context.Context, q database.Querier, org, id, location string, after float64, status string) error
	InsertMovement(ctx context.Context, q database.Querier, v *models.Movement) error
	ItemWarehouse(ctx context.Context, q database.Querier, org, itemID string) (string, error)
	LockItemStock(ctx context.Context, q database.Querier, org, itemID string) (qty, reserved float64, status string, expiry *time.Time, e error)
	LockItemQuantity(ctx context.Context, q database.Querier, org, itemID string) (float64, error)
	LockItemQuantityAndLocation(ctx context.Context, q database.Querier, org, itemID string) (qty, reserved float64, location string, e error)
	SetItemStatus(ctx context.Context, q database.Querier, itemID, status string) error
	SetItemQuantityAndStatus(ctx context.Context, q database.Querier, org, itemID string, quantity float64, status string) error
	LoadLines(ctx context.Context, order *models.FulfilmentOrder) error
	InsertFulfilmentOrder(ctx context.Context, q database.Querier, v models.FulfilmentOrder) (models.FulfilmentOrder, error)
	InsertFulfilmentLine(ctx context.Context, q database.Querier, l *models.FulfilmentLine) error
	GetFulfilmentOrder(ctx context.Context, org, id string) (models.FulfilmentOrder, error)
	ListFulfilmentOrders(ctx context.Context, org string, branches []string) ([]models.FulfilmentOrder, error)
	UpdateFulfilmentOrder(ctx context.Context, q database.Querier, org, id string, expected int64, v models.FulfilmentOrder) (models.FulfilmentOrder, error)
	LockFulfilmentOrder(ctx context.Context, q database.Querier, org, id string) (models.FulfilmentOrderState, error)
	LockFulfilmentLines(ctx context.Context, q database.Querier, org, id string) ([]models.FulfilmentLineState, error)
	ReserveItem(ctx context.Context, q database.Querier, org, itemID string, quantity float64) error
	InsertReservation(ctx context.Context, q database.Querier, org, itemID, orderID string, quantity float64, expiresAt *time.Time) error
	MarkLinePicked(ctx context.Context, q database.Querier, lineID string) error
	SetReservationsStatus(ctx context.Context, q database.Querier, org, itemID, orderID, status string) error
	InsertPickMovement(ctx context.Context, q database.Querier, org, itemID, orderID, actor string, quantity float64) error
	SetLinePackage(ctx context.Context, q database.Querier, lineID, packageRef string) error
	DispatchItem(ctx context.Context, q database.Querier, org, itemID string, after, requested float64, status string) error
	InsertDispatchMovement(ctx context.Context, q database.Querier, org, itemID, location, orderID, actor string, quantity, before, after float64) error
	ReleaseItemReservation(ctx context.Context, q database.Querier, org, itemID string, quantity float64) error
	TransitionFulfilmentOrder(ctx context.Context, q database.Querier, org, id string, expected int64, next, shipment, manifest string) (models.FulfilmentOrder, error)
	LockExpiredReservations(ctx context.Context, q database.Querier, org string) ([]models.ExpiredReservation, error)
	ExpireReservation(ctx context.Context, q database.Querier, v models.ExpiredReservation) error
	SweepStaleOperations(ctx context.Context, q database.Querier, org string) error
	InsertReservationExpiryMovement(ctx context.Context, q database.Querier, org string, v models.ExpiredReservation)
	LockDock(ctx context.Context, q database.Querier, org, dockLocationID string)
	DockConflict(ctx context.Context, q database.Querier, org, dockLocationID string, start, end time.Time) bool
	InsertDock(ctx context.Context, q database.Querier, v models.DockAppointment) (models.DockAppointment, error)
	GetDock(ctx context.Context, org, id string) (models.DockAppointment, error)
	ListDocks(ctx context.Context, org string, branches []string) ([]models.DockAppointment, error)
	UpdateDock(ctx context.Context, q database.Querier, org, id string, expected int64, command, status string) (models.DockAppointment, error)
	InsertYard(ctx context.Context, q database.Querier, v models.YardVisit) (models.YardVisit, error)
	GetYard(ctx context.Context, org, id string) (models.YardVisit, error)
	ListYards(ctx context.Context, org string, branches []string) ([]models.YardVisit, error)
	UpdateYard(ctx context.Context, q database.Querier, org, id string, expected int64, status, security, newSeal string) (models.YardVisit, error)
	InsertYardHistory(ctx context.Context, q database.Querier, org, id string, old, next models.YardVisit, reason, actor string) error
	InsertTransfer(ctx context.Context, q database.Querier, v models.TerminalTransfer) (models.TerminalTransfer, error)
	GetTransfer(ctx context.Context, org, id string) (models.TerminalTransfer, error)
	ListTransfers(ctx context.Context, org string, branches []string) ([]models.TerminalTransfer, error)
	TransferSnapshot(ctx context.Context, q database.Querier, transferID, snapshotType string) ([]byte, error)
	InsertDiscrepancy(ctx context.Context, q database.Querier, org, transferID string, expected, actual []byte)
	ManifestPackageSnapshot(ctx context.Context, q database.Querier, manifestID string) (json.RawMessage, error)
	InsertTransferSnapshot(ctx context.Context, q database.Querier, id, org, transferID, snapshotType string, pkg json.RawMessage, actor string) error
	UpdateTransfer(ctx context.Context, q database.Querier, org, id string, expected int64, next, snapshotType, snapRef string) (models.TerminalTransfer, error)
	InsertReturn(ctx context.Context, q database.Querier, v models.Return) (models.Return, error)
	GetReturn(ctx context.Context, org, id string) (models.Return, error)
	ListReturns(ctx context.Context, org string, branches []string) ([]models.Return, error)
	LockItemForReturn(ctx context.Context, q database.Querier, org, itemID string) (float64, string, error)
	InsertReturnMovement(ctx context.Context, q database.Querier, org string, v models.Return, location, actor string, before, after float64) error
	UpdateReturn(ctx context.Context, q database.Querier, org, id string, expected int64, next, disposition string) (models.Return, error)
}
