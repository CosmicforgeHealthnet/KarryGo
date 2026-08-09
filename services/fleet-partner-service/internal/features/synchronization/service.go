package synchronization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

type Flags struct{ Enabled, WorkerEnabled, KillSwitch bool }
type AuthoritativeState struct {
	SourceVersion int64
	SafeState     json.RawMessage
}
type Authority interface {
	Read(context.Context, string, string) (AuthoritativeState, error)
}
type Service struct {
	pool        *pgxpool.Pool
	flags       Flags
	now         func() time.Time
	authorities map[string]Authority
}

func New(pool *pgxpool.Pool, flags Flags) *Service {
	return &Service{pool: pool, flags: flags, now: func() time.Time { return time.Now().UTC() }, authorities: map[string]Authority{}}
}
func (s *Service) RegisterAuthority(eventType string, authority Authority) {
	s.authorities[eventType] = authority
}

// AuthorityConfigured reports whether at least one real Authority has been
// registered -- used by retention's SupersededProjectionExecutor wiring to
// decide whether it can be offered at all (see internal/app/wiring.go and
// internal/app/retentionexec_adapter.go): a synchronization service with
// zero registered authorities can never confirm a projection is genuinely
// superseded, so the executor must not be registered in that state rather
// than silently finding zero candidates forever.
func (s *Service) AuthorityConfigured() bool { return len(s.authorities) > 0 }

// AuthoritySourceVersion resolves the live source_version a registered
// Authority reports for (eventType, organizationID, subjectReference) --
// exposes RegisterAuthority's existing per-eventType registry to external
// confirmation callers (retention's SupersededProjectionExecutor) without
// duplicating the registration itself. eventType is this service's own
// projection_type/event-type vocabulary (see RegisterAuthority's call
// sites in internal/app/section26_adapters.go for the real registered
// values), not a general lookup key.
func (s *Service) AuthoritySourceVersion(ctx context.Context, eventType, organizationID, subjectReference string) (int64, error) {
	authority, ok := s.authorities[eventType]
	if !ok {
		return 0, fmt.Errorf("synchronization: no authority registered for event type %q", eventType)
	}
	state, err := authority.Read(ctx, organizationID, subjectReference)
	if err != nil {
		return 0, err
	}
	return state.SourceVersion, nil
}

