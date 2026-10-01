package assignment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func unique(err error) bool { var e *pgconn.PgError; return errors.As(err, &e) && e.Code == "23505" }

func (r *Repository) Start(ctx context.Context, in StartRequest, strategy Strategy, candidates []Candidate, window time.Duration, policyVersion string, queueEnabled bool) (Assignment, error) {
	fp := fingerprint(in)
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var currentState string
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT state,version FROM dispatch_delivery_missions WHERE id=$1 FOR UPDATE`, in.MissionID).Scan(&currentState, &currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Assignment{}, ErrNotFound
		}
		return Assignment{}, err
	}
	if currentState != "PLANNED" && currentState != "QUEUED" && currentState != "OFFERING" {
		return Assignment{}, ErrConflict
	}
	if in.ExpectedMissionVersion > 0 && in.ExpectedMissionVersion != currentVersion {
		return Assignment{}, ErrConflict
	}
	var storedStrategy Strategy
	var storedKey, storedFP string
	err = tx.QueryRow(ctx, `INSERT INTO dispatch_assignment_policies(mission_id,strategy,organization_id,policy_version,idempotency_key,request_fingerprint)
		VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6) ON CONFLICT(mission_id) DO UPDATE SET mission_id=EXCLUDED.mission_id
		RETURNING strategy,idempotency_key,request_fingerprint`, in.MissionID, strategy, in.OrganizationID, policyVersion, in.IdempotencyKey, fp).Scan(&storedStrategy, &storedKey, &storedFP)
	if err != nil {
		return Assignment{}, err
	}
	if storedStrategy != strategy || storedKey != in.IdempotencyKey || storedFP != fp {
		return Assignment{}, ErrConflict
	}
	var existing Offer
	getErr := tx.QueryRow(ctx, `SELECT o.id::text,o.mission_id::text,o.provider_id::text,o.status,c.rank,o.offered_at,o.expires_at,o.version
		FROM dispatch_assignment_offers o JOIN dispatch_assignment_candidates c ON c.id=o.candidate_id
		WHERE o.mission_id=$1 AND o.status IN ('OFFERED','ACCEPTED') ORDER BY o.created_at DESC LIMIT 1`, in.MissionID).
		Scan(&existing.ID, &existing.MissionID, &existing.ProviderID, &existing.Status, &existing.Rank, &existing.OfferedAt, &existing.ExpiresAt, &existing.Version)
	if getErr == nil {
		status := "OFFERING"
		if existing.Status == "ACCEPTED" {
			status = "ASSIGNED"
		}
		return Assignment{MissionID: in.MissionID, Strategy: strategy, Status: status, Offer: &existing, Replayed: true}, tx.Commit(ctx)
	}
	if !errors.Is(getErr, pgx.ErrNoRows) {
		return Assignment{}, getErr
	}
	result := Assignment{MissionID: in.MissionID, Strategy: strategy}
	if strategy == BroadcastFirstAccept {
		result.Status = "DELEGATED"
		return result, tx.Commit(ctx)
	}
	// An explicitly selected sequential policy takes ownership of matching for
	// this mission. Close any still-live legacy broadcast in the same
	// transaction so broadcast acceptance and exclusive acceptance cannot
	// produce conflicting winners. Deployments with the flag off never execute
	// this statement and preserve the original broadcast lifecycle verbatim.
	if _, err = tx.Exec(ctx, `UPDATE request_broadcasts b SET status='cancelled',updated_at=now()
		FROM dispatch_delivery_missions m WHERE m.id=$1 AND b.booking_id=m.booking_id AND b.status='broadcasting'`, in.MissionID); err != nil {
		return Assignment{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE provider_request_inbox i SET status='expired',responded_at=COALESCE(responded_at,now())
		FROM request_broadcasts b,dispatch_delivery_missions m WHERE m.id=$1 AND b.booking_id=m.booking_id AND i.broadcast_id=b.id AND i.status='pending'`, in.MissionID); err != nil {
		return Assignment{}, err
	}
	var legacyWon bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM request_broadcasts b JOIN dispatch_delivery_missions m ON m.booking_id=b.booking_id WHERE m.id=$1 AND b.status='accepted')`, in.MissionID).Scan(&legacyWon); err != nil {
		return Assignment{}, err
	}
	if legacyWon {
		return Assignment{}, ErrConflict
	}
	queueEnabled = queueEnabled && strategy == SequentialNearest
	if queueEnabled {
		if in.QueueExpiresAt.IsZero() {
			in.QueueExpiresAt = time.Now().UTC().Add(30 * time.Minute)
		}
		if !in.QueueExpiresAt.After(time.Now().UTC()) {
			return Assignment{}, ErrExpired
		}
		nextAttempt := time.Now().UTC().Add(window)
		if in.ScheduledFor != nil && in.ScheduledFor.After(nextAttempt) {
			nextAttempt = *in.ScheduledFor
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_queue_entries(mission_id,status,priority,next_attempt_at,expires_at,customer_decision_deadline,scheduled_for,quote_ref,quote_expires_at,idempotency_key,request_fingerprint,last_error_code,strategy,initial_radius_km,current_radius_km,max_radius_km,service_tier,priority_reason,priority_policy_version,search_policy_version,serviceability_ref)
			VALUES($1,'SCHEDULED',$2,$3,$4,$5,$6,$7,$8,$9,$10,'CANDIDATES_ACTIVE',$11,$12,$12,$13,$14,$15,$16,'dispatch_queue_search_v1',$17) ON CONFLICT(idempotency_key) DO NOTHING`, in.MissionID, in.Priority, nextAttempt, in.QueueExpiresAt, in.CustomerDecisionDeadline, in.ScheduledFor, in.QuoteRef, in.QuoteExpiresAt, in.IdempotencyKey, fp, strategy, in.InitialRadiusKM, in.MaxRadiusKM, normalizeServiceTier(in.ServiceTier), in.PriorityReason, in.PriorityPolicyVersion, in.ServiceabilityRef)
		if err != nil {
			return Assignment{}, err
		}
	}
	for _, candidate := range candidates {
		snapshot, _ := json.Marshal(candidate.Snapshot)
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_assignment_candidates(mission_id,provider_id,rank,distance_meters,candidate_snapshot)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT(mission_id,provider_id) DO NOTHING`, in.MissionID, candidate.ProviderID, candidate.Rank, candidate.DistanceM, snapshot)
		if err != nil {
			return Assignment{}, err
		}
	}
	var offer *Offer
	if in.ScheduledFor == nil || !in.ScheduledFor.After(time.Now().UTC()) {
		offer, err = createNextOffer(ctx, tx, in.MissionID, time.Now().UTC(), window)
	}
	if err != nil {
		return Assignment{}, err
	}
	if offer == nil {
		if !queueEnabled {
			_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='FAILED',version=version+1,updated_at=now() WHERE id=$1`, in.MissionID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE request_broadcasts b SET status='no_provider_found',updated_at=now() FROM dispatch_delivery_missions m WHERE m.id=$1 AND b.booking_id=m.booking_id AND b.status='cancelled'`, in.MissionID)
			}
			if err != nil {
				return Assignment{}, err
			}
			result.Status = "NO_PROVIDER_FOUND"
		} else {
			if in.QueueExpiresAt.IsZero() {
				in.QueueExpiresAt = time.Now().UTC().Add(30 * time.Minute)
			}
			if !in.QueueExpiresAt.After(time.Now().UTC()) {
				return Assignment{}, ErrExpired
			}
			_, err = tx.Exec(ctx, `INSERT INTO dispatch_queue_entries(mission_id,status,priority,expires_at,customer_decision_deadline,scheduled_for,quote_ref,quote_expires_at,idempotency_key,request_fingerprint,strategy,initial_radius_km,current_radius_km,max_radius_km,service_tier,priority_reason,priority_policy_version,search_policy_version,serviceability_ref)
			VALUES($1,'QUEUED',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11,$12,$13,$14,$15,'dispatch_queue_search_v1',$16) ON CONFLICT(idempotency_key) DO NOTHING`, in.MissionID, in.Priority, in.QueueExpiresAt, in.CustomerDecisionDeadline, in.ScheduledFor, in.QuoteRef, in.QuoteExpiresAt, in.IdempotencyKey, fp, strategy, in.InitialRadiusKM, in.MaxRadiusKM, normalizeServiceTier(in.ServiceTier), in.PriorityReason, in.PriorityPolicyVersion, in.ServiceabilityRef)
			if err != nil {
				return Assignment{}, err
			}
			_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='QUEUED',version=version+1,updated_at=now() WHERE id=$1`, in.MissionID)
			result.Status = "QUEUED"
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='OFFERING',version=version+1,updated_at=now() WHERE id=$1`, in.MissionID)
		result.Status, result.Offer = "OFFERING", offer
	}
	if err != nil {
		return Assignment{}, err
	}
	if err = emit(ctx, tx, "dispatch.assignment.started.v1", in.MissionID, 1, map[string]any{"strategy": strategy, "status": result.Status}); err != nil {
		return Assignment{}, err
	}
	if strategy == OrganizationDispatch || strategy == AdminAssisted {
		actor := in.ActorRef
		if actor == "" {
			actor = "organization:" + in.OrganizationID
		}
		reason := in.ReasonCode
		if reason == "" {
			reason = "ORGANIZATION_POLICY"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO dispatch_assignment_audit(mission_id,provider_id,strategy,actor_ref,reason_code,safe_note,result,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.MissionID, in.ProviderID, strategy, actor, reason, in.Note, result.Status, in.CorrelationID); err != nil {
			return Assignment{}, err
		}
	}
	if err = tx.Commit(ctx); unique(err) {
		return Assignment{}, ErrConflict
	}
	return result, err
}

func createNextOffer(ctx context.Context, tx pgx.Tx, missionID string, now time.Time, window time.Duration) (*Offer, error) {
	var candidateID, providerID string
	var rank int
	err := tx.QueryRow(ctx, `SELECT c.id::text,c.provider_id::text,c.rank FROM dispatch_assignment_candidates c
		WHERE c.mission_id=$1 AND NOT EXISTS(SELECT 1 FROM dispatch_assignment_offers o WHERE o.candidate_id=c.id)
		ORDER BY c.rank LIMIT 1 FOR UPDATE SKIP LOCKED`, missionID).Scan(&candidateID, &providerID, &rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o := &Offer{MissionID: missionID, ProviderID: providerID, Status: "OFFERED", Rank: rank, OfferedAt: now, ExpiresAt: now.Add(window), Version: 1}
	err = tx.QueryRow(ctx, `INSERT INTO dispatch_assignment_offers(mission_id,candidate_id,provider_id,offered_at,expires_at)
		VALUES($1,$2,$3,$4,$5) RETURNING id::text`, missionID, candidateID, providerID, o.OfferedAt, o.ExpiresAt).Scan(&o.ID)
	if err == nil {
		err = emit(ctx, tx, "dispatch.offer.created.v1", missionID, int64(rank), map[string]any{"offer_id": o.ID, "provider_id": providerID, "rank": rank, "expires_at": o.ExpiresAt})
	}
	return o, err
}

func (r *Repository) ListProviderOffers(ctx context.Context, providerID string) ([]Offer, error) {
	rows, err := r.db.Query(ctx, `SELECT o.id::text,o.mission_id::text,o.provider_id::text,o.status,c.rank,o.offered_at,o.expires_at,o.version,CASE WHEN o.incident_id IS NULL THEN '' ELSE 'PROVIDER_REPLACEMENT' END,COALESCE(o.incident_id::text,'')
		FROM dispatch_assignment_offers o JOIN dispatch_assignment_candidates c ON c.id=o.candidate_id
		WHERE o.provider_id=$1 AND o.status='OFFERED' AND o.expires_at>now() ORDER BY o.expires_at`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Offer
	for rows.Next() {
		var o Offer
		if err = rows.Scan(&o.ID, &o.MissionID, &o.ProviderID, &o.Status, &o.Rank, &o.OfferedAt, &o.ExpiresAt, &o.Version, &o.Purpose, &o.IncidentRef); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *Repository) OfferAuthorityContext(ctx context.Context, offerID, providerID string) (OfferAuthorityContext, error) {
	var out OfferAuthorityContext
	err := r.db.QueryRow(ctx, `SELECT o.mission_id::text,CASE WHEN o.incident_id IS NOT NULL AND COALESCE(t.organization_id,'')<>'' THEN 'ORGANIZATION_DISPATCH' ELSE COALESCE(p.strategy,'SEQUENTIAL_NEAREST') END,COALESCE(NULLIF(t.organization_id,''),p.organization_id::text,'') FROM dispatch_assignment_offers o LEFT JOIN dispatch_assignment_policies p ON p.mission_id=o.mission_id LEFT JOIN dispatch_delivery_incidents i ON i.id=o.incident_id LEFT JOIN trips t ON t.id=i.trip_id WHERE o.id=$1 AND o.provider_id=$2 AND o.status IN ('OFFERED','ACCEPTED')`, offerID, providerID).Scan(&out.MissionID, &out.Strategy, &out.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

func (r *Repository) ListAcceptedWithoutTrip(ctx context.Context, limit int) ([]PendingWinner, error) {
	rows, err := r.db.Query(ctx, `SELECT o.mission_id::text,o.provider_id::text FROM dispatch_assignment_offers o JOIN dispatch_delivery_missions m ON m.id=o.mission_id WHERE o.status='ACCEPTED' AND m.trip_id IS NULL ORDER BY o.responded_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingWinner
	for rows.Next() {
		var p PendingWinner
		if err = rows.Scan(&p.MissionID, &p.ProviderID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) AddCandidates(ctx context.Context, missionID string, candidates []Candidate) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM dispatch_delivery_missions WHERE id=$1 FOR UPDATE`, missionID).Scan(&locked); err != nil {
		return err
	}
	var rank int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(rank),0) FROM dispatch_assignment_candidates WHERE mission_id=$1`, missionID).Scan(&rank); err != nil {
		return err
	}
	for _, candidate := range candidates {
		snapshot, _ := json.Marshal(candidate.Snapshot)
		rank++
		if _, err = tx.Exec(ctx, `INSERT INTO dispatch_assignment_candidates(mission_id,provider_id,rank,distance_meters,candidate_snapshot) VALUES($1,$2,$3,$4,$5) ON CONFLICT(mission_id,provider_id) DO NOTHING`, missionID, candidate.ProviderID, rank, candidate.DistanceM, snapshot); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) Accept(ctx context.Context, offerID, providerID string, d Decision, now time.Time) (Assignment, error) {
	if replacement, err := r.replacementOffer(ctx, offerID); err != nil {
		return Assignment{}, err
	} else if replacement {
		return r.acceptReplacement(ctx, offerID, providerID, d, now)
	}
	fp := fingerprint(struct {
		Offer, Provider string
		Version         int64
	}{offerID, providerID, d.ExpectedVersion})
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var o Offer
	var existingKey, existingFP *string
	err = tx.QueryRow(ctx, `SELECT id::text,mission_id::text,provider_id::text,status,offered_at,expires_at,version,idempotency_key,request_fingerprint
		FROM dispatch_assignment_offers WHERE id=$1 FOR UPDATE`, offerID).Scan(&o.ID, &o.MissionID, &o.ProviderID, &o.Status, &o.OfferedAt, &o.ExpiresAt, &o.Version, &existingKey, &existingFP)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrNotFound
	}
	if err != nil {
		return Assignment{}, err
	}
	if o.ProviderID != providerID {
		return Assignment{}, ErrNotFound
	}
	if o.Status == "ACCEPTED" {
		if existingKey == nil || *existingKey != d.IdempotencyKey || existingFP == nil || *existingFP != fp {
			return Assignment{}, ErrConflict
		}
		var tripID *string
		_ = tx.QueryRow(ctx, `SELECT trip_id::text FROM dispatch_delivery_missions WHERE id=$1`, o.MissionID).Scan(&tripID)
		var strategy Strategy
		_ = tx.QueryRow(ctx, `SELECT strategy FROM dispatch_assignment_policies WHERE mission_id=$1`, o.MissionID).Scan(&strategy)
		result := Assignment{MissionID: o.MissionID, Strategy: strategy, Status: "ASSIGNED", Offer: &o, Replayed: true}
		if tripID != nil {
			result.TripID = *tripID
		}
		return result, tx.Commit(ctx)
	}
	if o.Status != "OFFERED" || !o.ExpiresAt.After(now) || o.Version != d.ExpectedVersion {
		return Assignment{}, ErrExpired
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT `+providerEligibleForMissionSQL+` FROM dispatch_delivery_missions m WHERE m.id=$2`, providerID, o.MissionID).Scan(&eligible); err != nil {
		return Assignment{}, err
	}
	if !eligible {
		return Assignment{}, ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='ACCEPTED',responded_at=$4,idempotency_key=$5,request_fingerprint=$6,version=version+1,updated_at=$4 WHERE id=$1 AND provider_id=$2 AND status='OFFERED' AND version=$3 AND expires_at>$4`, offerID, providerID, d.ExpectedVersion, now, d.IdempotencyKey, fp)
	if err != nil || tag.RowsAffected() != 1 {
		if unique(err) {
			return Assignment{}, ErrConflict
		}
		if err != nil {
			return Assignment{}, err
		}
		return Assignment{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='ASSIGNED',version=version+1,updated_at=$2 WHERE id=$1 AND state='OFFERING'`, o.MissionID, now); err != nil {
		return Assignment{}, err
	}
	_, _ = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='ASSIGNED',version=version+1,updated_at=$2 WHERE mission_id=$1 AND status IN ('QUEUED','RETRYING','WAITING_CUSTOMER','SCHEDULED')`, o.MissionID, now)
	if err = emit(ctx, tx, "dispatch.mission.assigned.v1", o.MissionID, o.Version+1, map[string]any{"offer_id": offerID, "provider_id": providerID}); err != nil {
		return Assignment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		if unique(err) {
			return Assignment{}, ErrConflict
		}
		return Assignment{}, err
	}
	o.Status = "ACCEPTED"
	o.Version++
	var strategy Strategy
	_ = r.db.QueryRow(ctx, `SELECT strategy FROM dispatch_assignment_policies WHERE mission_id=$1`, o.MissionID).Scan(&strategy)
	return Assignment{MissionID: o.MissionID, Strategy: strategy, Status: "ASSIGNED", Offer: &o}, nil
}

