package vendors

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	// ErrCaseNotFound: no vendor case has this reference.
	ErrCaseNotFound = errors.New("vendor verification case not found")
	// ErrVersionConflict: the caller decided against a stale source version.
	ErrVersionConflict = errors.New("vendor verification case version conflict")
	// ErrAppealReused: an appeal reference can reopen a case only once.
	ErrAppealReused = errors.New("appeal reference already decided for this case")
)

// Decider kinds recorded in the formal history.
const (
	DecidedByStaff   = "STAFF"
	DecidedByService = "SERVICE"
	DecidedBySystem  = "SYSTEM"
)

// CallbackEventType is the durable Vendor callback this service already
// delivers (signed, retried, dead-lettered and replayable).
const CallbackEventType = "vendor.verification.status_changed"

// DecisionInput is one formal decision against a fenced case version.
type DecisionInput struct {
	CaseRef         string
	ExpectedVersion int64
	Status          string
	ReasonCode      string
	Remediation     string
	ExpiresAt       *time.Time
	AppealRef       string
	DecidedBy       string
	DecidedByKind   string
	OperationRef    string
	Now             time.Time
}

// DecisionResult carries the case before and after the decision.
type DecisionResult struct {
	Prior   CaseView
	Current CaseView
}

// CaseView is the case shape Vendor reads and receives in callbacks.
type CaseView = view

// LoadCaseForUpdate locks one case. An unparseable reference is simply absent.
func LoadCaseForUpdate(ctx context.Context, tx pgx.Tx, ref string) (CaseView, error) {
	id, err := uuid.Parse(ref)
	if err != nil || id.String() != strings.ToLower(ref) {
		return CaseView{}, ErrCaseNotFound
	}
	v, err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM vendor_verification_cases WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return CaseView{}, ErrCaseNotFound
	}
	return v, err
}

// ApplyDecision applies one formal decision inside the caller's transaction:
// lock, fence on source_version, run the native state machine, write the new
// version, the append-only history row (naming the human/service/system that
// decided), the appeal record for a reopen and the durable Vendor callback.
// The admin event is appended by the table trigger in the same transaction.
func ApplyDecision(ctx context.Context, tx pgx.Tx, in DecisionInput) (DecisionResult, error) {
	var out DecisionResult
	if strings.TrimSpace(in.DecidedBy) == "" || (in.DecidedByKind != DecidedByStaff && in.DecidedByKind != DecidedByService && in.DecidedByKind != DecidedBySystem) || strings.TrimSpace(in.OperationRef) == "" {
		return out, errors.New("decision must name its decider and operation")
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	prior, err := LoadCaseForUpdate(ctx, tx, in.CaseRef)
	if err != nil {
		return out, err
	}
	out.Prior = prior
	if prior.SourceVersion != in.ExpectedVersion {
		return out, ErrVersionConflict
	}
	if err := (Transition{From: prior.Status, To: in.Status, ReasonCode: in.ReasonCode, ExpiresAt: in.ExpiresAt, AppealRef: in.AppealRef, Now: in.Now}).Validate(); err != nil {
		return out, err
	}
	expires := in.ExpiresAt
	if expires == nil && (in.Status == StatusExpired || in.Status == StatusSuspended) {
		expires = prior.ExpiresAt
	}
	current, err := scan(tx.QueryRow(ctx, `UPDATE vendor_verification_cases SET status=$1,source_version=source_version+1,reason_code=NULLIF($2,''),remediation=NULLIF($3,''),expires_at=$4,observed_at=$5,updated_at=$5 WHERE id=$6::uuid AND source_version=$7 RETURNING `+cols,
		in.Status, in.ReasonCode, in.Remediation, expires, in.Now, prior.SourceRef, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrVersionConflict
	}
	if err != nil {
		return out, err
	}
	out.Current = current
	if _, err = tx.Exec(ctx, `INSERT INTO vendor_verification_case_history(case_id,source_version,status,reason_code,remediation,expires_at,observed_at,decided_by,decided_by_kind,appeal_ref) VALUES($1::uuid,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,NULLIF($10,''))`,
		current.SourceRef, current.SourceVersion, current.Status, current.ReasonCode, current.Remediation, current.ExpiresAt, current.ObservedAt, in.DecidedBy, in.DecidedByKind, in.AppealRef); err != nil {
		return out, err
	}
	if IsReopen(prior.Status, current.Status) {
		tag, err := tx.Exec(ctx, `INSERT INTO vendor_case_appeals(case_id,appeal_ref,status,reopened_from,case_version,decided_operation_ref,decided_by) VALUES($1::uuid,$2,'REOPENED',$3,$4,$5,$6) ON CONFLICT (case_id,appeal_ref) DO NOTHING`,
			current.SourceRef, in.AppealRef, prior.Status, current.SourceVersion, in.OperationRef, in.DecidedBy)
		if err != nil {
			return out, err
		}
		if tag.RowsAffected() != 1 {
			return out, ErrAppealReused
		}
	}
	payload, err := json.Marshal(current)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload) VALUES($1::uuid,$2,$3,$4)`, current.SourceRef, CallbackEventType, current.SourceVersion, payload); err != nil {
		return out, err
	}
	return out, nil
}
