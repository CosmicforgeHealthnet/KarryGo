package production

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type operationalAlertCandidate struct {
	businessID string
	branchID   string
	alertType  string
	sourceType string
	sourceRef  string
	dedupeKey  string
	payload    map[string]any
}

// GenerateOperationalAlerts emits durable, deduplicated operational evidence.
// It intentionally publishes through the transactional outbox only; delivery
// to people remains Notification-domain work.
func (s *Service) GenerateOperationalAlerts(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := s.Now()
	candidates := []operationalAlertCandidate{}
	appendJobs := func(query, alertType, dedupePrefix string, args ...any) error {
		rows, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var business, branch, id string
			var scheduledStart, scheduledEnd *time.Time
			if err = rows.Scan(&business, &branch, &id, &scheduledStart, &scheduledEnd); err != nil {
				return err
			}
			candidates = append(candidates, operationalAlertCandidate{businessID: business, branchID: branch, alertType: alertType, sourceType: "PRODUCTION_JOB", sourceRef: id, dedupeKey: dedupePrefix + ":" + id, payload: map[string]any{"production_job_id": id, "scheduled_start": scheduledStart, "scheduled_end": scheduledEnd}})
			if len(candidates) >= limit {
				break
			}
		}
		return rows.Err()
	}
	if err := appendJobs(`SELECT business_id,branch_id,id,scheduled_start,scheduled_end FROM production_jobs WHERE status IN ('PLANNED','QUEUED') AND scheduled_start>$1 AND scheduled_start<=$2 ORDER BY scheduled_start,id LIMIT $3`, "PRODUCTION_STARTING_SOON", "starting-soon", now, now.Add(30*time.Minute), limit); err != nil {
		return 0, err
	}
	if len(candidates) < limit {
		if err := appendJobs(`SELECT business_id,branch_id,id,scheduled_start,scheduled_end FROM production_jobs WHERE status IN ('PLANNED','QUEUED','IN_PROGRESS','BLOCKED') AND scheduled_end IS NOT NULL AND scheduled_end<$1 ORDER BY scheduled_end,id LIMIT $2`, "LATE_PRODUCTION_JOB", "late-job", now, limit-len(candidates)); err != nil {
			return 0, err
		}
	}
	if len(candidates) < limit {
		rows, err := s.db.Query(ctx, `SELECT p.business_id,p.branch_id,o.id,o.conflict_reason,o.attempt_count FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.state='CAPACITY_CONFLICT' ORDER BY o.starts_at,o.id LIMIT $1`, limit-len(candidates))
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var business, branch, id string
			var reason *string
			var attempt int
			if err = rows.Scan(&business, &branch, &id, &reason, &attempt); err != nil {
				rows.Close()
				return 0, err
			}
			candidates = append(candidates, operationalAlertCandidate{businessID: business, branchID: branch, alertType: "CAPACITY_CONFLICT", sourceType: "CORPORATE_OCCURRENCE", sourceRef: id, dedupeKey: fmt.Sprintf("capacity-conflict:%s:%d", id, attempt), payload: map[string]any{"occurrence_id": id, "reason": reason, "attempt_count": attempt}})
		}
		rows.Close()
	}
	if len(candidates) < limit {
		rows, err := s.db.Query(ctx, `SELECT r.business_id,r.branch_id,c.id,c.created_at FROM pastry_change_requests c JOIN pastry_custom_requests r ON r.id=c.request_id WHERE c.status='OPEN' AND c.created_at<$1 ORDER BY c.created_at,c.id LIMIT $2`, now.Add(-15*time.Minute), limit-len(candidates))
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var business, branch, id string
			var createdAt time.Time
			if err = rows.Scan(&business, &branch, &id, &createdAt); err != nil {
				rows.Close()
				return 0, err
			}
			candidates = append(candidates, operationalAlertCandidate{businessID: business, branchID: branch, alertType: "CUSTOM_CAKE_CHANGE_AWAITING_REVIEW", sourceType: "PASTRY_CHANGE_REQUEST", sourceRef: id, dedupeKey: "cake-change-review:" + id, payload: map[string]any{"change_request_id": id, "requested_at": createdAt}})
		}
		rows.Close()
	}

	created := 0
	for _, candidate := range candidates {
		insertedAlert := false
		err := s.inTx(ctx, func(tx pgx.Tx) error {
			payload, err := json.Marshal(candidate.payload)
			if err != nil {
				return err
			}
			id := "poa_" + uuid.NewString()
			var inserted string
			err = tx.QueryRow(ctx, `INSERT INTO production_operational_alerts(id,business_id,branch_id,alert_type,source_type,source_ref,dedupe_key,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(dedupe_key) DO NOTHING RETURNING id`, id, candidate.businessID, candidate.branchID, candidate.alertType, candidate.sourceType, candidate.sourceRef, candidate.dedupeKey, payload).Scan(&inserted)
			if err == pgx.ErrNoRows {
				return nil
			}
			if err != nil {
				return err
			}
			if err = s.RecordMutation(ctx, tx, Actor{VendorID: candidate.businessID}, "production.alert.created", "production.alert.created", "production_operational_alert", inserted, candidate.branchID, 1, nil, map[string]any{"alert_type": candidate.alertType, "source_type": candidate.sourceType, "source_ref": candidate.sourceRef}, candidate.payload, ""); err != nil {
				return err
			}
			insertedAlert = true
			return nil
		})
		if err != nil {
			return created, err
		}
		if insertedAlert {
			created++
		}
	}
	return created, nil
}