func (r *Repository) Decline(ctx context.Context, offerID, providerID string, d Decision, now time.Time, window time.Duration) (Assignment, error) {
	if replacement, err := r.replacementOffer(ctx, offerID); err != nil {
		return Assignment{}, err
	} else if replacement {
		return r.declineReplacement(ctx, offerID, providerID, d, now)
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var missionID, status string
	var strategy Strategy
	var storedKey, storedFP *string
	var version int64
	err = tx.QueryRow(ctx, `SELECT o.mission_id::text,o.status,o.version,o.idempotency_key,o.request_fingerprint,p.strategy FROM dispatch_assignment_offers o JOIN dispatch_assignment_policies p ON p.mission_id=o.mission_id WHERE o.id=$1 AND o.provider_id=$2 FOR UPDATE OF o`, offerID, providerID).Scan(&missionID, &status, &version, &storedKey, &storedFP, &strategy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrNotFound
	}
	if err != nil {
		return Assignment{}, err
	}
	fp := fingerprint(d)
	if status == "DECLINED" {
		if storedKey == nil || *storedKey != d.IdempotencyKey || storedFP == nil || *storedFP != fp {
			return Assignment{}, ErrConflict
		}
		return Assignment{MissionID: missionID, Strategy: strategy, Status: "OFFERING", Replayed: true}, tx.Commit(ctx)
	}
	if status != "OFFERED" || version != d.ExpectedVersion {
		return Assignment{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='DECLINED',responded_at=$3,decline_reason=$4,idempotency_key=$5,request_fingerprint=$6,version=version+1,updated_at=$3 WHERE id=$1 AND provider_id=$2`, offerID, providerID, now, d.Reason, d.IdempotencyKey, fp)
	if err != nil {
		return Assignment{}, err
	}
	if err = emit(ctx, tx, "dispatch.offer.declined.v1", missionID, version+1, map[string]any{"offer_id": offerID, "provider_id": providerID, "reason": d.Reason}); err != nil {
		return Assignment{}, err
	}
	// Section 14: a decline FOR safety needs operations to look at the vehicle
	// or route before it is offered again. Queued in the decline's own
	// transaction; delivered after commit by the notification worker.
	if networkanalytics.IsSafetyDecline(d.Reason) {
		if err = emitSafetyDecline(ctx, tx, offerID, missionID, providerID, d.Reason); err != nil {
			return Assignment{}, err
		}
	}
	next, err := createNextOffer(ctx, tx, missionID, now, window)
	if err != nil {
		return Assignment{}, err
	}
	result := Assignment{MissionID: missionID, Strategy: strategy, Status: "OFFERING", Offer: next}
	if next == nil {
		tag, queueErr := tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='QUEUED',next_attempt_at=$2,last_error_code='CANDIDATES_EXHAUSTED',version=version+1,updated_at=$2 WHERE mission_id=$1 AND status IN ('SCHEDULED','RETRYING','WAITING_CUSTOMER')`, missionID, now)
		if queueErr != nil {
			return Assignment{}, queueErr
		}
		state, event := "QUEUED", "dispatch.mission.queued.v1"
		result.Status = "QUEUED"
		if tag.RowsAffected() == 0 {
			state, event, result.Status = "FAILED", "dispatch.mission.no_provider.v1", "NO_PROVIDER_FOUND"
		}
		_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state=$2,version=version+1,updated_at=$3 WHERE id=$1`, missionID, state, now)
		if err == nil && state == "FAILED" {
			_, err = tx.Exec(ctx, `UPDATE request_broadcasts b SET status='no_provider_found',updated_at=$2 FROM dispatch_delivery_missions m WHERE m.id=$1 AND b.booking_id=m.booking_id AND b.status='cancelled'`, missionID, now)
		}
		if err == nil {
			err = emit(ctx, tx, event, missionID, version+2, map[string]any{"reason": "CANDIDATES_EXHAUSTED"})
		}
	}
	if err != nil {
		return Assignment{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Assignment{}, err
	}
	return result, nil
}

func (r *Repository) ExpireOffers(ctx context.Context, now time.Time, window time.Duration, limit int) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id::text,mission_id::text FROM dispatch_assignment_offers WHERE incident_id IS NULL AND status='OFFERED' AND expires_at<=$1 ORDER BY expires_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	type pair struct{ id, mission string }
	var due []pair
	for rows.Next() {
		var p pair
		if err = rows.Scan(&p.id, &p.mission); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, p)
	}
	rows.Close()
	for _, p := range due {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='EXPIRED',responded_at=$2,version=version+1,updated_at=$2 WHERE id=$1`, p.id, now); err != nil {
			return 0, err
		}
		if err = emit(ctx, tx, "dispatch.offer.expired.v1", p.mission, now.UnixNano(), map[string]any{"offer_id": p.id, "expired_at": now}); err != nil {
			return 0, err
		}
		allowed, e := queueAllowsOffer(ctx, tx, p.mission, now)
		if e != nil {
			return 0, e
		}
		var next *Offer
		if allowed {
			next, e = createNextOffer(ctx, tx, p.mission, now, window)
		} else {
			if _, e = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='EXPIRED',wallet_operation_ref='queue-release-'||id::text,wallet_state='REQUESTED',last_error_code='QUEUE_EXPIRED',version=version+1,updated_at=$2 WHERE mission_id=$1 AND status IN ('QUEUED','RETRYING','SCHEDULED','WAITING_CUSTOMER')`, p.mission, now); e != nil {
				return 0, e
			}
			if _, e = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='FAILED',version=version+1,updated_at=$2 WHERE id=$1 AND state='OFFERING'`, p.mission, now); e != nil {
				return 0, e
			}
			continue
		}
		if e != nil {
			return 0, e
		}
		if next == nil {
			tag, queueErr := tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='QUEUED',next_attempt_at=$2,last_error_code='CANDIDATES_EXHAUSTED',version=version+1,updated_at=$2 WHERE mission_id=$1 AND status IN ('SCHEDULED','RETRYING','WAITING_CUSTOMER')`, p.mission, now)
			if queueErr != nil {
				return 0, queueErr
			}
			state, event := "QUEUED", "dispatch.mission.queued.v1"
			if tag.RowsAffected() == 0 {
				state, event = "FAILED", "dispatch.mission.no_provider.v1"
			}
			if _, e = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state=$2,version=version+1,updated_at=$3 WHERE id=$1 AND state='OFFERING'`, p.mission, state, now); e != nil {
				return 0, e
			}
			if state == "FAILED" {
				if _, e = tx.Exec(ctx, `UPDATE request_broadcasts b SET status='no_provider_found',updated_at=$2 FROM dispatch_delivery_missions m WHERE m.id=$1 AND b.booking_id=m.booking_id AND b.status='cancelled'`, p.mission, now); e != nil {
					return 0, e
				}
			}
			if e = emit(ctx, tx, event, p.mission, now.UnixNano()+1, map[string]any{"reason": "CANDIDATES_EXHAUSTED"}); e != nil {
				return 0, e
			}
		}
	}
	return len(due), tx.Commit(ctx)
}

