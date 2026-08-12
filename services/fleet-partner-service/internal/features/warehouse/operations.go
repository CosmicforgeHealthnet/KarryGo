package warehouse

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

const orderCols = `id,organization_id,warehouse_id,customer_reference,source_order_id,priority,status,requested_dispatch_at,assigned_team_reference,packing_requirements,proof_policy_reference,shipment_reference,manifest_reference,version,created_at,updated_at,completed_at`

func scanOrder(r pgx.Row) (FulfilmentOrder, error) {
	var v FulfilmentOrder
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.CustomerReference, &v.SourceOrderID, &v.Priority, &v.Status, &v.RequestedDispatchAt, &v.AssignedTeamReference, &v.PackingRequirements, &v.ProofPolicyReference, &v.ShipmentReference, &v.ManifestReference, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, e
}
func (s *Service) loadLines(ctx context.Context, order *FulfilmentOrder) error {
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,fulfilment_order_id,inventory_item_id,requested_quantity,picked_quantity,package_reference,version,created_at,updated_at FROM fleet_partner_fulfilment_lines WHERE organization_id=$1 AND fulfilment_order_id=$2 ORDER BY id`, order.OrganizationID, order.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var v FulfilmentLine
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.FulfilmentOrderID, &v.InventoryItemID, &v.RequestedQuantity, &v.PickedQuantity, &v.PackageReference, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return e
		}
		order.Lines = append(order.Lines, v)
	}
	return rows.Err()
}
func (s *Service) CreateFulfilment(ctx context.Context, t tenant.Context, v FulfilmentOrder) (FulfilmentOrder, error) {
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
	v.PackingRequirements = raw(v.PackingRequirements, "{}")
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanOrder(tx.QueryRow(ctx, `INSERT INTO fleet_partner_fulfilment_orders(id,organization_id,warehouse_id,customer_reference,source_order_id,priority,requested_dispatch_at,assigned_team_reference,packing_requirements,proof_policy_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+orderCols, v.ID, v.OrganizationID, v.WarehouseID, v.CustomerReference, v.SourceOrderID, v.Priority, v.RequestedDispatchAt, v.AssignedTeamReference, v.PackingRequirements, v.ProofPolicyReference))
		if x != nil {
			return x
		}
		for i := range v.Lines {
			l := &v.Lines[i]
			if l.InventoryItemID == "" || l.RequestedQuantity <= 0 {
				return apperrors.BadRequest("Every line needs an inventory item and positive quantity.", nil)
			}
			var wh string
			if x = tx.QueryRow(ctx, `SELECT warehouse_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2`, t.OrganizationID, l.InventoryItemID).Scan(&wh); x != nil || wh != v.WarehouseID {
				return apperrors.NotFound("Inventory item not found in warehouse.", nil)
			}
			l.ID = ids.New("fuline")
			l.OrganizationID = t.OrganizationID
			l.FulfilmentOrderID = v.ID
			x = tx.QueryRow(ctx, `INSERT INTO fleet_partner_fulfilment_lines(id,organization_id,fulfilment_order_id,inventory_item_id,requested_quantity)VALUES($1,$2,$3,$4,$5) RETURNING version,created_at,updated_at`, l.ID, l.OrganizationID, l.FulfilmentOrderID, l.InventoryItemID, l.RequestedQuantity).Scan(&l.Version, &l.CreatedAt, &l.UpdatedAt)
			if x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "fulfilment_order.created.v1", "fulfilment_order", v.ID, v.Version, map[string]any{"fulfilmentOrderId": v.ID, "warehouseId": v.WarehouseID})
	})
	return v, e
}
func (s *Service) GetFulfilment(ctx context.Context, t tenant.Context, id string) (FulfilmentOrder, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return FulfilmentOrder{}, e
	}
	v, e := scanOrder(s.o.Pool.QueryRow(ctx, `SELECT `+orderCols+` FROM fleet_partner_fulfilment_orders WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
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
	e = s.loadLines(ctx, &v)
	return v, e
}
func (s *Service) ListFulfilment(ctx context.Context, t tenant.Context) ([]FulfilmentOrder, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+orderCols+` FROM fleet_partner_fulfilment_orders f WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=f.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FulfilmentOrder{}
	for rows.Next() {
		v, e := scanOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) UpdateFulfilment(ctx context.Context, t tenant.Context, id string, expected int64, v FulfilmentOrder) (FulfilmentOrder, error) {
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
		v, x = scanOrder(tx.QueryRow(ctx, `UPDATE fleet_partner_fulfilment_orders SET priority=COALESCE(NULLIF($4,''),priority),requested_dispatch_at=COALESCE($5,requested_dispatch_at),assigned_team_reference=COALESCE(NULLIF($6,''),assigned_team_reference),packing_requirements=CASE WHEN $7::jsonb='{}' THEN packing_requirements ELSE $7 END,proof_policy_reference=COALESCE(NULLIF($8,''),proof_policy_reference),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+orderCols, t.OrganizationID, id, expected, v.Priority, v.RequestedDispatchAt, v.AssignedTeamReference, raw(v.PackingRequirements, "{}"), v.ProofPolicyReference))
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
func (s *Service) FulfilmentCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, in FulfilmentOrder) (FulfilmentOrder, error) {
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
	var out FulfilmentOrder
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var status, warehouse, proofRef, shipment string
		var version int64
		if x := tx.QueryRow(ctx, `SELECT status,version,warehouse_id,proof_policy_reference,shipment_reference FROM fleet_partner_fulfilment_orders WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&status, &version, &warehouse, &proofRef, &shipment); x != nil {
			return apperrors.NotFound("Fulfilment order not found.", nil)
		}
		if version != expected {
			return apperrors.Conflict("Version conflict.", nil)
		}
		rows, x := tx.Query(ctx, `SELECT id,inventory_item_id,requested_quantity,picked_quantity,package_reference FROM fleet_partner_fulfilment_lines WHERE organization_id=$1 AND fulfilment_order_id=$2 ORDER BY id FOR UPDATE`, t.OrganizationID, id)
		if x != nil {
			return x
		}
		type line struct {
			id, item, pkg     string
			requested, picked float64
		}
		var lines []line
		for rows.Next() {
			var l line
			if x = rows.Scan(&l.id, &l.item, &l.requested, &l.picked, &l.pkg); x != nil {
				rows.Close()
				return x
			}
			lines = append(lines, l)
		}
		rows.Close()
		next := ""
		switch command {
		case "RELEASE":
			if status != "DRAFT" {
				return apperrors.Conflict("Only draft orders can be released.", nil)
			}
			for _, l := range lines {
				var qty, res float64
				var state string
				var expiry *time.Time
				if x = tx.QueryRow(ctx, `SELECT quantity_projection,reserved_quantity,status,expiry_at FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, l.item).Scan(&qty, &res, &state, &expiry); x != nil {
					return x
				}
				if state == "QUARANTINED" || state == "DAMAGED" || state == "EXPIRED" || expiry != nil && !expiry.After(s.o.Now()) || qty-res < l.requested {
					return apperrors.Conflict("Required stock is unavailable or restricted.", nil)
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=reserved_quantity+$3,status='RESERVED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, l.item, l.requested)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_reservations(id,organization_id,inventory_item_id,fulfilment_order_id,quantity,expires_at)VALUES($1,$2,$3,$4,$5,COALESCE($6,now()+interval '24 hours'))`, ids.New("res"), t.OrganizationID, l.item, id, l.requested, in.RequestedDispatchAt)
				if x != nil {
					return x
				}
			}
			next = "RELEASED"
		case "PICK":
			if status != "RELEASED" && status != "PICKING" {
				return apperrors.Conflict("Order is not releasable for picking.", nil)
			}
			for _, l := range lines {
				var before float64
				if x = tx.QueryRow(ctx, `SELECT quantity_projection FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, l.item).Scan(&before); x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET status='PICKING',version=version+1,updated_at=now() WHERE id=$1`, l.item)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_fulfilment_lines SET picked_quantity=requested_quantity,version=version+1,updated_at=now() WHERE id=$1`, l.id)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status='PICKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND inventory_item_id=$2 AND fulfilment_order_id=$3 AND status='ACTIVE'`, t.OrganizationID, l.item, id)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'PICK',location_id,$3,'FULFILMENT_PICK',$4,$5,$6,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$7`, ids.New("imv"), t.OrganizationID, l.requested, t.ActorID, id, id+":pick:"+l.item, l.item)
				if x != nil {
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
				ref := pkg[l.item]
				if ref == "" {
					return apperrors.BadRequest("Every picked line requires a package reference.", nil)
				}
				var owned bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, ref).Scan(&owned)
				if !owned {
					return apperrors.NotFound("Package not found.", nil)
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_fulfilment_lines SET package_reference=$2,version=version+1,updated_at=now() WHERE id=$1`, l.id, ref)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET status='PACKED',version=version+1,updated_at=now() WHERE id=$1`, l.item)
				if x != nil {
					return x
				}
			}
			next = "PACKED"
		case "DISPATCH":
			if status != "PACKED" && status != "READY_FOR_DISPATCH" {
				return apperrors.Conflict("Order is not ready for dispatch.", nil)
			}
			if proofRef != "" {
				if s.o.Proof == nil {
					return apperrors.Unavailable("Proof policy validation is unavailable.", nil)
				}
				ok, e := s.o.Proof.MandatorySatisfied(ctx, t.OrganizationID, proofRef, id, shipment, "", "")
				if e != nil {
					return e
				}
				if !ok {
					return apperrors.Conflict("Mandatory proof is missing.", nil)
				}
			}
			for _, l := range lines {
				var qty, res float64
				var loc string
				if x = tx.QueryRow(ctx, `SELECT quantity_projection,reserved_quantity,location_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, l.item).Scan(&qty, &res, &loc); x != nil {
					return x
				}
				if qty < l.requested || res < l.requested {
					return apperrors.Conflict("Reserved stock changed before dispatch.", nil)
				}
				after := qty - l.requested
				state := "AVAILABLE"
				if after == 0 {
					state = "DISPATCHED"
				}
				_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=$3,reserved_quantity=reserved_quantity-$4,status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, l.item, after, l.requested, state)
				if x != nil {
					return x
				}
				_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'DISPATCH',$4,$5,'FULFILMENT_DISPATCH',$6,$7,$8,$9,$10)`, ids.New("imv"), t.OrganizationID, l.item, loc, l.requested, t.ActorID, id, id+":dispatch:"+l.item, qty, after)
				if x != nil {
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
					_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=GREATEST(0,reserved_quantity-$3),status=CASE WHEN reserved_quantity-$3<=0 THEN 'AVAILABLE' ELSE status END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, l.item, l.requested)
					if x != nil {
						return x
					}
					_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status='RELEASED',version=version+1,updated_at=now() WHERE organization_id=$1 AND inventory_item_id=$2 AND fulfilment_order_id=$3 AND status='ACTIVE'`, t.OrganizationID, l.item, id)
					if x != nil {
						return x
					}
				}
			}
			next = "CANCELLED"
		default:
			return apperrors.BadRequest("Unsupported fulfilment command.", nil)
		}
		out, x = scanOrder(tx.QueryRow(ctx, `UPDATE fleet_partner_fulfilment_orders SET status=$4,shipment_reference=COALESCE(NULLIF($5,''),shipment_reference),manifest_reference=COALESCE(NULLIF($6,''),manifest_reference),version=version+1,updated_at=now(),completed_at=CASE WHEN $4 IN('DISPATCHED','COMPLETED','CANCELLED') THEN now() ELSE completed_at END WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+orderCols, t.OrganizationID, id, expected, next, in.ShipmentReference, in.ManifestReference))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "fulfilment_order."+strings.ToLower(next)+".v1", "fulfilment_order", id, out.Version, map[string]any{"fulfilmentOrderId": id, "status": next})
	})
	if e == nil {
		_ = s.loadLines(ctx, &out)
	}
	return out, e
}

const dockCols = `id,organization_id,warehouse_id,dock_location_id,carrier_reference,provider_reference,vehicle_or_truck_reference,appointment_type,window_start,window_end,status,checked_in_at,loading_started_at,loading_completed_at,checked_out_at,version,created_at,updated_at`

func scanDock(r pgx.Row) (DockAppointment, error) {
	var v DockAppointment
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.DockLocationID, &v.CarrierReference, &v.ProviderReference, &v.VehicleOrTruckReference, &v.AppointmentType, &v.WindowStart, &v.WindowEnd, &v.Status, &v.CheckedInAt, &v.LoadingStartedAt, &v.LoadingCompletedAt, &v.CheckedOutAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateDock(ctx context.Context, t tenant.Context, v DockAppointment) (DockAppointment, error) {
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
		_, _ = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, t.OrganizationID+":"+v.DockLocationID)
		var conflict bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_dock_appointments WHERE organization_id=$1 AND dock_location_id=$2 AND status NOT IN('COMPLETED','NO_SHOW','CANCELLED') AND window_start<$4 AND window_end>$3)`, t.OrganizationID, v.DockLocationID, v.WindowStart, v.WindowEnd).Scan(&conflict)
		if conflict {
			return apperrors.Conflict("Dock capacity conflicts with another appointment.", nil)
		}
		var x error
		v, x = scanDock(tx.QueryRow(ctx, `INSERT INTO fleet_partner_dock_appointments(id,organization_id,warehouse_id,dock_location_id,carrier_reference,provider_reference,vehicle_or_truck_reference,appointment_type,window_start,window_end)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+dockCols, v.ID, v.OrganizationID, v.WarehouseID, v.DockLocationID, v.CarrierReference, v.ProviderReference, v.VehicleOrTruckReference, v.AppointmentType, v.WindowStart, v.WindowEnd))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dock_appointment.created.v1", "dock_appointment", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetDock(ctx context.Context, t tenant.Context, id string) (DockAppointment, error) {
	if e := s.gate(s.o.Flags.Dock, false); e != nil {
		return DockAppointment{}, e
	}
	v, e := scanDock(s.o.Pool.QueryRow(ctx, `SELECT `+dockCols+` FROM fleet_partner_dock_appointments WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Dock appointment not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListDocks(ctx context.Context, t tenant.Context) ([]DockAppointment, error) {
	if e := s.gate(s.o.Flags.Dock, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+dockCols+` FROM fleet_partner_dock_appointments d WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=d.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY window_start`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DockAppointment{}
	for rows.Next() {
		v, e := scanDock(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) DockCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, v DockAppointment) (DockAppointment, error) {
	old, e := s.GetDock(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	status := old.Status
	set := ""
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
		set = "checked_in_at=now(),"
	case "COMPLETE":
		if old.Status == "CANCELLED" || old.Status == "NO_SHOW" {
			return v, apperrors.Conflict("Appointment cannot complete.", nil)
		}
		status = "COMPLETED"
		set = "loading_completed_at=COALESCE(loading_completed_at,now()),checked_out_at=now(),"
	default:
		return v, apperrors.BadRequest("Unsupported dock command.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanDock(tx.QueryRow(ctx, `UPDATE fleet_partner_dock_appointments SET `+set+`status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+dockCols, t.OrganizationID, id, expected, status))
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

const yardCols = `id,organization_id,warehouse_id,provider_reference,vehicle_or_truck_reference,gate_reference,bay_reference,seal_reference,security_status,status,checked_in_at,entered_yard_at,exited_yard_at,checked_out_at,version,created_at,updated_at`

func scanYard(r pgx.Row) (YardVisit, error) {
	var v YardVisit
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ProviderReference, &v.VehicleOrTruckReference, &v.GateReference, &v.BayReference, &v.SealReference, &v.SecurityStatus, &v.Status, &v.CheckedInAt, &v.EnteredYardAt, &v.ExitedYardAt, &v.CheckedOutAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateYard(ctx context.Context, t tenant.Context, v YardVisit) (YardVisit, error) {
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
	var e error
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanYard(tx.QueryRow(ctx, `INSERT INTO fleet_partner_yard_visits(id,organization_id,warehouse_id,provider_reference,vehicle_or_truck_reference,gate_reference,bay_reference,seal_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+yardCols, v.ID, v.OrganizationID, v.WarehouseID, v.ProviderReference, v.VehicleOrTruckReference, v.GateReference, v.BayReference, v.SealReference))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "yard_visit.created.v1", "yard_visit", v.ID, v.Version, map[string]any{"yardVisitId": v.ID, "warehouseId": v.WarehouseID})
	})
	return v, e
}
func (s *Service) GetYard(ctx context.Context, t tenant.Context, id string) (YardVisit, error) {
	if e := s.gate(s.o.Flags.Yard, false); e != nil {
		return YardVisit{}, e
	}
	v, e := scanYard(s.o.Pool.QueryRow(ctx, `SELECT `+yardCols+` FROM fleet_partner_yard_visits WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Yard visit not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListYards(ctx context.Context, t tenant.Context) ([]YardVisit, error) {
	if e := s.gate(s.o.Flags.Yard, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+yardCols+` FROM fleet_partner_yard_visits y WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=y.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []YardVisit{}
	for rows.Next() {
		v, e := scanYard(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) YardCommand(ctx context.Context, t tenant.Context, id, command, reason, newSeal string, expected int64) (YardVisit, error) {
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
	var v YardVisit
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanYard(tx.QueryRow(ctx, `UPDATE fleet_partner_yard_visits SET status=$4,security_status=$5,seal_reference=CASE WHEN $6<>'' THEN $6 ELSE seal_reference END,checked_in_at=CASE WHEN $4='GATE_CHECK_IN' THEN now() ELSE checked_in_at END,exited_yard_at=CASE WHEN $4='RELEASED' THEN now() ELSE exited_yard_at END,checked_out_at=CASE WHEN $4='CHECKED_OUT' THEN now() ELSE checked_out_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+yardCols, t.OrganizationID, id, expected, status, security, newSeal))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_yard_visit_history(id,organization_id,yard_visit_id,previous_status,new_status,previous_seal,new_seal,reason,actor_reference,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("yrh"), t.OrganizationID, id, old.Status, v.Status, old.SealReference, v.SealReference, reason, t.ActorID, v.Version)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "yard_visit."+strings.ToLower(command)+".v1", "yard_visit", id, v.Version, map[string]any{"yardVisitId": id, "status": v.Status, "securityStatus": v.SecurityStatus})
	})
	return v, e
}

const transferCols = `id,organization_id,origin_warehouse_id,destination_warehouse_id,manifest_id,route_reference,status,departed_at,arrived_at,origin_snapshot_reference,destination_snapshot_reference,reconciliation_status,version,created_at,updated_at`

func scanTransfer(r pgx.Row) (TerminalTransfer, error) {
	var v TerminalTransfer
	e := r.Scan(&v.ID, &v.OrganizationID, &v.OriginWarehouseID, &v.DestinationWarehouseID, &v.ManifestID, &v.RouteReference, &v.Status, &v.DepartedAt, &v.ArrivedAt, &v.OriginSnapshotReference, &v.DestinationSnapshotReference, &v.ReconciliationStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateTransfer(ctx context.Context, t tenant.Context, v TerminalTransfer) (TerminalTransfer, error) {
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
	var manifest bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_manifests WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, v.ManifestID).Scan(&manifest)
	if !manifest {
		return v, apperrors.NotFound("Manifest not found.", nil)
	}
	v.ID = ids.New("trf")
	v.OrganizationID = t.OrganizationID
	var e error
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanTransfer(tx.QueryRow(ctx, `INSERT INTO fleet_partner_terminal_transfers(id,organization_id,origin_warehouse_id,destination_warehouse_id,manifest_id,route_reference)VALUES($1,$2,$3,$4,$5,$6) RETURNING `+transferCols, v.ID, v.OrganizationID, v.OriginWarehouseID, v.DestinationWarehouseID, v.ManifestID, v.RouteReference))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "terminal_transfer.created.v1", "terminal_transfer", v.ID, v.Version, map[string]any{"transferId": v.ID, "manifestId": v.ManifestID})
	})
	return v, e
}
func (s *Service) GetTransfer(ctx context.Context, t tenant.Context, id string) (TerminalTransfer, error) {
	if e := s.gate(s.o.Flags.Terminal, false); e != nil {
		return TerminalTransfer{}, e
	}
	v, e := scanTransfer(s.o.Pool.QueryRow(ctx, `SELECT `+transferCols+` FROM fleet_partner_terminal_transfers WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
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
func (s *Service) ListTransfers(ctx context.Context, t tenant.Context) ([]TerminalTransfer, error) {
	if e := s.gate(s.o.Flags.Terminal, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+transferCols+` FROM fleet_partner_terminal_transfers tr WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=tr.origin_warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=tr.destination_warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TerminalTransfer{}
	for rows.Next() {
		v, e := scanTransfer(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) TransferCommand(ctx context.Context, t tenant.Context, id, command string, expected int64, actual json.RawMessage) (TerminalTransfer, error) {
	old, e := s.GetTransfer(ctx, t, id)
	if e != nil {
		return old, e
	}
	if expected <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	var v TerminalTransfer
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
			var origin, dest []byte
			if x := tx.QueryRow(ctx, `SELECT package_snapshot FROM fleet_partner_terminal_transfer_snapshots WHERE transfer_id=$1 AND snapshot_type='ORIGIN'`, id).Scan(&origin); x != nil {
				return x
			}
			if x := tx.QueryRow(ctx, `SELECT package_snapshot FROM fleet_partner_terminal_transfer_snapshots WHERE transfer_id=$1 AND snapshot_type='DESTINATION'`, id).Scan(&dest); x != nil {
				return x
			}
			if string(origin) == string(dest) {
				next = "COMPLETED"
			} else {
				next = "DISCREPANCY"
				_, _ = tx.Exec(ctx, `INSERT INTO fleet_partner_terminal_discrepancies(id,organization_id,transfer_id,discrepancy_type,expected_snapshot,actual_snapshot)VALUES($1,$2,$3,'PACKAGE_SNAPSHOT',$4,$5)`, ids.New("tds"), t.OrganizationID, id, origin, dest)
			}
		default:
			return apperrors.BadRequest("Unsupported transfer command.", nil)
		}
		snapRef := ""
		if snapshotType != "" {
			snapRef = ids.New("tss")
			pkg := actual
			if len(pkg) == 0 {
				if x := tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('packageId',p.id,'status',p.status) ORDER BY p.id),'[]') FROM fleet_partner_manifest_items mi JOIN fleet_partner_packages p ON p.id=mi.item_id WHERE mi.manifest_id=$1 AND mi.item_type='PACKAGE'`, old.ManifestID).Scan(&pkg); x != nil {
					return x
				}
			}
			_, x := tx.Exec(ctx, `INSERT INTO fleet_partner_terminal_transfer_snapshots(id,organization_id,transfer_id,snapshot_type,package_snapshot,inventory_snapshot,custody_snapshot,captured_by)VALUES($1,$2,$3,$4,$5,'[]','[]',$6)`, snapRef, t.OrganizationID, id, snapshotType, pkg, t.ActorID)
			if x != nil {
				return x
			}
		}
		var x error
		v, x = scanTransfer(tx.QueryRow(ctx, `UPDATE fleet_partner_terminal_transfers SET status=$4,departed_at=CASE WHEN $4='DEPARTED' THEN now() ELSE departed_at END,arrived_at=CASE WHEN $4='ARRIVED' THEN now() ELSE arrived_at END,origin_snapshot_reference=CASE WHEN $5='ORIGIN' THEN $6 ELSE origin_snapshot_reference END,destination_snapshot_reference=CASE WHEN $5='DESTINATION' THEN $6 ELSE destination_snapshot_reference END,reconciliation_status=CASE WHEN $4='COMPLETED' THEN 'MATCHED' WHEN $4='DISCREPANCY' THEN 'DISCREPANCY' ELSE reconciliation_status END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+transferCols, t.OrganizationID, id, expected, next, snapshotType, snapRef))
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

const returnCols = `id,organization_id,warehouse_id,source_order_id,source_shipment_id,COALESCE(package_id,''),reason_code,condition,disposition,financial_reference,status,COALESCE(inventory_item_id,''),version,created_at,updated_at,completed_at`

func scanReturn(r pgx.Row) (Return, error) {
	var v Return
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.SourceOrderID, &v.SourceShipmentID, &v.PackageID, &v.ReasonCode, &v.Condition, &v.Disposition, &v.FinancialReference, &v.Status, &v.InventoryItemID, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, e
}
func (s *Service) CreateReturn(ctx context.Context, t tenant.Context, v Return) (Return, error) {
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
	var e error
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanReturn(tx.QueryRow(ctx, `INSERT INTO fleet_partner_returns(id,organization_id,warehouse_id,source_order_id,source_shipment_id,package_id,reason_code,condition,financial_reference,inventory_item_id)VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,NULLIF($10,'')) RETURNING `+returnCols, v.ID, v.OrganizationID, v.WarehouseID, v.SourceOrderID, v.SourceShipmentID, v.PackageID, v.ReasonCode, v.Condition, v.FinancialReference, v.InventoryItemID))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "return.created.v1", "return", v.ID, v.Version, map[string]any{"returnId": v.ID, "warehouseId": v.WarehouseID, "financialReference": v.FinancialReference})
	})
	return v, e
}
func (s *Service) GetReturn(ctx context.Context, t tenant.Context, id string) (Return, error) {
	if e := s.gate(s.o.Flags.Returns, false); e != nil {
		return Return{}, e
	}
	v, e := scanReturn(s.o.Pool.QueryRow(ctx, `SELECT `+returnCols+` FROM fleet_partner_returns WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Return not found.", nil)
	}
	if e == nil {
		_, e = s.GetWarehouse(ctx, t, v.WarehouseID)
	}
	return v, e
}
func (s *Service) ListReturns(ctx context.Context, t tenant.Context) ([]Return, error) {
	if e := s.gate(s.o.Flags.Returns, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+returnCols+` FROM fleet_partner_returns r WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=r.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Return{}
	for rows.Next() {
		v, e := scanReturn(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) ReturnCommand(ctx context.Context, t tenant.Context, id, command, disposition, reason string, expected int64) (Return, error) {
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
	var v Return
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if command == "RECEIVE" && old.PackageID != "" {
			var shipment string
			if x := tx.QueryRow(ctx, `SELECT shipment_id FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2`, t.OrganizationID, old.PackageID).Scan(&shipment); x != nil {
				return x
			}
			_, x := tx.Exec(ctx, `INSERT INTO fleet_partner_custody_events(id,organization_id,package_id,shipment_id,from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,occurred_at,source,correlation_id)VALUES($1,$2,$3,$4,'RETURN_SOURCE',$5,'WAREHOUSE',$6,'',$6,$7,now(),'WAREHOUSE_RETURN',$8)`, ids.New("cst"), t.OrganizationID, old.PackageID, shipment, old.SourceOrderID, old.WarehouseID, old.Condition, id+":receive")
			if x != nil {
				return x
			}
		}
		if command == "RECEIVE" && old.InventoryItemID != "" {
			var qty float64
			var loc string
			if x := tx.QueryRow(ctx, `SELECT quantity_projection,location_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, old.InventoryItemID).Scan(&qty, &loc); x != nil {
				return x
			}
			after := qty + 1
			stockStatus := "RETURNED"
			if strings.EqualFold(old.Condition, "DAMAGED") || strings.EqualFold(old.Condition, "RESTRICTED") {
				stockStatus = "QUARANTINED"
			}
			if _, x := tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=$3,status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, old.InventoryItemID, after, stockStatus); x != nil {
				return x
			}
			_, x := tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,to_location_id,quantity,reason_code,actor_reference,return_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'RETURN',$4,$4,1,$5,$6,$7,$8,$9,$10)`, ids.New("imv"), t.OrganizationID, old.InventoryItemID, loc, old.ReasonCode, t.ActorID, id, id+":receive", qty, after)
			if x != nil {
				return x
			}
		}
		var x error
		v, x = scanReturn(tx.QueryRow(ctx, `UPDATE fleet_partner_returns SET status=$4,disposition=CASE WHEN $5<>'' THEN $5 ELSE disposition END,completed_at=CASE WHEN $4='COMPLETED' THEN now() ELSE completed_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+returnCols, t.OrganizationID, id, expected, next, strings.ToUpper(disposition)))
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
