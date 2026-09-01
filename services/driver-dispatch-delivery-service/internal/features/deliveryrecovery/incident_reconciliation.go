package deliveryrecovery

import (
	"context"
	"github.com/google/uuid"
	"log"
	"time"
)

type incidentReconciler interface {
	ReconcileIncident(context.Context, string) (bool, error)
}
type PendingIncident struct {
	ID       string
	Attempts int
}

func (s *Service) reconcileIncidents(ctx context.Context) error {
	authority, ok := s.replacement.(incidentReconciler)
	if !ok || !s.policy.ReplacementEnabled {
		return nil
	}
	owner := s.workerOwner + ":" + uuid.NewString()
	items, err := s.repo.ClaimIncidentRecovery(ctx, s.now(), owner, s.workerLease, s.workerMaxAttempts)
	if err != nil {
		return err
	}
	var pending, attempts, exhausted int64
	if err = s.repo.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE recovery_result IN ('PENDING','UNKNOWN')),COALESCE(sum(recovery_attempts),0),count(*) FILTER(WHERE recovery_result='MANUAL_REVIEW') FROM dispatch_delivery_incidents`).Scan(&pending, &attempts, &exhausted); err == nil {
		log.Printf("dispatch replacement metrics replacement_recovery_backlog=%d replacement_recovery_attempts=%d replacement_recovery_exhausted=%d", pending, attempts, exhausted)
	}
	for _, item := range items {
		ready, reconcileErr := authority.ReconcileIncident(ctx, item.ID)
		result := "PENDING"
		if ready {
			result = "READY"
		} else if reconcileErr != nil {
			result = "UNKNOWN"
		}
		if err = s.repo.CompleteIncidentRecovery(ctx, item.ID, owner, result, errorCode(reconcileErr), s.now().Add(s.backoff(item.Attempts))); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ClaimIncidentRecovery(ctx context.Context, now time.Time, owner string, lease time.Duration, maxAttempts int) ([]PendingIncident, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Read-back/terminalization happens before any matching. Exhaustion does not
	// cancel money or transfer custody: it leaves an explicit Operations task.
	_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_incidents i SET recovery_result=CASE WHEN t.status IN ('completed','cancelled','failed') OR NOT EXISTS(SELECT 1 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.trip_id=t.id AND m.state='REASSIGNING' AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED')) THEN 'CANCELLED' ELSE 'MANUAL_REVIEW' END,recovery_error_code='REPLACEMENT_RECOVERY_STOPPED' FROM trips t WHERE t.id=i.trip_id AND i.recovery_result IN ('PENDING','UNKNOWN') AND (i.recovery_lease_expires_at IS NULL OR i.recovery_lease_expires_at<=$1) AND (i.recovery_deadline<=$1 OR i.recovery_attempts>=$2 OR t.status IN ('completed','cancelled','failed') OR NOT EXISTS(SELECT 1 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.trip_id=t.id AND m.state='REASSIGNING' AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED')))`, now, maxAttempts)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_assignment_offers o SET status='CANCELLED',version=version+1 FROM dispatch_delivery_incidents i WHERE i.id=o.incident_id AND i.recovery_result IN ('CANCELLED','MANUAL_REVIEW') AND o.status='OFFERED'`)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `WITH due AS (SELECT i.id FROM dispatch_delivery_incidents i JOIN trips t ON t.id=i.trip_id WHERE i.state='REASSIGNING' AND i.recovery_result IN ('PENDING','UNKNOWN') AND i.recovery_next_at<=$1 AND i.recovery_deadline>$1 AND i.recovery_attempts<$4 AND t.provider_id=i.provider_id AND (i.recovery_lease_expires_at IS NULL OR i.recovery_lease_expires_at<=$1) ORDER BY i.recovery_next_at FOR UPDATE OF i SKIP LOCKED LIMIT 100) UPDATE dispatch_delivery_incidents i SET recovery_attempts=recovery_attempts+1,recovery_lease_owner=$2,recovery_lease_expires_at=$3 FROM due WHERE i.id=due.id RETURNING i.id::text,i.recovery_attempts`, now, owner, now.Add(lease), maxAttempts)
	if err != nil {
		return nil, err
	}
	var items []PendingIncident
	for rows.Next() {
		var i PendingIncident
		if err = rows.Scan(&i.ID, &i.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, i)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return items, tx.Commit(ctx)
}
func (r *Repository) CompleteIncidentRecovery(ctx context.Context, id, owner, result, code string, next time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_result=$3,recovery_error_code=$4,recovery_next_at=GREATEST($5,COALESCE((SELECT max(o.expires_at) FROM dispatch_assignment_offers o WHERE o.incident_id=$1 AND o.status='OFFERED'),$5)),recovery_lease_owner=NULL,recovery_lease_expires_at=NULL WHERE id=$1 AND recovery_lease_owner=$2 AND recovery_result IN ('PENDING','UNKNOWN')`, id, owner, result, code, next)
	return err
}