func (r *Repository) ApplyQueueOption(ctx context.Context, missionID, customerID string, option QueueOption, now time.Time) (QueueEntry, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return QueueEntry{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	fp := fingerprint(option)
	var storedFP string
	err = tx.QueryRow(ctx, `SELECT request_fingerprint FROM dispatch_queue_operations WHERE idempotency_key=$1`, option.IdempotencyKey).Scan(&storedFP)
	if err == nil {
		if storedFP != fp {
			return QueueEntry{}, ErrConflict
		}
		q, readErr := readCustomerQueue(ctx, tx, missionID, customerID)
		q.Replayed = true
		if readErr == nil {
			readErr = tx.Commit(ctx)
		}
		return q, readErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return QueueEntry{}, err
	}
	status := QueueQueued
	next := now
	switch option.Option {
	case "CANCEL":
		status = QueueCancelled
	case "SCHEDULE":
		status = QueueScheduled
		next = *option.ScheduledFor
	case "REQUOTE", "EXPANDED_SEARCH":
		status = QueueRetrying
	}
	var q QueueEntry
	err = tx.QueryRow(ctx, `UPDATE dispatch_queue_entries q SET status=$3,customer_option=$4,scheduled_for=$5,next_attempt_at=$6,quote_ref=CASE WHEN $7='' THEN quote_ref ELSE $7 END,quote_expires_at=COALESCE($8,quote_expires_at),current_radius_km=CASE WHEN $4='EXPANDED_SEARCH' THEN LEAST(max_radius_km,current_radius_km*2) ELSE current_radius_km END,version=q.version+1,updated_at=$9 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE q.mission_id=$1 AND m.id=q.mission_id AND g.customer_id=$2 AND q.status IN ('QUEUED','RETRYING','WAITING_CUSTOMER','SCHEDULED') AND NOT EXISTS(SELECT 1 FROM dispatch_assignment_offers o WHERE o.mission_id=q.mission_id AND o.status='OFFERED') RETURNING q.id::text,q.mission_id::text,q.status,q.priority,q.retry_count,q.next_attempt_at,q.expires_at,q.version`, missionID, customerID, status, option.Option, option.ScheduledFor, next, option.QuoteRef, option.QuoteExpiresAt, now).Scan(&q.ID, &q.MissionID, &q.Status, &q.Priority, &q.RetryCount, &q.NextAttemptAt, &q.ExpiresAt, &q.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, ErrNotFound
	}
	if err != nil {
		return q, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_queue_operations(queue_id,mission_id,idempotency_key,request_fingerprint,option) VALUES($1,$2,$3,$4,$5)`, q.ID, missionID, option.IdempotencyKey, fp, option.Option); err != nil {
		if unique(err) {
			return QueueEntry{}, ErrConflict
		}
		return QueueEntry{}, err
	}
	if err = emit(ctx, tx, "dispatch.queue.option_selected.v1", missionID, q.Version, map[string]any{"queue_id": q.ID, "option": option.Option, "status": q.Status}); err != nil {
		return QueueEntry{}, err
	}
	if option.Option == "CANCEL" {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='CANCELLED',version=version+1,updated_at=$2 WHERE id=$1 AND state IN ('PLANNED','QUEUED','OFFERING')`, missionID, now); err != nil {
			return QueueEntry{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET wallet_operation_ref='queue-release-'||id::text,wallet_state='REQUESTED' WHERE id=$1`, q.ID); err != nil {
			return QueueEntry{}, err
		}
	}
	return q, tx.Commit(ctx)
}

func (r *Repository) ApplyPartnerQueueOption(ctx context.Context, missionID, sourceRef string, sourceVersion int64, option QueueOption, now time.Time) (QueueEntry, error) {
	var customerID *string
	err := r.db.QueryRow(ctx, `SELECT g.customer_id::text FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.id=$1 AND g.source_type='PARTNER_API' AND g.source_ref=$2 AND g.source_version=$3`, missionID, sourceRef, sourceVersion).Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) || customerID == nil {
		return QueueEntry{}, ErrNotFound
	}
	if err != nil {
		return QueueEntry{}, err
	}
	return r.ApplyQueueOption(ctx, missionID, *customerID, option, now)
}

func readCustomerQueue(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, missionID, customerID string) (QueueEntry, error) {
	var out QueueEntry
	err := q.QueryRow(ctx, `SELECT q.id::text,q.mission_id::text,q.status,q.priority,q.retry_count,q.next_attempt_at,q.expires_at,q.version FROM dispatch_queue_entries q JOIN dispatch_delivery_missions m ON m.id=q.mission_id JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE q.mission_id=$1 AND g.customer_id=$2`, missionID, customerID).Scan(&out.ID, &out.MissionID, &out.Status, &out.Priority, &out.RetryCount, &out.NextAttemptAt, &out.ExpiresAt, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

func (r *Repository) ListDueQueueMissions(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT mission_id::text FROM dispatch_queue_entries WHERE status IN ('QUEUED','RETRYING','SCHEDULED') AND next_attempt_at<=$1 AND expires_at>$1 AND (customer_decision_deadline IS NULL OR customer_decision_deadline>$1) ORDER BY priority DESC,next_attempt_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) ProcessQueue(ctx context.Context, now time.Time, window, retryInterval time.Duration, limit int) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id::text,mission_id::text,quote_expires_at,expires_at,customer_decision_deadline,serviceability_ref FROM dispatch_queue_entries WHERE status IN ('QUEUED','RETRYING','SCHEDULED') AND next_attempt_at<=$1 ORDER BY priority DESC,next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	type item struct {
		id, mission       string
		quote             *time.Time
		expires           time.Time
		decision          *time.Time
		serviceabilityRef string
	}
	var items []item
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.id, &x.mission, &x.quote, &x.expires, &x.decision, &x.serviceabilityRef); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, x)
	}
	rows.Close()
	for _, x := range items {
		if !x.expires.After(now) || (x.decision != nil && !x.decision.After(now)) {
			if _, err = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='EXPIRED',wallet_operation_ref='queue-release-'||id::text,wallet_state='REQUESTED',last_error_code='QUEUE_EXPIRED',version=version+1,updated_at=$2 WHERE id=$1`, x.id, now); err != nil {
				return 0, err
			}
			_, _ = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='FAILED',version=version+1,updated_at=$2 WHERE id=$1 AND state IN ('PLANNED','QUEUED','OFFERING')`, x.mission, now)
			continue
		}
		var current bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id JOIN dispatch_bookings b ON b.id=m.booking_id WHERE m.id=$1 AND m.state IN ('PLANNED','QUEUED','OFFERING') AND g.state NOT IN ('CANCELLED','FAILED','DELIVERED','COMPLETED','RETURNING','RETURNED') AND b.status='searching' AND NOT EXISTS(SELECT 1 FROM trips t WHERE t.booking_id=b.id) AND NOT EXISTS(SELECT 1 FROM dispatch_parcel_manifests pm JOIN dispatch_manifest_parcels mp ON mp.manifest_id=pm.id JOIN dispatch_parcels p ON p.id=mp.parcel_id WHERE pm.mission_id=m.id AND pm.status IN ('ACTIVE','CUSTODY_ACTIVE') AND p.admission_result IN ('REFUSED_PROHIBITED','REFUSED_UNSAFE','OPERATIONAL_REVIEW')))`, x.mission).Scan(&current)
		if err != nil {
			return 0, err
		}
		if !current {
			_, err = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='CANCELLED',last_error_code='MISSION_TERMINAL',version=version+1,updated_at=$2 WHERE id=$1`, x.id, now)
			if err != nil {
				return 0, err
			}
			continue
		}
		if x.serviceabilityRef != "" {
			var valid bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dispatch_serviceability_quotes WHERE id=$1 AND expires_at>$2 AND decision IN ('SERVICEABLE','SERVICEABLE_WITH_CONDITIONS'))`, x.serviceabilityRef, now).Scan(&valid); err != nil {
				return 0, err
			}
			if !valid {
				_, err = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='WAITING_CUSTOMER',last_error_code='SERVICEABILITY_STALE',version=version+1,updated_at=$2 WHERE id=$1`, x.id, now)
				if err != nil {
					return 0, err
				}
				continue
			}
		}
		if x.quote != nil && !x.quote.After(now) {
			_, err = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='WAITING_CUSTOMER',customer_option=NULL,last_error_code='QUOTE_STALE',version=version+1,updated_at=$2 WHERE id=$1`, x.id, now)
			if err != nil {
				return 0, err
			}
			if err = emit(ctx, tx, "dispatch.queue.requote_required.v1", x.mission, now.UnixNano(), map[string]any{"queue_id": x.id}); err != nil {
				return 0, err
			}
			continue
		}
		next, e := createNextOffer(ctx, tx, x.mission, now, window)
		if e != nil {
			return 0, e
		}
		if next != nil {
			_, e = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status='RETRYING',retry_count=retry_count+1,next_attempt_at=$2::timestamptz+$3::interval,version=version+1,updated_at=$2::timestamptz WHERE id=$1`, x.id, now, retryInterval.String())
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='OFFERING',version=version+1,updated_at=$2 WHERE id=$1`, x.mission, now)
			}
		} else {
			_, e = tx.Exec(ctx, `UPDATE dispatch_queue_entries SET status=CASE WHEN expires_at<=$2 THEN 'EXPIRED' ELSE 'WAITING_CUSTOMER' END,retry_count=retry_count+1,version=version+1,updated_at=$2 WHERE id=$1`, x.id, now)
			if e == nil {
				e = emit(ctx, tx, "dispatch.queue.exhausted.v1", x.mission, now.UnixNano(), map[string]any{"queue_id": x.id})
			}
		}
		if e != nil {
			return 0, e
		}
	}
	return len(items), tx.Commit(ctx)
}

