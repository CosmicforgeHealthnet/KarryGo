package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var logisticsTimingEvents = map[string]bool{
	"TIMING_UPDATED":        true,
	"PICKUP_TIMING_UPDATED": true,
	"ETA_UPDATED":           true,
	"TIMING_APPLIED":        true,
	"TIMING_NO_CHANGE":      true,
	"TIMING_TOO_LATE":       true,
	"TIMING_TERMINAL":       true,
	"TIMING_UNKNOWN":        true,
}

type logisticsTimingPayload struct {
	TimingStatus         string          `json:"timing_status"`
	Outcome              string          `json:"outcome"`
	PickupNotBefore      *time.Time      `json:"pickup_not_before"`
	ScheduledPickupAt    *time.Time      `json:"scheduled_pickup_at"`
	ETAStart             *time.Time      `json:"eta_start"`
	ETAEnd               *time.Time      `json:"eta_end"`
	LogisticsWindowStart *time.Time      `json:"logistics_window_start"`
	LogisticsWindowEnd   *time.Time      `json:"logistics_window_end"`
	SourceEvidence       json.RawMessage `json:"source_evidence"`
}

func isLogisticsTimingEvent(eventType string) bool {
	return logisticsTimingEvents[strings.ToUpper(strings.TrimSpace(eventType))]
}

func parseLogisticsTiming(payload json.RawMessage, eventType string) (logisticsTimingPayload, error) {
	var timing logisticsTimingPayload
	if len(payload) > 32<<10 || json.Unmarshal(payload, &timing) != nil {
		return timing, ErrOrderConflict
	}
	if timing.LogisticsWindowStart == nil {
		timing.LogisticsWindowStart = timing.ETAStart
	}
	if timing.LogisticsWindowEnd == nil {
		timing.LogisticsWindowEnd = timing.ETAEnd
	}
	status := strings.ToUpper(strings.TrimSpace(timing.TimingStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(timing.Outcome))
	}
	if status == "" {
		status = "AVAILABLE"
	}
	switch status {
	case "AVAILABLE", "APPLIED", "NO_CHANGE", "TOO_LATE_TO_RESCHEDULE", "TERMINAL", "UNKNOWN", "NOT_AVAILABLE":
	default:
		return timing, ErrOrderConflict
	}
	if status == "AVAILABLE" && (timing.LogisticsWindowStart == nil || timing.LogisticsWindowEnd == nil || !timing.LogisticsWindowEnd.After(*timing.LogisticsWindowStart)) {
		return timing, ErrOrderConflict
	}
	if timing.SourceEvidence == nil {
		timing.SourceEvidence = json.RawMessage(`{}`)
	}
	_ = eventType
	timing.TimingStatus = status
	return timing, nil
}

