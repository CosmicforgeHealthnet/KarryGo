package bookingrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
)

// AcceptedScheduledBookings lists accepted rides whose scheduled pickup is
// still ahead and within the monitoring horizon.
func (r *PostgresBookingRepository) AcceptedScheduledBookings(ctx context.Context, before time.Time, limit int) ([]bookingmodels.Booking, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text FROM taxi_bookings WHERE status='accepted' AND provider_id IS NOT NULL AND scheduled_at IS NOT NULL AND scheduled_at > now() AND scheduled_at <= $1 ORDER BY scheduled_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]bookingmodels.Booking, 0, len(ids))
	for _, id := range ids {
		booking, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, booking)
	}
	return out, nil
}

// ReopenForRecovery releases an invalid provider and returns the booking to
// matching in one transaction. The compare-and-set on status and provider
// makes concurrent monitors and accepts safe: only one reopen can win.
func (r *PostgresBookingRepository) ReopenForRecovery(ctx context.Context, booking bookingmodels.Booking, providerID, trigger string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var vehicleID *string
	err = tx.QueryRow(ctx, `SELECT vehicle_id::text FROM taxi_bookings WHERE id=$1 AND status='accepted' AND provider_id=$2 FOR UPDATE`, booking.ID, providerID).Scan(&vehicleID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already accepted by someone else, cancelled or re-matched.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE taxi_bookings SET status='pending_match', provider_id=NULL, vehicle_id=NULL, matching_started_at=now(),
		provider_replacement_count=provider_replacement_count+1, updated_at=now() WHERE id=$1`, booking.ID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE provider_operational_state SET state='OFFLINE', reason='scheduled_risk_recovery', booking_id=NULL, effective_at=now(), version=version+1 WHERE provider_id=$1 AND booking_id=$2`, providerID, booking.ID); err != nil {
		return false, err
	}
	audit, _ := json.Marshal([]map[string]any{{"at": time.Now().UTC(), "state": "IN_PROGRESS", "trigger": trigger}})
	if _, err = tx.Exec(ctx, `INSERT INTO taxi_recovery_cases(subject_type,booking_id,trigger_code,state,original_provider_id,original_vehicle_id,operation_ref,audit_json)
		VALUES('BOOKING',$1,$2,'IN_PROGRESS',$3,NULLIF($4,'')::uuid,$5,$6) ON CONFLICT DO NOTHING`, booking.ID, trigger, providerID, deref(vehicleID), "scheduled-risk:"+booking.ID+":"+providerID, audit); err != nil {
		return false, err
	}
	payload, _ := json.Marshal(map[string]any{"booking_id": booking.ID, "trigger_code": trigger, "original_provider_id": providerID})
	for _, event := range []string{"taxi.guarantee.breached", "taxi.recovery.started"} {
		if err = outbox.Write(ctx, tx, "booking", booking.ID, event, payload); err != nil {
			return false, err
		}
	}
	reopened := booking
	reopened.Status, reopened.ProviderID, reopened.VehicleID = bookingmodels.StatusPendingMatch, nil, nil
	if err = r.movement.WriteBooking(ctx, tx, reopened, booking.ID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ResolveRecoveredBookings closes open booking recovery cases once a new
// provider accepted or the booking reached a terminal state.
func (r *PostgresBookingRepository) ResolveRecoveredBookings(ctx context.Context) (int, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE taxi_recovery_cases c SET state=CASE WHEN b.status IN ('cancelled','unmatched') THEN 'FAILED' ELSE 'RESOLVED' END,
			replacement_provider_id=CASE WHEN b.provider_id IS DISTINCT FROM c.original_provider_id THEN b.provider_id ELSE c.replacement_provider_id END,
			resolved_at=now(), version=c.version+1
		FROM taxi_bookings b
		WHERE c.booking_id=b.id AND c.state='IN_PROGRESS'
		  AND ((b.provider_id IS NOT NULL AND b.provider_id IS DISTINCT FROM c.original_provider_id) OR b.status IN ('completed','cancelled','unmatched'))
		RETURNING c.booking_id::text, c.state`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var bookingID, state string
		if err = rows.Scan(&bookingID, &state); err != nil {
			return count, err
		}
		event := "taxi.recovery.resolved"
		if state == "FAILED" {
			event = "taxi.recovery.failed"
		} else {
			payload, _ := json.Marshal(map[string]any{"booking_id": bookingID})
			_ = outbox.Write(ctx, r.db, "booking", bookingID, "taxi.recovery.provider_replaced", payload)
		}
		payload, _ := json.Marshal(map[string]any{"booking_id": bookingID, "state": state})
		_ = outbox.Write(ctx, r.db, "booking", bookingID, event, payload)
		count++
	}
	return count, rows.Err()
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