func queueAllowsOffer(ctx context.Context, tx pgx.Tx, missionID string, now time.Time) (bool, error) {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM dispatch_queue_entries WHERE mission_id=$1) OR EXISTS(SELECT 1 FROM dispatch_queue_entries WHERE mission_id=$1 AND status IN ('QUEUED','RETRYING','SCHEDULED') AND expires_at>$2 AND (customer_decision_deadline IS NULL OR customer_decision_deadline>$2))`, missionID, now).Scan(&allowed)
	return allowed, err
}

func (r *Repository) ListPendingQueueFinancial(ctx context.Context, limit int) ([]PendingQueueFinancial, error) {
	rows, err := r.db.Query(ctx, `SELECT q.id::text,q.mission_id::text,m.booking_id::text,q.wallet_operation_ref FROM dispatch_queue_entries q JOIN dispatch_delivery_missions m ON m.id=q.mission_id WHERE q.wallet_state IN ('REQUESTED','UNKNOWN') AND q.wallet_operation_ref<>'' AND m.booking_id IS NOT NULL ORDER BY q.updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingQueueFinancial
	for rows.Next() {
		var item PendingQueueFinancial
		if err = rows.Scan(&item.QueueID, &item.MissionID, &item.BookingID, &item.OperationRef); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) MarkQueueFinancial(ctx context.Context, queueID, state, code string) error {
	if state != "ACKNOWLEDGED" && state != "UNKNOWN" && state != "FAILED" {
		return ErrConflict
	}
	_, err := r.db.Exec(ctx, `UPDATE dispatch_queue_entries SET wallet_state=$2,wallet_error_code=$3,wallet_lease_owner=NULL,wallet_lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND wallet_state IN ('REQUESTED','UNKNOWN')`, queueID, state, code)
	return err
}

