package orders

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
)

func ScheduledCutoffAt(desired time.Time, windowStart, productionReadyBy *time.Time) time.Time {
	if productionReadyBy != nil && !productionReadyBy.IsZero() {
		return productionReadyBy.UTC()
	}
	if windowStart != nil && !windowStart.IsZero() {
		return windowStart.UTC()
	}
	return desired.UTC().Add(-time.Hour)
}

// EmitScheduledCutoffNotifications durably marks and publishes each due
// cutoff once. All values are timestamptz instants; no server-local timezone
// participates in the calculation.
func (s *Section21Service) EmitScheduledCutoffNotifications(ctx context.Context, limit int) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id,business_id,customer_id,vendor_order_id,desired_delivery_at,
		delivery_window_start,production_ready_by,source_version
		FROM commerce_scheduled_orders o
		WHERE scheduling_status='ACCEPTED'
		AND NOT EXISTS (SELECT 1 FROM commerce_scheduled_notification_markers m
			WHERE m.scheduled_order_id=o.id AND m.notification_type='PREPARATION_CUTOFF')
		AND COALESCE(production_ready_by,delivery_window_start,desired_delivery_at-interval '1 hour')
			<= now()+interval '1 hour'
		ORDER BY COALESCE(production_ready_by,delivery_window_start,desired_delivery_at-interval '1 hour'),id
		LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	type due struct {
		id, business, customer, vendorOrder string
		desired                             time.Time
		windowStart, productionReady        *time.Time
		version                             int64
	}
	var dueOrders []due
	for rows.Next() {
		var item due
		if err = rows.Scan(&item.id, &item.business, &item.customer, &item.vendorOrder,
			&item.desired, &item.windowStart, &item.productionReady, &item.version); err != nil {
			rows.Close()
			return 0, err
		}
		dueOrders = append(dueOrders, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	var created int64
	for _, item := range dueOrders {
		cutoff := ScheduledCutoffAt(item.desired, item.windowStart, item.productionReady)
		envelope, envelopeErr := vendorevents.NewEnvelope("commerce.scheduled_order.cutoff", 1,
			"commerce_scheduled_order", item.id, item.version, item.business, item.id, "", map[string]any{
				"business_ref": item.business, "customer_ref": item.customer,
				"vendor_order_ref": item.vendorOrder, "scheduled_order_ref": item.id,
				"cutoff_at": cutoff, "source_version": item.version,
			}, false)
		if envelopeErr != nil {
			return 0, envelopeErr
		}
		tag, insertErr := tx.Exec(ctx, `INSERT INTO commerce_scheduled_notification_markers(
			scheduled_order_id,notification_type,cutoff_at,source_version,event_id)
			VALUES($1,'PREPARATION_CUTOFF',$2,$3,$4) ON CONFLICT DO NOTHING`,
			item.id, cutoff, item.version, envelope.EventID)
		if insertErr != nil {
			return 0, insertErr
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if s.events != nil {
			if err = s.events.Append(ctx, tx, envelope); err != nil {
				return 0, err
			}
		}
		created++
	}
	return created, tx.Commit(ctx)
}
