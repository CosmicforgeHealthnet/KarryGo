package warehouse

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

const JobWarehouseSweep = "section21.operations_sweep"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobWarehouseSweep, s.handleSweep) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT organization_id FROM fleet_partner_warehouses WHERE status='ACTIVE' GROUP BY organization_id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		if e = rows.Scan(&org); e != nil {
			return e
		}
		_, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobWarehouseSweep, DedupeKey: "section21:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if e != nil {
			return e
		}
	}
	return rows.Err()
}
func (s *Service) handleSweep(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id,inventory_item_id,quantity FROM fleet_partner_inventory_reservations WHERE organization_id=$1 AND status='ACTIVE' AND expires_at<=now() FOR UPDATE SKIP LOCKED`, p.OrganizationID)
		if e != nil {
			return e
		}
		type res struct {
			id, item string
			qty      float64
		}
		var expired []res
		for rows.Next() {
			var v res
			if e = rows.Scan(&v.id, &v.item, &v.qty); e != nil {
				rows.Close()
				return e
			}
			expired = append(expired, v)
		}
		rows.Close()
		for _, v := range expired {
			if _, e = tx.Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status='EXPIRED',version=version+1,updated_at=now() WHERE id=$1`, v.id); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=GREATEST(0,reserved_quantity-$2),status=CASE WHEN reserved_quantity-$2<=0 AND status='RESERVED' THEN 'AVAILABLE' ELSE status END,version=version+1,updated_at=now() WHERE id=$1`, v.item, v.qty); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND expiry_at<=now() AND status NOT IN('EXPIRED','DISPATCHED')`, p.OrganizationID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE fleet_partner_dock_appointments SET status='NO_SHOW',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('REQUESTED','CONFIRMED') AND window_end<now()`, p.OrganizationID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE fleet_partner_yard_visits SET status='SECURITY_HOLD',security_status='HOLD',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('GATE_CHECK_IN','IN_YARD','AT_BAY') AND updated_at<now()-interval '12 hours'`, p.OrganizationID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE fleet_partner_terminal_transfers SET status='RECONCILING',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='ARRIVED' AND arrived_at<now()-interval '1 hour'`, p.OrganizationID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE fleet_partner_returns SET status='INSPECTING',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='RECEIVED' AND updated_at<now()-interval '24 hours'`, p.OrganizationID); e != nil {
			return e
		}
		for _, v := range expired {
			_, _ = tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'ADJUSTMENT',location_id,$3,'RESERVATION_EXPIRED','SYSTEM',$4,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$5 ON CONFLICT DO NOTHING`, ids.New("imv"), p.OrganizationID, v.qty, "reservation-expired:"+v.id, v.item)
		}
		return nil
	})
}
