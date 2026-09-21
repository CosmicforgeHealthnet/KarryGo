package supportrepositories

// Persistence for Section 15's network context: references, snapshots and
// action requests.
//
// It is a store of its own rather than more methods on SupportRepository so the
// existing repository interface -- and the in-memory double the rest of this
// feature's tests use -- does not change shape for a capability they do not
// exercise.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/supportcase"
)

// ErrActionKeyReused means an idempotency key was reused for a different
// request on the same complaint.
var ErrActionKeyReused = errors.New("support: idempotency key reused for a different action")

// ErrNetworkNotFound means the row does not exist.
var ErrNetworkNotFound = errors.New("support: network context not found")

// NetworkContextStore persists Section 15 state.
type NetworkContextStore struct {
	db *pgxpool.Pool
}

// NewNetworkContextStore builds the store.
func NewNetworkContextStore(db *pgxpool.Pool) *NetworkContextStore {
	return &NetworkContextStore{db: db}
}

// SetContextRefs records the references a complaint is about.
//
// Only references are written. There is no column here that could hold a
// state, so there is no way for this to become a second copy of one.
func (s *NetworkContextStore) SetContextRefs(ctx context.Context, complaintID string,
	refs supportmodels.ContextRefs) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE complaints SET
			context_type = NULLIF($2,''), order_ref = NULLIF($3,''),
			fulfilment_ref = NULLIF($4,''), mission_ref = NULLIF($5,''),
			provider_ref = NULLIF($6,''), asset_ref = NULLIF($7,''),
			guarantee_breach_ref = NULLIF($8,''), recovery_case_ref = NULLIF($9,''),
			updated_at = now()
		 WHERE id = $1`,
		complaintID, refs.ContextType, refs.OrderRef, refs.FulfilmentRef, refs.MissionRef,
		refs.ProviderRef, refs.AssetRef, refs.GuaranteeBreachRef, refs.RecoveryCaseRef)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNetworkNotFound
	}
	return nil
}

// GetContextRefs reads a complaint's references.
func (s *NetworkContextStore) GetContextRefs(ctx context.Context,
	complaintID string) (supportmodels.ContextRefs, error) {
	var refs supportmodels.ContextRefs
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(context_type,''), COALESCE(order_ref,''), COALESCE(fulfilment_ref,''),
		       COALESCE(mission_ref,''), COALESCE(provider_ref,''), COALESCE(asset_ref,''),
		       COALESCE(guarantee_breach_ref,''), COALESCE(recovery_case_ref,'')
		  FROM complaints WHERE id = $1`, complaintID).
		Scan(&refs.ContextType, &refs.OrderRef, &refs.FulfilmentRef, &refs.MissionRef,
			&refs.ProviderRef, &refs.AssetRef, &refs.GuaranteeBreachRef, &refs.RecoveryCaseRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return refs, ErrNetworkNotFound
	}
	return refs, err
}

