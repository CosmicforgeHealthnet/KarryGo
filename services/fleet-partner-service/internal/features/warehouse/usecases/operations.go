package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) CreateDock(ctx context.Context, t tenant.Context, v models.DockAppointment) (models.DockAppointment, error) {
	if e := s.gate(s.o.Flags.Dock, true); e != nil {
		return v, e
	}
	if _, e := s.GetWarehouse(ctx, t, v.WarehouseID); e != nil {
		return v, e
	}
	l, e := s.GetLocation(ctx, t, v.WarehouseID, v.DockLocationID)
	if e != nil || l.LocationType != "DOCK" {
		return v, apperrors.NotFound("Dock location not found.", nil)
	}
	if v.WindowStart.IsZero() || !v.WindowEnd.After(v.WindowStart) || v.VehicleOrTruckReference == "" {
		return v, apperrors.BadRequest("Vehicle and valid appointment window are required.", nil)
	}
	v.ID = ids.New("dck")
	v.OrganizationID = t.OrganizationID
	e = s.tx(ctx, func(tx pgx.Tx) error {
		s.repo.LockDock(ctx, tx, t.OrganizationID, v.DockLocationID)
		if s.repo.DockConflict(ctx, tx, t.OrganizationID, v.DockLocationID, v.WindowStart, v.WindowEnd) {
			return apperrors.Conflict("Dock capacity conflicts with another appointment.", nil)
		}
		var x error
		v, x = s.repo.InsertDock(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dock_appointment.created.v1", "dock_appointment", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetDock(ctx context.Context, t tenant.Context, id string) (models.DockAppointment, error) {
	if e := s.gate(s.o.Flags.Dock, false); e != nil {
		return models.DockAppointment{}, e
	}
	v, e := s.repo.GetDock(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Dock appointment not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListDocks(ctx context.Context, t tenant.Context) ([]models.DockAppointment, error) {
	if e := s.gate(s.o.Flags.Dock, false); e != nil {
		return nil, e
	}
	return s.repo.ListDocks(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) DockCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, v models.DockAppointment) (models.DockAppointment, error) {
	old, e := s.GetDock(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	status := old.Status
	switch command {
	case "UPDATE":
		status = v.Status
		if status == "" {
			status = old.Status
		}
	case "CHECK_IN":
		if old.Status != "CONFIRMED" && old.Status != "REQUESTED" {
			return v, apperrors.Conflict("Appointment cannot check in.", nil)
		}
		status = "CHECKED_IN"
	case "COMPLETE":
		if old.Status == "CANCELLED" || old.Status == "NO_SHOW" {
			return v, apperrors.Conflict("Appointment cannot complete.", nil)
		}
		status = "COMPLETED"
	default:
		return v, apperrors.BadRequest("Unsupported dock command.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateDock(ctx, tx, t.OrganizationID, id, expected, command, status)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dock_appointment."+strings.ToLower(status)+".v1", "dock_appointment", id, v.Version, map[string]any{"appointmentId": id, "status": status})
	})
	return v, e
}

func (s *Service) CreateYard(ctx context.Context, t tenant.Context, v models.YardVisit) (models.YardVisit, error) {
	if e := s.gate(s.o.Flags.Yard, true); e != nil {
		return v, e
	}
	if _, e := s.GetWarehouse(ctx, t, v.WarehouseID); e != nil {
		return v, e
	}
	if v.ProviderReference == "" || v.VehicleOrTruckReference == "" {
		return v, apperrors.BadRequest("Provider and vehicle references are required.", nil)
	}
	v.ID = ids.New("yrd")
	v.OrganizationID = t.OrganizationID
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertYard(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "yard_visit.created.v1", "yard_visit", v.ID, v.Version, map[string]any{"yardVisitId": v.ID, "warehouseId": v.WarehouseID})
	})
	return v, e
}
func (s *Service) GetYard(ctx context.Context, t tenant.Context, id string) (models.YardVisit, error) {
	if e := s.gate(s.o.Flags.Yard, false); e != nil {
		return models.YardVisit{}, e
	}
	v, e := s.repo.GetYard(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Yard visit not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListYards(ctx context.Context, t tenant.Context) ([]models.YardVisit, error) {
	if e := s.gate(s.o.Flags.Yard, false); e != nil {
		return nil, e
	}
	return s.repo.ListYards(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) YardCommand(ctx context.Context, t tenant.Context, id, command, reason, newSeal string, expected int64) (models.YardVisit, error) {
	old, e := s.GetYard(ctx, t, id)
	if e != nil {
		return old, e
	}
	if expected <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	status := old.Status
	security := old.SecurityStatus
	switch command {
	case "CHECK_IN":
		status = "GATE_CHECK_IN"
	case "SECURITY_HOLD":
		status = "SECURITY_HOLD"
		security = "HOLD"
	case "RELEASE":
		if security == "HOLD" && reason == "" {
			return old, apperrors.Conflict("Security hold requires an authorized reason.", nil)
		}
		status = "RELEASED"
		security = "CLEARED"
	case "CHECK_OUT":
		if old.Status != "RELEASED" {
			return old, apperrors.Conflict("Visit must be released before checkout.", nil)
		}
		status = "CHECKED_OUT"
	case "SEAL":
		if reason == "" || newSeal == "" {
			return old, apperrors.BadRequest("Seal change requires a value and reason.", nil)
		}
	default:
		return old, apperrors.BadRequest("Unsupported yard command.", nil)
	}
	var v models.YardVisit
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateYard(ctx, tx, t.OrganizationID, id, expected, status, security, newSeal)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		if x = s.repo.InsertYardHistory(ctx, tx, t.OrganizationID, id, old, v, reason, t.ActorID); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "yard_visit."+strings.ToLower(command)+".v1", "yard_visit", id, v.Version, map[string]any{"yardVisitId": id, "status": v.Status, "securityStatus": v.SecurityStatus})
	})
	return v, e
}

func (s *Service) CreateTransfer(ctx context.Context, t tenant.Context, v models.TerminalTransfer) (models.TerminalTransfer, error) {
	if e := s.gate(s.o.Flags.Terminal, true); e != nil {
		return v, e
	}
	if v.OriginWarehouseID == v.DestinationWarehouseID || v.ManifestID == "" {
		return v, apperrors.BadRequest("Distinct warehouses and manifest are required.", nil)
	}
	if _, e := s.GetWarehouse(ctx, t, v.OriginWarehouseID); e != nil {
		return v, e
	}
	if _, e := s.GetWarehouse(ctx, t, v.DestinationWarehouseID); e != nil {
		return v, e
	}
	if !s.repo.ManifestExists(ctx, t.OrganizationID, v.ManifestID) {
		return v, apperrors.NotFound("Manifest not found.", nil)
	}
	v.ID = ids.New("trf")
	v.OrganizationID = t.OrganizationID
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertTransfer(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "terminal_transfer.created.v1", "terminal_transfer", v.ID, v.Version, map[string]any{"transferId": v.ID, "manifestId": v.ManifestID})
	})
	return v, e
}
func (s *Service) GetTransfer(ctx context.Context, t tenant.Context, id string) (models.TerminalTransfer, error) {
	if e := s.gate(s.o.Flags.Terminal, false); e != nil {
		return models.TerminalTransfer{}, e
	}
	v, e := s.repo.GetTransfer(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Terminal transfer not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.OriginWarehouseID)
		if e == nil {
			_, e = s.GetWarehouse(ctx, t, v.DestinationWarehouseID)
		}
	}
	return v, e
}
func (s *Service) ListTransfers(ctx context.Context, t tenant.Context) ([]models.TerminalTransfer, error) {
	if e := s.gate(s.o.Flags.Terminal, false); e != nil {
		return nil, e
	}
	return s.repo.ListTransfers(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) TransferCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, actual json.RawMessage) (models.TerminalTransfer, error) {
	old, e := s.GetTransfer(ctx, t, id)
	if e != nil {
		return old, e
	}
	if expected <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var v models.TerminalTransfer
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var snapshotType, next string
		switch command {
		case "DEPART":
			if old.Status != "DRAFT" && old.Status != "READY" {
				return apperrors.Conflict("Transfer cannot depart.", nil)
			}
			snapshotType = "ORIGIN"
			next = "DEPARTED"
		case "ARRIVE":
			if old.Status != "DEPARTED" && old.Status != "IN_TRANSIT" {
				return apperrors.Conflict("Transfer cannot arrive.", nil)
			}
			snapshotType = "DESTINATION"
			next = "ARRIVED"
		case "RECONCILE":
			if old.Status != "ARRIVED" && old.Status != "DISCREPANCY" {
				return apperrors.Conflict("Transfer is not ready for reconciliation.", nil)
			}
			origin, x := s.repo.TransferSnapshot(ctx, tx, id, "ORIGIN")
			if x != nil {
				return x
			}
			dest, x := s.repo.TransferSnapshot(ctx, tx, id, "DESTINATION")
			if x != nil {
				return x
			}
			if string(origin) == string(dest) {
				next = "COMPLETED"
			} else {
				next = "DISCREPANCY"
				s.repo.InsertDiscrepancy(ctx, tx, t.OrganizationID, id, origin, dest)
			}
		default:
			return apperrors.BadRequest("Unsupported transfer command.", nil)
		}
		snapRef := ""
		if snapshotType != "" {
			snapRef = ids.New("tss")
			pkg := actual
			if len(pkg) == 0 {
				var x error
				if pkg, x = s.repo.ManifestPackageSnapshot(ctx, tx, old.ManifestID); x != nil {
					return x
				}
			}
			if x := s.repo.InsertTransferSnapshot(ctx, tx, snapRef, t.OrganizationID, id, snapshotType, pkg, t.ActorID); x != nil {
				return x
			}
		}
		var x error
		v, x = s.repo.UpdateTransfer(ctx, tx, t.OrganizationID, id, expected, next, snapshotType, snapRef)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "terminal_transfer."+strings.ToLower(next)+".v1", "terminal_transfer", id, v.Version, map[string]any{"transferId": id, "status": next})
	})
	return v, e
}

