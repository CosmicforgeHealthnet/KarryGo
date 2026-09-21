package fulfilment_readiness

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns this feature's statements.
//
// Readiness and its history move together or not at all. A state change with no
// history row is a change nobody can explain afterwards, and the explanation is
// the whole reason the history exists.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Pool exposes the pool for callers that must own the transaction.
func (r *Repository) Pool() *pgxpool.Pool { return r.db }

const readinessColumns = `vendor_order_id, parent_order_id, business_id, branch_id,
	vendor_type, state, substate, readiness_version, prep_started_at,
	predicted_ready_start, predicted_ready_end, vendor_estimated_ready_at,
	actual_ready_at, first_actual_ready_at, confidence_score, confidence_band,
	source, block_reason, delay_reason, last_signal_at, updated_at`

func scanReadiness(row pgx.Row) (Readiness, error) {
	var r Readiness
	err := row.Scan(&r.VendorOrderID, &r.ParentOrderID, &r.BusinessID, &r.BranchID,
		&r.VendorType, &r.State, &r.Substate, &r.Version, &r.PrepStartedAt,
		&r.PredictedReadyStart, &r.PredictedReadyEnd, &r.VendorEstimatedReadyAt,
		&r.ActualReadyAt, &r.FirstActualReadyAt, &r.ConfidenceScore, &r.ConfidenceBand,
		&r.Source, &r.BlockReason, &r.DelayReason, &r.LastSignalAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Get reads one order's readiness.
func (r *Repository) Get(ctx context.Context, vendorOrderID string) (Readiness, error) {
	return scanReadiness(r.db.QueryRow(ctx,
		`SELECT `+readinessColumns+` FROM commerce_vendor_order_readiness WHERE vendor_order_id=$1`,
		vendorOrderID))
}

// Ensure creates the readiness row for an order that has none yet.
//
// It exists because the readiness table predates this section and is also
// written by the production timing projection, so a row may already be there
// with timing but no state. ON CONFLICT therefore fills in the state fields
// without disturbing whatever timing evidence is already recorded.
func (r *Repository) Ensure(ctx context.Context, tx pgx.Tx, seed Readiness) (Readiness, error) {
	exec := func(query string, args ...any) pgx.Row {
		if tx != nil {
			return tx.QueryRow(ctx, query, args...)
		}
		return r.db.QueryRow(ctx, query, args...)
	}
	return scanReadiness(exec(`
		INSERT INTO commerce_vendor_order_readiness(
			vendor_order_id, parent_order_id, business_id, branch_id, production_job_id,
			vendor_type, state, readiness_version, source)
		VALUES($1,$2,$3,$4,'',$5,$6,1,$7)
		ON CONFLICT (vendor_order_id) DO UPDATE
		   SET vendor_type = CASE WHEN commerce_vendor_order_readiness.vendor_type = ''
		                          THEN EXCLUDED.vendor_type
		                          ELSE commerce_vendor_order_readiness.vendor_type END
		RETURNING `+readinessColumns,
		seed.VendorOrderID, seed.ParentOrderID, seed.BusinessID, seed.BranchID,
		seed.VendorType, seed.State, seed.Source))
}

// Lock reads readiness inside a transaction and holds it, so two staff members
// moving the same order serialize rather than interleave.
func (r *Repository) Lock(ctx context.Context, tx pgx.Tx, vendorOrderID string) (Readiness, error) {
	return scanReadiness(tx.QueryRow(ctx,
		`SELECT `+readinessColumns+` FROM commerce_vendor_order_readiness
		  WHERE vendor_order_id=$1 FOR UPDATE`, vendorOrderID))
}

// ApplyInput is one committed readiness change.
type ApplyInput struct {
	VendorOrderID   string
	ExpectedVersion int64
	State           State
	Substate        Substate
	Source          Source
	BlockReason     string
	DelayReason     string
	VendorEstimate  *time.Time
	Prediction      Prediction
	PrepStartedAt   *time.Time
	ActualReadyAt   *time.Time
	Now             time.Time
}

// Apply writes the new readiness, guarding on version.
//
// first_actual_ready_at is set with COALESCE so the first READY wins forever.
// The database also refuses to change it, which makes the rule hold even for a
// statement written later by someone who has not read this comment.
func (r *Repository) Apply(ctx context.Context, tx pgx.Tx, in ApplyInput) (Readiness, error) {
	updated, err := scanReadiness(tx.QueryRow(ctx, `
		UPDATE commerce_vendor_order_readiness
		   SET state=$3, substate=$4, source=$5,
		       block_reason=$6, delay_reason=$7,
		       vendor_estimated_ready_at=COALESCE($8, vendor_estimated_ready_at),
		       predicted_ready_start=$9, predicted_ready_end=$10,
		       confidence_score=$11, confidence_band=$12,
		       prep_started_at=COALESCE(prep_started_at, $13),
		       actual_ready_at=COALESCE($14, actual_ready_at),
		       first_actual_ready_at=COALESCE(first_actual_ready_at, $14),
		       last_signal_at=$15,
		       readiness_version=readiness_version+1,
		       updated_at=now()
		 WHERE vendor_order_id=$1 AND readiness_version=$2
		RETURNING `+readinessColumns,
		in.VendorOrderID, in.ExpectedVersion, in.State, in.Substate, in.Source,
		in.BlockReason, in.DelayReason, in.VendorEstimate,
		in.Prediction.Start, in.Prediction.End, in.Prediction.Score, in.Prediction.Band,
		in.PrepStartedAt, in.ActualReadyAt, in.Now))
	if errors.Is(err, ErrNotFound) {
		return Readiness{}, ErrVersion
	}
	return updated, err
}

// AppendHistory records one transition.
func (r *Repository) AppendHistory(ctx context.Context, tx pgx.Tx, entry HistoryEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO vendor_readiness_history(
			id, vendor_order_id, from_state, to_state, from_substate, to_substate,
			source, estimate_before, estimate_after, actor_ref, origin_event_ref,
			reason_code, readiness_version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		"rdh_"+uuid.NewString(), entry.VendorOrderID, entry.FromState, entry.ToState,
		entry.FromSubstate, entry.ToSubstate, entry.Source, entry.EstimateBefore,
		entry.EstimateAfter, entry.ActorRef, entry.OriginEventRef, entry.ReasonCode,
		entry.Version)
	return err
}

// History returns one order's transitions, newest first.
func (r *Repository) History(ctx context.Context, vendorOrderID string) ([]HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, vendor_order_id, from_state, to_state, from_substate, to_substate,
		       source, estimate_before, estimate_after, actor_ref, origin_event_ref,
		       reason_code, readiness_version, created_at
		  FROM vendor_readiness_history WHERE vendor_order_id=$1
		 ORDER BY readiness_version DESC`, vendorOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var entry HistoryEntry
		if err = rows.Scan(&entry.ID, &entry.VendorOrderID, &entry.FromState, &entry.ToState,
			&entry.FromSubstate, &entry.ToSubstate, &entry.Source, &entry.EstimateBefore,
			&entry.EstimateAfter, &entry.ActorRef, &entry.OriginEventRef, &entry.ReasonCode,
			&entry.Version, &entry.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// Profile reads a branch's measured baseline.
func (r *Repository) Profile(ctx context.Context, businessID, branchID string,
	vendorType VendorType) (BranchProfile, error) {
	var p BranchProfile
	err := r.db.QueryRow(ctx, `
		SELECT business_id, branch_id, vendor_type, profile_version,
		       baseline_p50_seconds, baseline_p80_seconds, baseline_p90_seconds,
		       handover_p50_seconds, handover_p90_seconds, confidence, sample_count, data_as_of
		  FROM vendor_branch_readiness_profiles
		 WHERE business_id=$1 AND branch_id=$2 AND vendor_type=$3`,
		businessID, branchID, vendorType).Scan(&p.BusinessID, &p.BranchID, &p.VendorType,
		&p.ProfileVersion, &p.BaselineP50, &p.BaselineP80, &p.BaselineP90,
		&p.HandoverP50, &p.HandoverP90, &p.Confidence, &p.SampleCount, &p.DataAsOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchProfile{}, ErrNotFound
	}
	return p, err
}

// UpsertProfile records a branch baseline, refusing a stale version.
func (r *Repository) UpsertProfile(ctx context.Context, p BranchProfile) error {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO vendor_branch_readiness_profiles(
			business_id, branch_id, vendor_type, profile_version,
			baseline_p50_seconds, baseline_p80_seconds, baseline_p90_seconds,
			handover_p50_seconds, handover_p90_seconds, confidence, sample_count, data_as_of)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (business_id, branch_id, vendor_type) DO UPDATE
		   SET profile_version=EXCLUDED.profile_version,
		       baseline_p50_seconds=EXCLUDED.baseline_p50_seconds,
		       baseline_p80_seconds=EXCLUDED.baseline_p80_seconds,
		       baseline_p90_seconds=EXCLUDED.baseline_p90_seconds,
		       handover_p50_seconds=EXCLUDED.handover_p50_seconds,
		       handover_p90_seconds=EXCLUDED.handover_p90_seconds,
		       confidence=EXCLUDED.confidence, sample_count=EXCLUDED.sample_count,
		       data_as_of=EXCLUDED.data_as_of, updated_at=now()
		 WHERE vendor_branch_readiness_profiles.profile_version < EXCLUDED.profile_version`,
		p.BusinessID, p.BranchID, p.VendorType, p.ProfileVersion,
		p.BaselineP50, p.BaselineP80, p.BaselineP90, p.HandoverP50, p.HandoverP90,
		p.Confidence, p.SampleCount, p.DataAsOf)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersion
	}
	return nil
}

