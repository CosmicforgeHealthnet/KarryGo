package http

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"encoding/json"
)

// UseCases is the slice of the warehouse use cases the handler calls.
type UseCases interface {
	ListWarehouses(ctx context.Context, t tenant.Context) ([]models.Warehouse, error)
	CreateWarehouse(ctx context.Context, t tenant.Context, v models.Warehouse) (models.Warehouse, error)
	GetWarehouse(ctx context.Context, t tenant.Context, id string) (models.Warehouse, error)
	UpdateWarehouse(ctx context.Context, t tenant.Context, id string, expected int64, v models.Warehouse, archive bool) (models.Warehouse, error)
	ListLocations(ctx context.Context, t tenant.Context, warehouseID string) ([]models.Location, error)
	CreateLocation(ctx context.Context, t tenant.Context, warehouseID string, v models.Location) (models.Location, error)
	GetLocation(ctx context.Context, t tenant.Context, warehouseID, id string) (models.Location, error)
	UpdateLocation(ctx context.Context, t tenant.Context, warehouseID, id string, expected int64, v models.Location) (models.Location, error)
	ListItems(ctx context.Context, t tenant.Context) ([]models.Item, error)
	Receipt(ctx context.Context, t tenant.Context, in models.ReceiptInput) (models.Item, error)
	Move(ctx context.Context, t tenant.Context, id string, in models.MovementInput) (models.Movement, error)
	GetItem(ctx context.Context, t tenant.Context, id string) (models.Item, error)
	Movements(ctx context.Context, t tenant.Context, id string) ([]models.Movement, error)
	ListFulfilment(ctx context.Context, t tenant.Context) ([]models.FulfilmentOrder, error)
	CreateFulfilment(ctx context.Context, t tenant.Context, v models.FulfilmentOrder) (models.FulfilmentOrder, error)
	GetFulfilment(ctx context.Context, t tenant.Context, id string) (models.FulfilmentOrder, error)
	UpdateFulfilment(ctx context.Context, t tenant.Context, id string, expected int64, v models.FulfilmentOrder) (models.FulfilmentOrder, error)
	FulfilmentCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, in models.FulfilmentOrder) (models.FulfilmentOrder, error)
	ListDocks(ctx context.Context, t tenant.Context) ([]models.DockAppointment, error)
	CreateDock(ctx context.Context, t tenant.Context, v models.DockAppointment) (models.DockAppointment, error)
	GetDock(ctx context.Context, t tenant.Context, id string) (models.DockAppointment, error)
	DockCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, v models.DockAppointment) (models.DockAppointment, error)
	ListYards(ctx context.Context, t tenant.Context) ([]models.YardVisit, error)
	CreateYard(ctx context.Context, t tenant.Context, v models.YardVisit) (models.YardVisit, error)
	GetYard(ctx context.Context, t tenant.Context, id string) (models.YardVisit, error)
	YardCommand(ctx context.Context, t tenant.Context, id, command, reason, newSeal string, expected int64) (models.YardVisit, error)
	ListTransfers(ctx context.Context, t tenant.Context) ([]models.TerminalTransfer, error)
	CreateTransfer(ctx context.Context, t tenant.Context, v models.TerminalTransfer) (models.TerminalTransfer, error)
	GetTransfer(ctx context.Context, t tenant.Context, id string) (models.TerminalTransfer, error)
	TransferCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, actual json.RawMessage) (models.TerminalTransfer, error)
	ListReturns(ctx context.Context, t tenant.Context) ([]models.Return, error)
	CreateReturn(ctx context.Context, t tenant.Context, v models.Return) (models.Return, error)
	GetReturn(ctx context.Context, t tenant.Context, id string) (models.Return, error)
	ReturnCommand(ctx context.Context, t tenant.Context, id, command, disposition, reason string, expected int64) (models.Return, error)
}