func (s *Service) CreateReturn(ctx context.Context, t tenant.Context, v models.Return) (models.Return, error) {
	if e := s.gate(s.o.Flags.Returns, true); e != nil {
		return v, e
	}
	if _, e := s.GetWarehouse(ctx, t, v.WarehouseID); e != nil {
		return v, e
	}
	if v.ReasonCode == "" || (v.SourceOrderID == "" && v.SourceShipmentID == "" && v.PackageID == "") {
		return v, apperrors.BadRequest("Return source and reason are required.", nil)
	}
	v.ID = ids.New("rtn")
	v.OrganizationID = t.OrganizationID
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertReturn(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "return.created.v1", "return", v.ID, v.Version, map[string]any{"returnId": v.ID, "warehouseId": v.WarehouseID, "financialReference": v.FinancialReference})
	})
	return v, e
}
func (s *Service) GetReturn(ctx context.Context, t tenant.Context, id string) (models.Return, error) {
	if e := s.gate(s.o.Flags.Returns, false); e != nil {
		return models.Return{}, e
	}
	v, e := s.repo.GetReturn(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Return not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListReturns(ctx context.Context, t tenant.Context) ([]models.Return, error) {
	if e := s.gate(s.o.Flags.Returns, false); e != nil {
		return nil, e
	}
	return s.repo.ListReturns(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) ReturnCommand(ctx context.Context, t tenant.Context, id, command, disposition, reason string, expected int64) (models.Return, error) {
	old, e := s.GetReturn(ctx, t, id)
	if e != nil {
		return old, e
	}
	if expected <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	next := old.Status
	switch command {
	case "RECEIVE":
		if old.Status != "REQUESTED" {
			return old, apperrors.Conflict("Return cannot be received.", nil)
		}
		next = "RECEIVED"
	case "DECIDE":
		if old.Status != "RECEIVED" && old.Status != "INSPECTING" {
			return old, apperrors.Conflict("Return is not ready for disposition.", nil)
		}
		if disposition == "" || reason == "" {
			return old, apperrors.BadRequest("Disposition and reason are required.", nil)
		}
		next = "DECIDED"
	case "COMPLETE":
		if old.Status != "DECIDED" {
			return old, apperrors.Conflict("Return must have a disposition.", nil)
		}
		next = "COMPLETED"
	default:
		return old, apperrors.BadRequest("Unsupported return command.", nil)
	}
	var v models.Return
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if command == "RECEIVE" && old.PackageID != "" {
			shipment, x := s.repo.PackageShipment(ctx, tx, t.OrganizationID, old.PackageID)
			if x != nil {
				return x
			}
			if x = s.repo.InsertReturnCustody(ctx, tx, t.OrganizationID, old, shipment); x != nil {
				return x
			}
		}
		if command == "RECEIVE" && old.InventoryItemID != "" {
			qty, loc, x := s.repo.LockItemForReturn(ctx, tx, t.OrganizationID, old.InventoryItemID)
			if x != nil {
				return x
			}
			after := qty + 1
			stockStatus := "RETURNED"
			if strings.EqualFold(old.Condition, "DAMAGED") || strings.EqualFold(old.Condition, "RESTRICTED") {
				stockStatus = "QUARANTINED"
			}
			if x = s.repo.SetItemQuantityAndStatus(ctx, tx, t.OrganizationID, old.InventoryItemID, after, stockStatus); x != nil {
				return x
			}
			if x = s.repo.InsertReturnMovement(ctx, tx, t.OrganizationID, old, loc, t.ActorID, qty, after); x != nil {
				return x
			}
		}
		var x error
		v, x = s.repo.UpdateReturn(ctx, tx, t.OrganizationID, id, expected, next, strings.ToUpper(disposition))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "return."+strings.ToLower(next)+".v1", "return", id, v.Version, map[string]any{"returnId": id, "status": next, "disposition": v.Disposition, "financialReference": v.FinancialReference})
	})
	return v, e
}
