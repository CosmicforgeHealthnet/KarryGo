package commercemission

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type runtimeFact struct {
	MissionID         string
	MissionRef        string
	BookingID         string
	State             string
	QueueState        string
	Version           int64
	AssignmentAttempt int
	PromisedBy        time.Time
	BroadcastStatus   string
	ProviderID        string
	TripID            string
	TripStatus        string
	NextRetryAt       *time.Time
}

func (s *MultiStopMissionService) RunRecoveryWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	s.runRecoveryPass(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRecoveryPass(ctx)
		}
	}
}

func (s *MultiStopMissionService) runRecoveryPass(ctx context.Context) {
	repo, ok := s.store.(*Repository)
	if !ok {
		return
	}
	facts, err := repo.ListRuntimeFacts(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("commerce mission recovery: list failed: %v", err)
		}
		return
	}
	now := s.now().UTC()
	for _, fact := range facts {
		if ctx.Err() != nil {
			return
		}
		if fact.TripStatus == "completed" {
			if err = repo.ReconcileCompleted(ctx, fact, now); err != nil && ctx.Err() == nil {
				log.Printf("commerce mission recovery: completion reconcile mission_ref=%s: %v", fact.MissionRef, err)
			}
			continue
		}
		if fact.BroadcastStatus == "accepted" && fact.ProviderID != "" {
			if err = repo.ReconcileAssigned(ctx, fact, now); err != nil && ctx.Err() == nil {
				log.Printf("commerce mission recovery: assignment reconcile mission_ref=%s: %v", fact.MissionRef, err)
			}
			continue
		}
		if fact.BroadcastStatus == "no_provider_found" {
			if !fact.PromisedBy.IsZero() && !fact.PromisedBy.After(now) {
				if err = repo.MarkQueueExhausted(ctx, fact, now); err != nil && ctx.Err() == nil {
					log.Printf("commerce mission recovery: expiry mission_ref=%s: %v", fact.MissionRef, err)
				}
				continue
			}
			if fact.NextRetryAt == nil {
				_ = repo.ScheduleQueueRetry(ctx, fact, now.Add(queueBackoff(fact.AssignmentAttempt)), now)
				continue
			}
			if !fact.NextRetryAt.After(now) {
				op := fmt.Sprintf("queue-retry:%s:%d", fact.MissionRef, fact.AssignmentAttempt+1)
				_, err = s.ReplaceProvider(ctx, dispatchcommerce.SignatureCaller, fact.MissionRef, dispatchcommerce.CommandRequest{OperationRef: op, ExpectedVersion: fact.Version, ReasonCode: "NO_PROVIDER_RETRY"})
				if err != nil && ctx.Err() == nil {
					log.Printf("commerce mission recovery: retry mission_ref=%s: %v", fact.MissionRef, err)
				}
			}
		}
	}
}

func queueBackoff(attempt int) time.Duration {
	delay := 30 * time.Second
	for i := 1; i < attempt && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	return delay
}