func (r *Repository) ClaimPendingQueueFinancial(ctx context.Context, now time.Time, owner string, lease time.Duration, maxAttempts, limit int) ([]PendingQueueFinancial, error) {
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	rows, err := r.db.Query(ctx, `WITH due AS (
		SELECT q.id FROM dispatch_queue_entries q JOIN dispatch_delivery_missions m ON m.id=q.mission_id
		WHERE q.wallet_state IN ('REQUESTED','UNKNOWN') AND q.wallet_operation_ref<>'' AND m.booking_id IS NOT NULL
		  AND q.wallet_attempt_count<$4 AND q.wallet_next_attempt_at<=$1
		  AND (q.wallet_lease_expires_at IS NULL OR q.wallet_lease_expires_at<=$1)
		ORDER BY q.wallet_next_attempt_at,q.updated_at FOR UPDATE OF q SKIP LOCKED LIMIT $5
	), claimed AS (
		UPDATE dispatch_queue_entries q SET wallet_lease_owner=$2,wallet_lease_expires_at=$1::timestamptz+$3::interval,updated_at=$1::timestamptz
		FROM due WHERE q.id=due.id RETURNING q.id,q.mission_id,q.wallet_operation_ref,q.wallet_attempt_count
	)
	SELECT c.id::text,c.mission_id::text,m.booking_id::text,c.wallet_operation_ref,c.wallet_attempt_count
	FROM claimed c JOIN dispatch_delivery_missions m ON m.id=c.mission_id`, now, owner, lease.String(), maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PendingQueueFinancial
	for rows.Next() {
		var item PendingQueueFinancial
		if err = rows.Scan(&item.QueueID, &item.MissionID, &item.BookingID, &item.OperationRef, &item.AttemptCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) CompleteQueueFinancial(ctx context.Context, queueID, state, code string, now time.Time, backoff time.Duration, maxAttempts int, owner string) error {
	if state != "ACKNOWLEDGED" && state != "UNKNOWN" && state != "FAILED" {
		return ErrConflict
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	_, err := r.db.Exec(ctx, `UPDATE dispatch_queue_entries SET
		wallet_attempt_count=wallet_attempt_count+1,
		wallet_state=CASE WHEN $2='ACKNOWLEDGED' THEN 'ACKNOWLEDGED' WHEN wallet_attempt_count+1 >= $6 THEN 'FAILED' ELSE $2 END,
		wallet_error_code=$3,wallet_next_attempt_at=$4::timestamptz+$5::interval,
		wallet_terminal_at=CASE WHEN $2='ACKNOWLEDGED' OR wallet_attempt_count+1 >= $6 THEN $4::timestamptz ELSE NULL END,
		wallet_lease_owner=NULL,wallet_lease_expires_at=NULL,updated_at=$4::timestamptz
		WHERE id=$1 AND wallet_state IN ('REQUESTED','UNKNOWN') AND wallet_lease_owner=$7`, queueID, state, code, now, backoff.String(), maxAttempts, owner)
	return err
}

func normalizeServiceTier(value string) string {
	if value == "express" || value == "EXPRESS" {
		return "express"
	}
	return "standard"
}

func emit(ctx context.Context, tx pgx.Tx, eventType, id string, version int64, payload any) error {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES($1,'delivery_mission',$2,$3,$4,$5,'vendor-service') ON CONFLICT DO NOTHING`, eventType, id, version, raw, hex.EncodeToString(sum[:]))
	return err
}

func (r *Repository) LinkTrip(ctx context.Context, missionID, tripID string) error {
	tag, err := r.db.Exec(ctx, `UPDATE dispatch_delivery_missions SET trip_id=$2,version=version+1,updated_at=now() WHERE id=$1 AND (trip_id IS NULL OR trip_id=$2)`, missionID, tripID)
	if err == nil && tag.RowsAffected() != 1 {
		return fmt.Errorf("mission trip link conflict")
	}
	return err
}