func (s *Service) ApplyTx(ctx context.Context, tx pgx.Tx, envelope events.Envelope) error {
	if !s.flags.Enabled || s.flags.KillSwitch {
		return errors.New("synchronization disabled")
	}
	if err := events.ValidateEnvelope(envelope); err != nil {
		return err
	}
	var payload events.ContractPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return err
	}
	sum := sha256.Sum256(envelope.Payload)
	hash := hex.EncodeToString(sum[:])
	now := s.now()
	var id, currentHash, state string
	var current int64
	err := tx.QueryRow(ctx, `SELECT id,source_version,payload_hash,gap_conflict_state FROM fleet_partner_synchronization_projections WHERE organization_id=$1 AND projection_type=$2 AND source_service=$3 AND subject_reference=$4 FOR UPDATE`, envelope.OrganizationID, envelope.EventType, envelope.Producer, payload.SubjectReference).Scan(&id, &current, &currentHash, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		id = ids.New("syn")
		gap := "CURRENT"
		reconcile := "IDLE"
		var next any
		if payload.SourceVersion > 1 {
			gap = "GAP"
			reconcile = "PENDING"
			next = now
		}
		_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_projections(id,organization_id,projection_type,source_service,subject_reference,source_version,payload_hash,safe_state,gap_conflict_state,last_event_at,next_reconciliation_at,reconciliation_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, envelope.OrganizationID, envelope.EventType, envelope.Producer, payload.SubjectReference, payload.SourceVersion, hash, envelope.Payload, gap, now, next, reconcile)
		if err != nil {
			return err
		}
		if gap == "GAP" {
			return s.openCase(ctx, tx, envelope.OrganizationID, id, "VERSION_GAP")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if payload.SourceVersion < current {
		_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_attempts(id,organization_id,projection_id,attempt,outcome,safe_error_code) VALUES($1,$2,$3,1,'LOWER_VERSION_REJECTED','LOWER_VERSION')`, ids.New("sya"), envelope.OrganizationID, id)
		return err
	}
	if payload.SourceVersion == current {
		if hash == currentHash {
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE fleet_partner_synchronization_projections SET gap_conflict_state='CONFLICT',reconciliation_status='PENDING',next_reconciliation_at=now(),last_safe_error_code='SAME_VERSION_CONFLICT',updated_at=now() WHERE id=$1`, id)
		if err != nil {
			return err
		}
		return s.openCase(ctx, tx, envelope.OrganizationID, id, "SAME_VERSION_CONFLICT")
	}
	gap := payload.SourceVersion > current+1
	gapState := "CURRENT"
	reconcile := "IDLE"
	var next any
	if gap {
		gapState = "GAP"
		reconcile = "PENDING"
		next = now
	}
	_, err = tx.Exec(ctx, `UPDATE fleet_partner_synchronization_projections SET source_version=$2,payload_hash=$3,safe_state=$4,gap_conflict_state=$5,last_event_at=$6,next_reconciliation_at=$7,reconciliation_status=$8,last_safe_error_code='',updated_at=now() WHERE id=$1`, id, payload.SourceVersion, hash, envelope.Payload, gapState, now, next, reconcile)
	if err != nil {
		return err
	}
	if gap {
		return s.openCase(ctx, tx, envelope.OrganizationID, id, "VERSION_GAP")
	}
	return nil
}
func (s *Service) openCase(ctx context.Context, tx pgx.Tx, org, id, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_cases(id,organization_id,projection_id,reason) SELECT $1,$2,$3,$4 WHERE NOT EXISTS(SELECT 1 FROM fleet_partner_synchronization_cases WHERE projection_id=$3 AND status IN('OPEN','LEASED'))`, ids.New("syc"), org, id, reason)
	return err
}

func (s *Service) ListProjections(ctx context.Context, org string) ([]map[string]any, error) {
	return s.list(ctx, `SELECT jsonb_build_object('id',id,'projectionType',projection_type,'sourceService',source_service,'subjectReference',subject_reference,'sourceVersion',source_version,'gapConflictState',gap_conflict_state,'lastEventAt',last_event_at,'lastReconciledAt',last_reconciled_at,'nextReconciliationAt',next_reconciliation_at,'reconciliationStatus',reconciliation_status,'lastSafeErrorCode',last_safe_error_code) FROM fleet_partner_synchronization_projections WHERE organization_id=$1 ORDER BY updated_at DESC`, org)
}
func (s *Service) ListCases(ctx context.Context, org string) ([]map[string]any, error) {
	return s.list(ctx, `SELECT jsonb_build_object('id',id,'projectionId',projection_id,'reason',reason,'status',status,'attempts',attempts,'nextAttemptAt',next_attempt_at,'lastSafeErrorCode',last_safe_error_code,'createdAt',created_at,'updatedAt',updated_at) FROM fleet_partner_synchronization_cases WHERE organization_id=$1 ORDER BY created_at DESC`, org)
}
func (s *Service) ListDeadLetters(ctx context.Context, org string) ([]map[string]any, error) {
	return s.list(ctx, `SELECT jsonb_build_object('id',id,'sourceService',source_service,'eventId',event_id,'eventType',event_type,'subjectReference',subject_reference,'sourceVersion',source_version,'status',status,'attempts',attempts,'lastSafeErrorCode',last_safe_error_code,'createdAt',created_at) FROM fleet_partner_synchronization_dead_letters WHERE organization_id=$1 ORDER BY created_at DESC`, org)
}
func (s *Service) list(ctx context.Context, query, org string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, query, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value map[string]any
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
func (s *Service) GetCase(ctx context.Context, org, id string) (map[string]any, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'projectionId',projection_id,'reason',reason,'status',status,'attempts',attempts,'nextAttemptAt',next_attempt_at,'lastSafeErrorCode',last_safe_error_code,'createdAt',created_at,'updatedAt',updated_at) FROM fleet_partner_synchronization_cases WHERE organization_id=$1 AND id=$2`, org, id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	err = json.Unmarshal(raw, &value)
	return value, err
}
func (s *Service) GetDeadLetter(ctx context.Context, org, id string) (map[string]any, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'sourceService',source_service,'eventId',event_id,'eventType',event_type,'subjectReference',subject_reference,'sourceVersion',source_version,'status',status,'attempts',attempts,'lastSafeErrorCode',last_safe_error_code,'createdAt',created_at) FROM fleet_partner_synchronization_dead_letters WHERE organization_id=$1 AND id=$2`, org, id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	err = json.Unmarshal(raw, &value)
	return value, err
}
func (s *Service) Reconcile(ctx context.Context, org, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE fleet_partner_synchronization_cases SET status='OPEN',next_attempt_at=now(),last_safe_error_code='',updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('OPEN','DEAD_LETTER')`, org, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
func (s *Service) Replay(ctx context.Context, org, id string) error {
	return databaseWithin(ctx, s.pool, func(tx pgx.Tx) error {
		var projectionID string
		err := tx.QueryRow(ctx, `SELECT p.id FROM fleet_partner_synchronization_dead_letters d JOIN fleet_partner_synchronization_projections p ON p.organization_id=d.organization_id AND p.source_service=d.source_service AND p.projection_type=d.event_type AND p.subject_reference=d.subject_reference WHERE d.organization_id=$1 AND d.id=$2 AND d.status='OPEN' FOR UPDATE OF d`, org, id).Scan(&projectionID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE fleet_partner_synchronization_dead_letters SET status='REPLAYED',replayed_at=now(),attempts=attempts+1 WHERE organization_id=$1 AND id=$2`, org, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE fleet_partner_synchronization_cases SET status='OPEN',next_attempt_at=now(),lease_owner=NULL,lease_expires_at=NULL,last_safe_error_code='',resolved_at=NULL,updated_at=now() WHERE organization_id=$1 AND projection_id=$2 AND status='DEAD_LETTER'`, org, projectionID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err = tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_cases(id,organization_id,projection_id,reason) VALUES($1,$2,$3,'DEAD_LETTER_REPLAY')`, ids.New("syc"), org, projectionID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE fleet_partner_synchronization_projections SET reconciliation_status='PENDING',next_reconciliation_at=now(),last_safe_error_code='',updated_at=now() WHERE id=$1`, projectionID)
		return err
	})
}