// AppendSnapshot records what Support was shown. There is no update path: the
// table refuses one by trigger, and this store does not offer one.
func (s *NetworkContextStore) AppendSnapshot(ctx context.Context,
	snapshot supportmodels.ContextSnapshot) (supportmodels.ContextSnapshot, error) {
	// Serialised through the contract's own Minimize, so a field the owner
	// sent that is not on the contract cannot end up in the stored JSON.
	minimized, err := supportcase.Minimize(snapshot.Context)
	if err != nil {
		return snapshot, err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO complaint_context_snapshots
			(complaint_id, source_service, source_ref, source_version, minimized_json, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, observed_at`,
		snapshot.ComplaintID, snapshot.SourceService, snapshot.SourceRef, snapshot.SourceVersion,
		minimized, snapshot.CorrelationID).Scan(&snapshot.ID, &snapshot.ObservedAt)
	return snapshot, err
}

// ListSnapshots returns a complaint's snapshots, newest first.
func (s *NetworkContextStore) ListSnapshots(ctx context.Context,
	complaintID string) ([]supportmodels.ContextSnapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, complaint_id, source_service, source_ref, source_version,
		       minimized_json, observed_at, correlation_id
		  FROM complaint_context_snapshots WHERE complaint_id = $1
		 ORDER BY observed_at DESC, id`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []supportmodels.ContextSnapshot{}
	for rows.Next() {
		var snapshot supportmodels.ContextSnapshot
		var raw []byte
		if err = rows.Scan(&snapshot.ID, &snapshot.ComplaintID, &snapshot.SourceService,
			&snapshot.SourceRef, &snapshot.SourceVersion, &raw, &snapshot.ObservedAt,
			&snapshot.CorrelationID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &snapshot.Context); err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

const actionColumns = `id, complaint_id, target_service, action_code, target_ref, parameters,
	request_version, status, idempotency_key, request_fingerprint, result_ref, result_code,
	requested_by, attempts, created_at, updated_at, completed_at`

func scanAction(row pgx.Row) (supportmodels.ActionRequest, error) {
	var action supportmodels.ActionRequest
	var params []byte
	err := row.Scan(&action.ID, &action.ComplaintID, &action.TargetService, &action.ActionCode,
		&action.TargetRef, &params, &action.RequestVersion, &action.Status, &action.IdempotencyKey,
		&action.RequestFingerprint, &action.ResultRef, &action.ResultCode, &action.RequestedBy,
		&action.Attempts, &action.CreatedAt, &action.UpdatedAt, &action.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return action, ErrNetworkNotFound
	}
	if err != nil {
		return action, err
	}
	action.Parameters = map[string]string{}
	if len(params) > 0 {
		if err = json.Unmarshal(params, &action.Parameters); err != nil {
			return action, err
		}
	}
	return action, nil
}

// CreateActionRequest records a request, exactly once per key.
//
// Same key and same fingerprint returns the original row -- a retry is the same
// request. Same key and a different fingerprint is ErrActionKeyReused: the
// caller has two different requests and one key, and quietly returning either
// would lose the other.
func (s *NetworkContextStore) CreateActionRequest(ctx context.Context,
	action supportmodels.ActionRequest) (supportmodels.ActionRequest, bool, error) {
	params, err := json.Marshal(action.Parameters)
	if err != nil {
		return action, false, err
	}
	created, err := scanAction(s.db.QueryRow(ctx, `
		INSERT INTO support_action_requests
			(complaint_id, target_service, action_code, target_ref, parameters,
			 idempotency_key, request_fingerprint, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (complaint_id, idempotency_key) DO NOTHING
		RETURNING `+actionColumns,
		action.ComplaintID, action.TargetService, action.ActionCode, action.TargetRef, params,
		action.IdempotencyKey, action.RequestFingerprint, action.RequestedBy))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, ErrNetworkNotFound) {
		return action, false, err
	}
	existing, err := scanAction(s.db.QueryRow(ctx,
		`SELECT `+actionColumns+` FROM support_action_requests
		  WHERE complaint_id = $1 AND idempotency_key = $2`,
		action.ComplaintID, action.IdempotencyKey))
	if err != nil {
		return action, false, err
	}
	if existing.RequestFingerprint != action.RequestFingerprint {
		return existing, false, ErrActionKeyReused
	}
	return existing, false, nil
}

// GetActionRequest reads one request.
func (s *NetworkContextStore) GetActionRequest(ctx context.Context,
	complaintID, id string) (supportmodels.ActionRequest, error) {
	return scanAction(s.db.QueryRow(ctx,
		`SELECT `+actionColumns+` FROM support_action_requests WHERE complaint_id=$1 AND id=$2`,
		complaintID, id))
}

// ListActionRequests reads a complaint's requests, newest first.
func (s *NetworkContextStore) ListActionRequests(ctx context.Context,
	complaintID string) ([]supportmodels.ActionRequest, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+actionColumns+` FROM support_action_requests WHERE complaint_id=$1
		 ORDER BY created_at DESC, id`, complaintID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []supportmodels.ActionRequest{}
	for rows.Next() {
		action, scanErr := scanAction(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, action)
	}
	return out, rows.Err()
}

// ClaimDeliverable claims requests awaiting delivery.
//
// PENDING and UNKNOWN are both claimable: UNKNOWN is the state after an
// ambiguous attempt, and the next attempt re-sends the SAME request under the
// same operation reference, which the owner treats as a replay rather than a
// second action.
func (s *NetworkContextStore) ClaimDeliverable(ctx context.Context, limit int,
	lease time.Duration) ([]supportmodels.ActionRequest, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `
		WITH due AS (
			SELECT id AS due_id FROM support_action_requests
			 WHERE status IN ('PENDING','UNKNOWN') AND next_attempt_at <= now()
			 ORDER BY next_attempt_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		UPDATE support_action_requests SET
			attempts = attempts + 1,
			next_attempt_at = now() + $2::interval,
			updated_at = now()
		  FROM due WHERE support_action_requests.id = due.due_id
		RETURNING `+qualifiedActionColumns, limit, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []supportmodels.ActionRequest{}
	for rows.Next() {
		action, scanErr := scanAction(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, action)
	}
	return out, rows.Err()
}

// RecordActionOutcome writes the owner's answer.
//
// A terminal answer -- ACCEPTED or REFUSED -- is written only over a row that
// is not already terminal. An owner's answer is final, and a late duplicate
// delivery must not overwrite it.
func (s *NetworkContextStore) RecordActionOutcome(ctx context.Context, id, status,
	resultRef, resultCode string, retryAfter time.Duration) (supportmodels.ActionRequest, error) {
	completed := status == supportmodels.ActionAccepted || status == supportmodels.ActionRefused
	return scanAction(s.db.QueryRow(ctx, `
		UPDATE support_action_requests SET
			status = $2, result_ref = $3, result_code = $4,
			completed_at = CASE WHEN $5 THEN now() ELSE NULL END,
			next_attempt_at = now() + $6::interval,
			request_version = request_version + 1,
			updated_at = now()
		 WHERE id = $1 AND status NOT IN ('ACCEPTED','REFUSED','DEAD_LETTER')
		RETURNING `+actionColumns, id, status, resultRef, resultCode, completed, retryAfter.String()))
}

// qualifiedActionColumns is actionColumns with the id qualified, for an
// UPDATE ... FROM ... RETURNING, where a bare "id" would be ambiguous against
// the CTE's own columns.
const qualifiedActionColumns = `support_action_requests.id, complaint_id, target_service,
	action_code, target_ref, parameters, request_version, status, idempotency_key,
	request_fingerprint, result_ref, result_code, requested_by, attempts, created_at,
	updated_at, completed_at`