// Rules reads every rule that could apply to a scope.
//
// It returns candidates rather than a single answer because the selection is a
// pure decision (SelectRules) that a test can exercise without a database.
func (r *Repository) Rules(ctx context.Context, vendorType VendorType,
	businessID, branchID string, now time.Time) ([]Rules, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, vendor_type, business_id, branch_id, default_prep_seconds,
		       ready_soon_threshold_seconds, stale_signal_seconds,
		       min_vendor_estimate_seconds, max_vendor_estimate_seconds, version
		  FROM vendor_readiness_rules
		 WHERE vendor_type=$1
		   AND (business_id='' OR business_id=$2)
		   AND (branch_id='' OR branch_id=$3)
		   AND effective_from<=$4
		   AND (effective_to IS NULL OR effective_to>$4)`,
		vendorType, businessID, branchID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rules{}
	for rows.Next() {
		var rule Rules
		var prep, readySoon, stale, minimum, maximum int
		if err = rows.Scan(&rule.ID, &rule.VendorType, &rule.BusinessID, &rule.BranchID,
			&prep, &readySoon, &stale, &minimum, &maximum, &rule.Version); err != nil {
			return nil, err
		}
		rule.DefaultPrep = time.Duration(prep) * time.Second
		rule.ReadySoonThreshold = time.Duration(readySoon) * time.Second
		rule.StaleSignal = time.Duration(stale) * time.Second
		rule.MinVendorEstimate = time.Duration(minimum) * time.Second
		rule.MaxVendorEstimate = time.Duration(maximum) * time.Second
		out = append(out, rule)
	}
	return out, rows.Err()
}

// ListStale returns active readiness rows that have gone quiet.
//
// The threshold is passed in rather than joined from the rules table because
// rules are scoped and versioned, and resolving the right one per row in SQL
// would put the selection logic in two places.
func (r *Repository) ListStale(ctx context.Context, cutoff time.Time, limit int) ([]Readiness, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+readinessColumns+` FROM commerce_vendor_order_readiness
		  WHERE state IN ('ACCEPTED','PREPARING','READY_SOON')
		    AND COALESCE(last_signal_at, updated_at) < $1
		  ORDER BY COALESCE(last_signal_at, updated_at) LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Readiness{}
	for rows.Next() {
		var readiness Readiness
		if err = rows.Scan(&readiness.VendorOrderID, &readiness.ParentOrderID,
			&readiness.BusinessID, &readiness.BranchID, &readiness.VendorType,
			&readiness.State, &readiness.Substate, &readiness.Version,
			&readiness.PrepStartedAt, &readiness.PredictedReadyStart,
			&readiness.PredictedReadyEnd, &readiness.VendorEstimatedReadyAt,
			&readiness.ActualReadyAt, &readiness.FirstActualReadyAt,
			&readiness.ConfidenceScore, &readiness.ConfidenceBand, &readiness.Source,
			&readiness.BlockReason, &readiness.DelayReason, &readiness.LastSignalAt,
			&readiness.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, readiness)
	}
	return out, rows.Err()
}