func (r *Repository) ListRuntimeFacts(ctx context.Context) ([]runtimeFact, error) {
	rows, err := r.db.Query(ctx, `SELECT m.id::text,m.canonical_ref,m.booking_id::text,m.runtime_state,m.queue_state,m.source_version,m.assignment_attempt,COALESCE(m.promised_by,'epoch'::timestamptz),COALESCE(b.status,''),COALESCE(b.accepted_by_provider_id::text,''),COALESCE(t.id::text,''),COALESCE(t.status,''),m.next_retry_at FROM commerce_missions m LEFT JOIN request_broadcasts b ON b.booking_id=m.booking_id LEFT JOIN trips t ON t.booking_id=m.booking_id WHERE m.plan_version>0 AND m.runtime_state NOT IN ('COMPLETED','CANCELLED','EXHAUSTED') ORDER BY m.created_at LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var facts []runtimeFact
	for rows.Next() {
		var fact runtimeFact
		if err = rows.Scan(&fact.MissionID, &fact.MissionRef, &fact.BookingID, &fact.State, &fact.QueueState, &fact.Version, &fact.AssignmentAttempt, &fact.PromisedBy, &fact.BroadcastStatus, &fact.ProviderID, &fact.TripID, &fact.TripStatus, &fact.NextRetryAt); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

func (r *Repository) ReconcileAssigned(ctx context.Context, fact runtimeFact, now time.Time) error {
	if fact.State == "ASSIGNED" && fact.TripID != "" {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var currentVersion int64
	var currentState, replacementState string
	if err = tx.QueryRow(ctx, `SELECT source_version,runtime_state,replacement_state FROM commerce_missions WHERE id=$1::uuid FOR UPDATE`, fact.MissionID).Scan(&currentVersion, &currentState, &replacementState); err != nil {
		return err
	}
	if currentState == "CANCELLED" || currentState == "COMPLETED" {
		return nil
	}
	version := currentVersion + 1
	if replacementState == "MATCHING" || replacementState == "REQUIRED" {
		if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET replacement_provider_id=$2::uuid,replacement_trip_id=NULLIF($3,'')::uuid,replacement_state='ASSIGNED',runtime_state='TRANSFER_PENDING',queue_state='ASSIGNED',source_version=$4,updated_at=$5 WHERE id=$1::uuid`, fact.MissionID, fact.ProviderID, fact.TripID, version, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_mission_assignment_attempts SET trip_id=NULLIF($2,'')::uuid,provider_id=$3::uuid,state='REPLACEMENT_ASSIGNED' WHERE commerce_mission_id=$1::uuid AND booking_id=$4::uuid`, fact.MissionID, fact.TripID, fact.ProviderID, fact.BookingID); err != nil {
			return err
		}
		return r.emitRuntimeEvent(ctx, tx, fact.MissionID, fact.MissionRef, "TRANSFER_PENDING", "ASSIGNED", "PROVIDER_TRANSFER_REQUIRED", version, now, func() error { return tx.Commit(ctx) })
	}
	if replacementState != "NONE" {
		return nil
	}
	state := "ASSIGNED"
	if fact.TripStatus == "in_progress" || fact.TripStatus == "proof_submitted" {
		state = "IN_TRANSIT"
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET trip_id=NULLIF($2,'')::uuid,current_provider_id=$3::uuid,runtime_state=$4,queue_state='ASSIGNED',source_version=$5,reason_code='',updated_at=$6 WHERE id=$1::uuid`, fact.MissionID, fact.TripID, fact.ProviderID, state, version, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_assignment_attempts SET trip_id=NULLIF($2,'')::uuid,provider_id=$3::uuid,state='ASSIGNED' WHERE commerce_mission_id=$1::uuid AND booking_id=$4::uuid`, fact.MissionID, fact.TripID, fact.ProviderID, fact.BookingID); err != nil {
		return err
	}
	return r.emitRuntimeEvent(ctx, tx, fact.MissionID, fact.MissionRef, state, "ASSIGNED", "", version, now, func() error { return tx.Commit(ctx) })
}

func (r *Repository) ReconcileCompleted(ctx context.Context, fact runtimeFact, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var version int64
	var state, replacementState, activeTrip string
	if err = tx.QueryRow(ctx, `SELECT source_version,runtime_state,replacement_state,COALESCE(trip_id::text,'') FROM commerce_missions WHERE id=$1::uuid FOR UPDATE`, fact.MissionID).Scan(&version, &state, &replacementState, &activeTrip); err != nil {
		return err
	}
	if replacementState != "NONE" || (activeTrip != "" && fact.TripID != activeTrip) {
		return nil
	}
	if state == "COMPLETED" {
		return nil
	}
	var incomplete int
	if err = tx.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM commerce_mission_stops WHERE commerce_mission_id=$1::uuid AND status NOT IN ('PICKED_UP','SKIPPED','RETURNED')) +
		(SELECT count(*) FROM commerce_mission_parcels WHERE commerce_mission_id=$1::uuid AND status NOT IN ('PICKED_UP','IN_TRANSIT','DELIVERED','RETURNED'))`, fact.MissionID).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete != 0 {
		return apperrors.New(409, "MISSION_COMPLETION_EVIDENCE_INCOMPLETE", "Mission cannot complete while a stop or parcel is outside a terminal delivery path.", nil)
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state='COMPLETED',queue_state='COMPLETED',source_version=$2,completed_at=$3,updated_at=$3 WHERE id=$1::uuid`, fact.MissionID, version, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_parcels SET status='DELIVERED',custodian_type='RECEIVER',delivered_at=$2,updated_at=$2 WHERE commerce_mission_id=$1::uuid AND status IN ('PICKED_UP','IN_TRANSIT')`, fact.MissionID, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_mission_carrier_history SET custody_ended_at=$2,end_reason='DELIVERED',source_version=$3
		WHERE commerce_mission_id=$1::uuid AND custody_ended_at IS NULL`, fact.MissionID, now, version); err != nil {
		return err
	}
	if err = r.emitParcelEvent(ctx, tx, fact.MissionID, fact.MissionRef, "", "DELIVERED", version, now); err != nil {
		return err
	}
	return r.emitRuntimeEvent(ctx, tx, fact.MissionID, fact.MissionRef, "COMPLETED", "COMPLETED", "", version, now, func() error { return tx.Commit(ctx) })
}

func (r *Repository) ScheduleQueueRetry(ctx context.Context, fact runtimeFact, next, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var version int64
	err = tx.QueryRow(ctx, `UPDATE commerce_missions SET runtime_state='QUEUED',queue_state='QUEUED',reason_code='NO_PROVIDER_QUEUED',next_retry_at=$2,source_version=source_version+1,updated_at=$3 WHERE id=$1::uuid AND booking_id=$4::uuid AND runtime_state NOT IN ('COMPLETED','CANCELLED','EXHAUSTED') RETURNING source_version`, fact.MissionID, next, now, fact.BookingID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.emitRuntimeEvent(ctx, tx, fact.MissionID, fact.MissionRef,
		"QUEUED", "QUEUED", "NO_PROVIDER_QUEUED", version, now,
		func() error { return tx.Commit(ctx) })
}

func (r *Repository) MarkQueueExhausted(ctx context.Context, fact runtimeFact, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var version int64
	err = tx.QueryRow(ctx, `UPDATE commerce_missions SET runtime_state='EXHAUSTED',queue_state='EXHAUSTED',reason_code='SLA_EXHAUSTED',source_version=source_version+1,updated_at=$2 WHERE id=$1::uuid AND runtime_state NOT IN ('COMPLETED','CANCELLED','EXHAUSTED') RETURNING source_version`, fact.MissionID, now).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.emitRuntimeEvent(ctx, tx, fact.MissionID, fact.MissionRef,
		"EXHAUSTED", "EXHAUSTED", "SLA_EXHAUSTED", version, now,
		func() error { return tx.Commit(ctx) })
}

func (r *Repository) emitRuntimeEvent(ctx context.Context, tx pgx.Tx,
	missionID, missionRef, state, queue, reason string, version int64,
	now time.Time, commit func() error) error {
	if r.events != nil {
		var operationRef, missionSlotRef, providerRef string
		var planVersion int64
		if err := tx.QueryRow(ctx, `SELECT operation_ref,mission_slot_ref,plan_version,
			COALESCE(replacement_provider_id::text,current_provider_id::text,'')
			FROM commerce_missions WHERE id=$1::uuid`, missionID).Scan(&operationRef, &missionSlotRef, &planVersion, &providerRef); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT parcel_ref FROM commerce_mission_parcels WHERE commerce_mission_id=$1::uuid ORDER BY parcel_ref`, missionID)
		if err != nil {
			return err
		}
		var refs []string
		for rows.Next() {
			var ref string
			if err = rows.Scan(&ref); err != nil {
				rows.Close()
				return err
			}
			refs = append(refs, ref)
		}
		rows.Close()
		if err = r.events.EmitTarget(ctx, tx, vendorServiceTarget,
			"commerce.mission.changed.v1", "COMMERCE_MISSION", missionRef,
			version, "", "", map[string]any{
				"missionRef": missionRef, "operationRef": operationRef,
				"missionSlotRef": missionSlotRef, "state": state, "queueState": queue,
				"reasonCode": reason, "parcelRefs": refs,
				"providerRef": providerRef, "planVersion": planVersion,
				"sourceVersion": version, "occurredAt": now,
			}); err != nil {
			return err
		}
	}
	return commit()
}

var _ = strings.TrimSpace
