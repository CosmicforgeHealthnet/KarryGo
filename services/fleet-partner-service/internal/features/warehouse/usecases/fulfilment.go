package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) CreateFulfilment(ctx context.Context, t tenant.Context, v models.FulfilmentOrder) (models.FulfilmentOrder, error) {
	if e := s.gate(s.o.Flags.Fulfilment, true); e != nil {
		return v, e
	}
	w, e := s.GetWarehouse(ctx, t, v.WarehouseID)
	if e != nil {
		return v, e
	}
	if w.Status != "ACTIVE" {
		return v, apperrors.Conflict("Warehouse is not active.", nil)
	}
	if v.CustomerReference == "" || len(v.Lines) == 0 {
		return v, apperrors.BadRequest("Customer reference and order lines are required.", nil)
	}
	v.ID = ids.New("ful")
	v.OrganizationID = t.OrganizationID
	if v.Priority == "" {
		v.Priority = "NORMAL"
	}
	v.PackingRequirements = models.Raw(v.PackingRequirements, "{}")
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertFulfilmentOrder(ctx, tx, v)
		if x != nil {
			return x
		}
		for i := range v.Lines {
			l := &v.Lines[i]
			if l.InventoryItemID == "" || l.RequestedQuantity <= 0 {
				return apperrors.BadRequest("Every line needs an inventory item and positive quantity.", nil)
			}
			if wh, x := s.repo.ItemWarehouse(ctx, tx, t.OrganizationID, l.InventoryItemID); x != nil || wh != v.WarehouseID {
				return apperrors.NotFound("Inventory item not found in warehouse.", nil)
			}
			l.ID = ids.New("fuline")
			l.OrganizationID = t.OrganizationID
			l.FulfilmentOrderID = v.ID
			if x = s.repo.InsertFulfilmentLine(ctx, tx, l); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "fulfilment_order.created.v1", "fulfilment_order", v.ID, v.Version, map[string]any{"fulfilmentOrderId": v.ID, "warehouseId": v.WarehouseID})
	})
	return v, e
}
func (s *Service) GetFulfilment(ctx context.Context, t tenant.Context, id string) (models.FulfilmentOrder, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return models.FulfilmentOrder{}, e
	}
	v, e := s.repo.GetFulfilmentOrder(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Fulfilment order not found.", nil)
	}
	if e != nil {
		return v, e
	}
	w, e := s.GetWarehouse(ctx, t, v.WarehouseID)
	if e != nil || w.ID == "" {
		return v, apperrors.NotFound("Fulfilment order not found.", nil)
	}
	e = s.repo.LoadLines(ctx, &v)
	return v, e
}
func (s *Service) ListFulfilment(ctx context.Context, t tenant.Context) ([]models.FulfilmentOrder, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return nil, e
	}
	return s.repo.ListFulfilmentOrders(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) UpdateFulfilment(ctx context.Context, t tenant.Context, id string, expected int64, v models.FulfilmentOrder) (models.FulfilmentOrder, error) {
	old, e := s.GetFulfilment(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if old.Status != "DRAFT" {
		return v, apperrors.Conflict("Only draft fulfilment orders can be edited.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateFulfilmentOrder(ctx, tx, t.OrganizationID, id, expected, v)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "fulfilment_order.updated.v1", "fulfilment_order", id, v.Version, map[string]any{"fulfilmentOrderId": id})
	})
	return v, e
}
func (s *Service) FulfilmentCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, in models.FulfilmentOrder) (models.FulfilmentOrder, error) {
	if expected <= 0 {
		return in, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if command == "RELEASE" || command == "DISPATCH" {
		current, e := s.GetFulfilment(ctx, t, id)
		if e != nil {
			return in, e
		}
		if e = s.requireUnrestricted(ctx, t, "WAREHOUSE", current.WarehouseID); e != nil {
			return in, e
		}
	}
	org := t.OrganizationID
	var out models.FulfilmentOrder
	e := s.tx(ctx, func(tx pgx.Tx) error {
		order, x := s.repo.LockFulfilmentOrder(ctx, tx, org, id)
		if x != nil {
			return apperrors.NotFound("Fulfilment order not found.", nil)
		}
		if order.Version != expected {
			return apperrors.Conflict("Version conflict.", nil)
		}
		lines, x := s.repo.LockFulfilmentLines(ctx, tx, org, id)
		if x != nil {
			return x
		}
		status := order.Status
		next := ""
		switch command {
		case "RELEASE":
			if status != "DRAFT" {
				return apperrors.Conflict("Only draft orders can be released.", nil)
			}
			for _, l := range lines {
				qty, res, state, expiry, x := s.repo.LockItemStock(ctx, tx, org, l.InventoryItemID)
				if x != nil {
					return x
				}
				if state == "QUARANTINED" || state == "DAMAGED" || state == "EXPIRED" || expiry != nil && !expiry.After(s.o.Now()) || qty-res < l.Requested {
					return apperrors.Conflict("Required stock is unavailable or restricted.", nil)
				}
				if x = s.repo.ReserveItem(ctx, tx, org, l.InventoryItemID, l.Requested); x != nil {
					return x
				}
				if x = s.repo.InsertReservation(ctx, tx, org, l.InventoryItemID, id, l.Requested, in.RequestedDispatchAt); x != nil {
					return x
				}
			}
			next = "RELEASED"
		case "PICK":
			if status != "RELEASED" && status != "PICKING" {
				return apperrors.Conflict("Order is not releasable for picking.", nil)
			}
			for _, l := range lines {
				if _, x = s.repo.LockItemQuantity(ctx, tx, org, l.InventoryItemID); x != nil {
					return x
				}
				if x = s.repo.SetItemStatus(ctx, tx, l.InventoryItemID, "PICKING"); x != nil {
					return x
				}
				if x = s.repo.MarkLinePicked(ctx, tx, l.ID); x != nil {
					return x
				}
				if x = s.repo.SetReservationsStatus(ctx, tx, org, l.InventoryItemID, id, "PICKED"); x != nil {
					return x
				}
				if x = s.repo.InsertPickMovement(ctx, tx, org, l.InventoryItemID, id, t.ActorID, l.Requested); x != nil {
					return x
				}
			}
			next = "PICKED"
		case "PACK":
			if status != "PICKED" && status != "PACKING" {
				return apperrors.Conflict("Order is not ready for packing.", nil)
			}
			pkg := map[string]string{}
			for _, l := range in.Lines {
				pkg[l.InventoryItemID] = l.PackageReference
			}
			for _, l := range lines {
				ref := pkg[l.InventoryItemID]
				if ref == "" {
					return apperrors.BadRequest("Every picked line requires a package reference.", nil)
				}
				if !s.repo.PackageExists(ctx, tx, org, ref) {
					return apperrors.NotFound("Package not found.", nil)
				}
				if x = s.repo.SetLinePackage(ctx, tx, l.ID, ref); x != nil {
					return x
				}
				if x = s.repo.SetItemStatus(ctx, tx, l.InventoryItemID, "PACKED"); x != nil {
					return x
				}
			}
			next = "PACKED"
		case "DISPATCH":
			if status != "PACKED" && status != "READY_FOR_DISPATCH" {
				return apperrors.Conflict("Order is not ready for dispatch.", nil)
			}
			if order.ProofPolicyReference != "" {
				if s.o.Proof == nil {
					return apperrors.Unavailable("Proof policy validation is unavailable.", nil)
				}
				ok, e := s.o.Proof.MandatorySatisfied(ctx, org, order.ProofPolicyReference, id, order.ShipmentReference, "", "")
				if e != nil {
					return e
				}
				if !ok {
					return apperrors.Conflict("Mandatory proof is missing.", nil)
				}
			}
			for _, l := range lines {
				qty, res, loc, x := s.repo.LockItemQuantityAndLocation(ctx, tx, org, l.InventoryItemID)
				if x != nil {
					return x
				}
				if qty < l.Requested || res < l.Requested {
					return apperrors.Conflict("Reserved stock changed before dispatch.", nil)
				}
				after := qty - l.Requested
				state := "AVAILABLE"
				if after == 0 {
					state = "DISPATCHED"
				}
				if x = s.repo.DispatchItem(ctx, tx, org, l.InventoryItemID, after, l.Requested, state); x != nil {
					return x
				}
				if x = s.repo.InsertDispatchMovement(ctx, tx, org, l.InventoryItemID, loc, id, t.ActorID, l.Requested, qty, after); x != nil {
					return x
				}
			}
			next = "DISPATCHED"
		case "CANCEL":
			if status != "DRAFT" && status != "RELEASED" {
				return apperrors.Conflict("Only draft or safely reserved orders can be cancelled.", nil)
			}
			if status == "RELEASED" {
				for _, l := range lines {
					if x = s.repo.ReleaseItemReservation(ctx, tx, org, l.InventoryItemID, l.Requested); x != nil {
						return x
					}
					if x = s.repo.SetReservationsStatus(ctx, tx, org, l.InventoryItemID, id, "RELEASED"); x != nil {
						return x
					}
				}
			}
			next = "CANCELLED"
		default:
			return apperrors.BadRequest("Unsupported fulfilment command.", nil)
		}
		out, x = s.repo.TransitionFulfilmentOrder(ctx, tx, org, id, expected, next, in.ShipmentReference, in.ManifestReference)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "fulfilment_order."+strings.ToLower(next)+".v1", "fulfilment_order", id, out.Version, map[string]any{"fulfilmentOrderId": id, "status": next})
	})
	if e == nil {
		_ = s.repo.LoadLines(ctx, &out)
	}
	return out, e
}