// applyLogisticsTimingTx applies an authority-owned timing answer. It never
// changes money, mission fare, assignment or VendorOrder lifecycle state.
func (s *Store) applyLogisticsTimingTx(ctx context.Context, tx pgx.Tx, event LogisticsOrderEvent, now time.Time) error {
	timing, err := parseLogisticsTiming(event.Payload, event.EventType)
	if err != nil {
		return err
	}
	var oldStatus string
	var oldStart, oldEnd *time.Time
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(logistics_timing_status,'NOT_AVAILABLE'),logistics_eta_start,logistics_eta_end,logistics_timing_version FROM commerce_order_missions WHERE mission_ref=$1 AND parent_order_id=$2 FOR UPDATE`, event.MissionRef, event.ParentOrderID).Scan(&oldStatus, &oldStart, &oldEnd, &currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}
	if event.SourceVersion < currentVersion || (event.SourceVersion == currentVersion && oldStatus != "NOT_AVAILABLE" && oldStatus != "UNKNOWN") {
		// The event is durable evidence, but an older authority revision can
		// never rewind current mission timing or the customer projection.
		return nil
	}
	missionStatus := timing.TimingStatus
	etaStart, etaEnd := timing.LogisticsWindowStart, timing.LogisticsWindowEnd
	if missionStatus != "AVAILABLE" && missionStatus != "APPLIED" && missionStatus != "NO_CHANGE" {
		etaStart, etaEnd = nil, nil
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_order_missions SET pickup_not_before=$1,scheduled_pickup_at=$2,logistics_eta_start=$3,logistics_eta_end=$4,logistics_timing_status=$5,logistics_timing_source=$6,logistics_timing_version=GREATEST(logistics_timing_version,$7),updated_at=$8 WHERE mission_ref=$9 AND parent_order_id=$10`, timing.PickupNotBefore, timing.ScheduledPickupAt, etaStart, etaEnd, missionStatus, string(event.Source), event.SourceVersion, now, event.MissionRef, event.ParentOrderID)
	if err != nil {
		return err
	}
	newStatus := "NOT_AVAILABLE"
	if etaStart != nil && etaEnd != nil && (missionStatus == "AVAILABLE" || missionStatus == "APPLIED" || missionStatus == "NO_CHANGE") {
		newStatus = "AVAILABLE"
	}
	var evidence json.RawMessage
	evidence, _ = json.Marshal(map[string]any{
		"mission_ref":        event.MissionRef,
		"source":             event.Source,
		"source_version":     event.SourceVersion,
		"timing_status":      missionStatus,
		"authority_evidence": json.RawMessage(timing.SourceEvidence),
	})
	_, err = tx.Exec(ctx, `UPDATE commerce_vendor_order_readiness r SET logistics_window_start=$1,logistics_window_end=$2,customer_eta_status=$3,customer_eta_start=$4,customer_eta_end=$5,timing_revision=timing_revision+1,timing_source_evidence=$6,source_version=source_version+1,updated_at=$7 FROM commerce_order_mission_allocations a JOIN commerce_vendor_orders v ON v.id=a.vendor_order_id WHERE a.mission_id=(SELECT id FROM commerce_order_missions WHERE mission_ref=$8 AND parent_order_id=$9) AND r.vendor_order_id=v.id AND v.parent_order_id=$9`, etaStart, etaEnd, newStatus, etaStart, etaEnd, evidence, now, event.MissionRef, event.ParentOrderID)
	if err != nil {
		return err
	}
	if materialETAChange(oldStatus, newStatus, oldStart, oldEnd, etaStart, etaEnd) {
		rows, queryErr := tx.Query(ctx, `SELECT a.vendor_order_id FROM commerce_order_mission_allocations a JOIN commerce_order_missions m ON m.id=a.mission_id WHERE m.mission_ref=$1 AND m.parent_order_id=$2 ORDER BY a.vendor_order_id`, event.MissionRef, event.ParentOrderID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var vendorOrderID string
			if err = rows.Scan(&vendorOrderID); err != nil {
				return err
			}
			if s.outbox != nil {
				var businessID, customerRef string
				if scanErr := tx.QueryRow(ctx, `SELECT business_id FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID).Scan(&businessID); scanErr != nil {
					return scanErr
				}
				if scanErr := tx.QueryRow(ctx, `SELECT customer_id FROM commerce_parent_orders WHERE id=$1`, event.ParentOrderID).Scan(&customerRef); scanErr != nil {
					return scanErr
				}
				if err = s.outbox.Publish(ctx, tx, "commerce.customer_eta.changed", "VendorOrder", vendorOrderID, 1, businessID, event.EventID, event.EventID, map[string]any{
					"customer_ref":        customerRef,
					"vendor_order_ref":    vendorOrderID,
					"parent_order_ref":    event.ParentOrderID,
					"customer_eta_status": newStatus,
					"customer_eta_start":  etaStart,
					"customer_eta_end":    etaEnd,
					"timing_source":       string(event.Source),
					"source_version":      event.SourceVersion,
				}); err != nil {
					return err
				}
			}
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

const materialETAThreshold = 5 * time.Minute

// materialETAChange bounds notification noise from tiny operational jitter.
// Availability transitions always notify; two available windows must move by
// at least the configured bounded threshold on either edge to be material.
func materialETAChange(oldStatus, newStatus string, oldStart, oldEnd, newStart, newEnd *time.Time) bool {
	if oldStatus != newStatus {
		return true
	}
	if oldStart == nil || oldEnd == nil || newStart == nil || newEnd == nil {
		return oldStart != nil || oldEnd != nil || newStart != nil || newEnd != nil
	}
	return absDuration(oldStart.Sub(*newStart)) >= materialETAThreshold || absDuration(oldEnd.Sub(*newEnd)) >= materialETAThreshold
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