type WorkerConfig struct {
	Owner                  string
	BatchSize, MaxAttempts int
	Lease                  time.Duration
}
type claimedCase struct {
	ID, OrganizationID, ProjectionID, ProjectionType, SubjectReference string
	Attempts                                                           int
	CurrentVersion                                                     int64
}

func (s *Service) RunReconciliationPass(ctx context.Context, cfg WorkerConfig) (int, error) {
	if !s.flags.Enabled || !s.flags.WorkerEnabled || s.flags.KillSwitch {
		return 0, nil
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	var cases []claimedCase
	err := databaseWithin(ctx, s.pool, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT c.id,c.organization_id,c.projection_id,p.projection_type,p.subject_reference,c.attempts,p.source_version FROM fleet_partner_synchronization_cases c JOIN fleet_partner_synchronization_projections p ON p.id=c.projection_id WHERE (c.status='OPEN' OR(c.status='LEASED' AND c.lease_expires_at<now())) AND c.next_attempt_at<=now() ORDER BY c.created_at FOR UPDATE OF c SKIP LOCKED LIMIT $1`, cfg.BatchSize)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c claimedCase
			if e = rows.Scan(&c.ID, &c.OrganizationID, &c.ProjectionID, &c.ProjectionType, &c.SubjectReference, &c.Attempts, &c.CurrentVersion); e != nil {
				return e
			}
			cases = append(cases, c)
		}
		for _, c := range cases {
			_, e = tx.Exec(ctx, `UPDATE fleet_partner_synchronization_cases SET status='LEASED',lease_owner=$2,lease_expires_at=now()+make_interval(secs=>$3),heartbeat_at=now(),attempts=attempts+1,updated_at=now() WHERE id=$1`, c.ID, cfg.Owner, cfg.Lease.Seconds())
			if e != nil {
				return e
			}
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, c := range cases {
		s.reconcileOne(ctx, c, cfg)
	}
	return len(cases), nil
}
func databaseWithin(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) reconcileOne(ctx context.Context, c claimedCase, cfg WorkerConfig) {
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(cfg.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				_, _ = s.pool.Exec(heartbeatCtx, `UPDATE fleet_partner_synchronization_cases SET heartbeat_at=now(),lease_expires_at=now()+make_interval(secs=>$3),updated_at=now() WHERE id=$1 AND status='LEASED' AND lease_owner=$2`, c.ID, cfg.Owner, cfg.Lease.Seconds())
			}
		}
	}()
	defer func() {
		stopHeartbeat()
		<-heartbeatDone
	}()

	authority := s.authorities[c.ProjectionType]
	var state AuthoritativeState
	var err error
	if authority == nil {
		err = errors.New("authority unavailable")
	} else {
		state, err = authority.Read(ctx, c.OrganizationID, c.SubjectReference)
	}
	outcome := "RECONCILED"
	code := ""
	dead := false
	if err != nil || state.SourceVersion < c.CurrentVersion || state.SourceVersion < 1 {
		outcome = "RETRY"
		code = "AUTHORITATIVE_READ_FAILED"
		if c.Attempts+1 >= cfg.MaxAttempts {
			outcome = "DEAD_LETTER"
			dead = true
		}
	}
	_ = databaseWithin(ctx, s.pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_attempts(id,organization_id,projection_id,case_id,attempt,outcome,safe_error_code) VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("sya"), c.OrganizationID, c.ProjectionID, c.ID, c.Attempts+1, outcome, code); e != nil {
			return e
		}
		if !dead && err == nil && state.SourceVersion >= c.CurrentVersion {
			sum := sha256.Sum256(state.SafeState)
			if _, e := tx.Exec(ctx, `UPDATE fleet_partner_synchronization_projections SET source_version=$2,payload_hash=$3,safe_state=$4,gap_conflict_state='CURRENT',last_reconciled_at=now(),next_reconciliation_at=NULL,reconciliation_status='IDLE',last_safe_error_code='',lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, c.ProjectionID, state.SourceVersion, hex.EncodeToString(sum[:]), state.SafeState); e != nil {
				return e
			}
			_, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_synchronization_cases SET status='RESOLVED',resolved_at=now(),lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, c.ID)
			return updateErr
		}
		status := "OPEN"
		if dead {
			status = "DEAD_LETTER"
			_, _ = tx.Exec(ctx, `INSERT INTO fleet_partner_synchronization_dead_letters(id,organization_id,source_service,event_id,event_type,subject_reference,source_version,payload_hash,last_safe_error_code) SELECT $1,organization_id,source_service,$2,projection_type,subject_reference,source_version,payload_hash,$3 FROM fleet_partner_synchronization_projections WHERE id=$4 ON CONFLICT DO NOTHING`, ids.New("syd"), "reconciliation:"+c.ID, code, c.ProjectionID)
		}
		_, e := tx.Exec(ctx, `UPDATE fleet_partner_synchronization_cases SET status=$2,last_safe_error_code=$3,next_attempt_at=now()+make_interval(secs=>LEAST(300,power(2,attempts))),lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, c.ID, status, code)
		return e
	})
}
